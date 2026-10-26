using Microsoft.EntityFrameworkCore;
using Npgsql;
using PredictBoss.Domain.Core;
using PredictBoss.Domain.Infrastructure;

namespace PredictBoss.Domain.Worker;

public sealed class IngestionWorker(
    IDbContextFactory<DomainDbContext> contexts,
    LeaseStore leases,
    MaterialReader reader,
    EsbClient esb,
    NpgsqlDataSource dataSource,
    ILogger<IngestionWorker> logger) : BackgroundService
{
    private readonly string owner = $"{Environment.MachineName}:{Guid.NewGuid():N}";

    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        var loops = new List<Task> { RunLoop(leases.ClaimProfile, ProcessProfile, stoppingToken),
            RunLoop(leases.ClaimSource, ProcessSource, stoppingToken) };
        // Each link loop can run in parallel; the database lease coordinates other replicas.
        loops.AddRange(Enumerable.Range(0, 4).Select(_ => RunLoop(leases.ClaimLink, ProcessLink, stoppingToken)));
        await Task.WhenAll(loops);
    }

    private async Task RunLoop(Func<string, CancellationToken, Task<Guid?>> claim,
        Func<Guid, CancellationToken, Task> process, CancellationToken ct)
    {
        while (!ct.IsCancellationRequested)
        {
            try
            {
                var id = await claim(owner, ct);
                if (id is null) await Task.Delay(TimeSpan.FromSeconds(2), ct);
                else await process(id.Value, ct);
            }
            catch (OperationCanceledException) when (ct.IsCancellationRequested) { break; }
            catch (Exception error)
            {
                logger.LogError(error, "Worker loop failed");
                await Task.Delay(TimeSpan.FromSeconds(3), ct);
            }
        }
    }

    private async Task ProcessProfile(Guid id, CancellationToken ct)
    {
        await using var db = await contexts.CreateDbContextAsync(ct);
        var company = await db.Companies.SingleAsync(x => x.Id == id, ct);
        var revision = company.ProfileRevision;
        try
        {
            await esb.SendProfile(company, ct);
            // A newer edit must remain pending even if the earlier revision was delivered.
            await db.Companies.Where(x => x.Id == id && x.ValeraLeaseOwner == owner && x.ProfileRevision == revision)
                .ExecuteUpdateAsync(s => s.SetProperty(x => x.ValeraDeliveryStatus, "accepted")
                    .SetProperty(x => x.ValeraLastSentRevision, revision)
                    .SetProperty(x => x.ValeraLeaseOwner, (string?)null)
                    .SetProperty(x => x.ValeraLeaseUntil, (DateTime?)null)
                    .SetProperty(x => x.ValeraLastError, (string?)null), ct);
        }
        catch (Exception e) when (e is not OperationCanceledException)
        {
            logger.LogWarning(e, "Profile delivery failed for {CompanyId}", id);
            await db.Companies.Where(x => x.Id == id && x.ValeraLeaseOwner == owner && x.ProfileRevision == revision)
                .ExecuteUpdateAsync(s => s.SetProperty(x => x.ValeraDeliveryStatus, "retry")
                    .SetProperty(x => x.ValeraAttempts, x => x.ValeraAttempts + 1)
                    .SetProperty(x => x.ValeraNextRetryAt, DateTime.UtcNow.AddMinutes(1))
                    .SetProperty(x => x.ValeraLeaseOwner, (string?)null)
                    .SetProperty(x => x.ValeraLeaseUntil, (DateTime?)null)
                    .SetProperty(x => x.ValeraLastError, e.Message[..Math.Min(e.Message.Length, 500)]), ct);
        }
    }

    private async Task ProcessSource(Guid id, CancellationToken ct)
    {
        await using var db = await contexts.CreateDbContextAsync(ct);
        var source = await db.NewsSources.SingleAsync(x => x.Id == id, ct);
        try
        {
            var result = await reader.Discover(source.Url, ct);
            foreach (var url in result.Links)
            {
                if (!TryCanonical(url, out var canonical)) continue;
                await UpsertLink(source, url, canonical, ct);
            }
            await db.NewsSources.Where(x => x.Id == id && x.LeaseOwner == owner)
                .ExecuteUpdateAsync(s => s.SetProperty(x => x.Kind, result.Kind)
                    .SetProperty(x => x.ValidationStatus, "active")
                    .SetProperty(x => x.LastSuccessAt, DateTime.UtcNow)
                    .SetProperty(x => x.NextPollAt, DateTime.UtcNow.AddMinutes(source.PollIntervalMinutes))
                    .SetProperty(x => x.LeaseOwner, (string?)null)
                    .SetProperty(x => x.LeaseUntil, (DateTime?)null)
                    .SetProperty(x => x.LastError, (string?)null), ct);
        }
        catch (Exception e) when (e is not OperationCanceledException)
        {
            var permanent = e is NotSupportedException;
            logger.LogWarning(e, "Source poll failed for {SourceId}", id);
            await db.NewsSources.Where(x => x.Id == id && x.LeaseOwner == owner)
                .ExecuteUpdateAsync(s => s.SetProperty(x => x.ValidationStatus, permanent ? "unsupported" : "pending")
                    .SetProperty(x => x.NextPollAt, DateTime.UtcNow.AddMinutes(permanent ? 60 : 10))
                    .SetProperty(x => x.LeaseOwner, (string?)null)
                    .SetProperty(x => x.LeaseUntil, (DateTime?)null)
                    .SetProperty(x => x.LastError, e.Message[..Math.Min(e.Message.Length, 500)]), ct);
        }
    }

    private async Task UpsertLink(NewsSource source, string url, string canonical, CancellationToken ct)
    {
        await using var connection = await dataSource.OpenConnectionAsync(ct);
        await using var command = new NpgsqlCommand("""
            INSERT INTO news_links (id, company_id, news_source_id, url, canonical_url, material_kind,
                                    ingest_mode, extraction_status, send_status, send_attempts, discovered_at, last_seen_at)
            VALUES (@id, @company, @source, @url, @canonical, 'external_news', 'scheduled',
                    'pending', 'pending', 0, now(), now())
            ON CONFLICT (company_id, canonical_url) DO UPDATE SET last_seen_at = now()
            """, connection);
        command.Parameters.AddWithValue("id", Guid.NewGuid());
        command.Parameters.AddWithValue("company", source.CompanyId);
        command.Parameters.AddWithValue("source", source.Id);
        command.Parameters.AddWithValue("url", url);
        command.Parameters.AddWithValue("canonical", canonical);
        await command.ExecuteNonQueryAsync(ct);
    }

    private async Task ProcessLink(Guid id, CancellationToken ct)
    {
        await using var db = await contexts.CreateDbContextAsync(ct);
        var link = await db.NewsLinks.SingleAsync(x => x.Id == id, ct);
        try
        {
            var material = await reader.ReadMaterial(link.Url, ct);
            if (System.Text.Encoding.UTF8.GetByteCount(material.Text) > 1024 * 1024)
                throw new NotSupportedException("Extracted text exceeds ESB's 1 MiB limit");
            await db.NewsLinks.Where(x => x.Id == id && x.LeaseOwner == owner)
                .ExecuteUpdateAsync(s => s.SetProperty(x => x.MediaType, material.MediaType)
                    .SetProperty(x => x.Title, material.Title)
                    .SetProperty(x => x.ExtractionStatus, "extracted"), ct);
            await esb.SendNews(link, material.Text, material.MediaType, material.Title, ct);
            await db.NewsLinks.Where(x => x.Id == id && x.LeaseOwner == owner)
                .ExecuteUpdateAsync(s => s.SetProperty(x => x.SendStatus, "accepted")
                    .SetProperty(x => x.LeaseOwner, (string?)null)
                    .SetProperty(x => x.LeaseUntil, (DateTime?)null)
                    .SetProperty(x => x.LastError, (string?)null), ct);
        }
        catch (Exception e) when (e is not OperationCanceledException)
        {
            var permanent = e is NotSupportedException;
            logger.LogWarning(e, "Material processing failed for {ArticleId}", id);
            await db.NewsLinks.Where(x => x.Id == id && x.LeaseOwner == owner)
                .ExecuteUpdateAsync(s => s.SetProperty(x => x.ExtractionStatus,
                        permanent ? "unsupported" : link.ExtractionStatus == "extracted" ? "extracted" : "retry")
                    .SetProperty(x => x.SendStatus, permanent ? "skipped" : link.SendAttempts >= 4 ? "failed" : "retry")
                    .SetProperty(x => x.SendAttempts, x => x.SendAttempts + 1)
                    .SetProperty(x => x.NextRetryAt, DateTime.UtcNow.AddMinutes(Math.Min(60, 1 << Math.Min(link.SendAttempts, 6))))
                    .SetProperty(x => x.LeaseOwner, (string?)null)
                    .SetProperty(x => x.LeaseUntil, (DateTime?)null)
                    .SetProperty(x => x.LastError, e.Message[..Math.Min(e.Message.Length, 500)]), ct);
        }
    }

    private static bool TryCanonical(string raw, out string canonical)
    {
        canonical = "";
        if (!Uri.TryCreate(raw, UriKind.Absolute, out var uri) || uri.Scheme is not ("https" or "http")
            || uri.UserInfo.Length > 0) return false;
        canonical = new UriBuilder(uri) { Fragment = "", Host = uri.Host.ToLowerInvariant() }.Uri.AbsoluteUri.TrimEnd('/');
        return true;
    }
}
