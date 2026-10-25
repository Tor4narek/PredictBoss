using System.Net.Http.Headers;
using System.Net.Http.Json;
using PredictBoss.Domain.Core;
using PredictBoss.Domain.Infrastructure;

namespace PredictBoss.Domain.Worker;

public sealed class EsbClient
{
    private readonly HttpClient http;

    public EsbClient(IConfiguration config)
    {
        var baseUrl = config["ESB_BASE_URL"] ?? throw new InvalidOperationException("ESB_BASE_URL is required");
        var key = config["ESB_API_KEY"] ?? throw new InvalidOperationException("ESB_API_KEY is required");
        http = new HttpClient { BaseAddress = new Uri(baseUrl), Timeout = TimeSpan.FromSeconds(30) };
        http.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", key);
    }

    public async Task SendProfile(Company c, CancellationToken ct)
    {
        // The ESB envelope is isolated here while its final contract is being agreed.
        using var response = await http.PostAsJsonAsync("/call?agent=valera", new
        {
            companyId = c.Id,
            profileRevision = c.ProfileRevision,
            company = CompanyProfiles.ToProfile(c)
        }, ct);
        response.EnsureSuccessStatusCode();
    }

    public async Task SendNews(NewsLink link, string text, string? mediaType, string? title, CancellationToken ct)
    {
        using var response = await http.PostAsJsonAsync("/call?agent=serezha", new
        {
            news_id = link.Id,
            source_id = link.Id,
            company_id = link.CompanyId,
            source_url = link.CanonicalUrl,
            material_kind = link.MaterialKind,
            media_type = mediaType ?? "text/html",
            title,
            published_at = link.PublishedAt,
            news_text = text
        }, ct);
        response.EnsureSuccessStatusCode();
    }
}
