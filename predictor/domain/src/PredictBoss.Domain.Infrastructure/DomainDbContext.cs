using System.Text.RegularExpressions;
using Microsoft.EntityFrameworkCore;
using PredictBoss.Domain.Core;

namespace PredictBoss.Domain.Infrastructure;

public sealed class DomainDbContext(DbContextOptions<DomainDbContext> options) : DbContext(options)
{
    public DbSet<User> Users => Set<User>();
    public DbSet<Company> Companies => Set<Company>();
    public DbSet<NewsSource> NewsSources => Set<NewsSource>();
    public DbSet<NewsLink> NewsLinks => Set<NewsLink>();

    protected override void OnModelCreating(ModelBuilder model)
    {
        model.Entity<User>(b =>
        {
            b.ToTable("users");
            b.HasKey(x => x.Id);
            b.HasIndex(x => x.ExternalSubject).IsUnique();
            b.HasIndex(x => x.NormalizedLogin).IsUnique().HasFilter("normalized_login IS NOT NULL");
            b.Property(x => x.ExternalSubject).IsRequired();
            b.Property(x => x.CreatedAt).HasDefaultValueSql("now()");
        });

        model.Entity<Company>(b =>
        {
            b.ToTable("companies", t =>
            {
                t.HasCheckConstraint("ck_companies_revenue", "revenue_company IS NULL OR revenue_company >= 0");
                t.HasCheckConstraint("ck_companies_revision", "profile_revision > 0");
                t.HasCheckConstraint("ck_companies_client_type", "client_type IS NULL OR client_type IN ('PROSPECT', 'CLIENT', 'CLOSE_CLIENT')");
                t.HasCheckConstraint("ck_companies_valera_status", "valera_delivery_status IN ('pending', 'retry', 'accepted')");
                t.HasCheckConstraint("ck_companies_valera_revision", "valera_last_sent_revision IS NULL OR (valera_last_sent_revision > 0 AND valera_last_sent_revision <= profile_revision)");
            });
            b.HasKey(x => x.Id);
            b.HasOne(x => x.Owner).WithOne(x => x.Company).HasForeignKey<Company>(x => x.OwnerUserId);
            b.HasIndex(x => x.SyntheticU).IsUnique();
            b.HasIndex(x => new { x.ValeraDeliveryStatus, x.ValeraNextRetryAt, x.Id })
                .HasFilter("valera_delivery_status IN ('pending', 'retry')");
            b.Property(x => x.RevenueCompany).HasPrecision(20, 2);
            b.Property(x => x.ImportPayload).HasColumnType("jsonb");
            b.Property(x => x.ImportWarnings).HasColumnType("jsonb").HasDefaultValueSql("'[]'::jsonb");
            b.Property(x => x.ProfileRevision).HasDefaultValue(1L).IsConcurrencyToken();
            b.Property(x => x.ValeraDeliveryStatus).HasDefaultValue("pending");
            b.Property(x => x.ValeraAttempts).HasDefaultValue(0);
            b.Property(x => x.CreatedAt).HasDefaultValueSql("now()");
            b.Property(x => x.UpdatedAt).HasDefaultValueSql("now()");
        });

        model.Entity<NewsSource>(b =>
        {
            b.ToTable("news_sources", t =>
            {
                t.HasCheckConstraint("ck_sources_kind", "kind IS NULL OR kind IN ('rss', 'atom', 'sitemap', 'html_index')");
                t.HasCheckConstraint("ck_sources_validation", "validation_status IN ('pending', 'active', 'unsupported', 'blocked')");
                t.HasCheckConstraint("ck_sources_interval", "poll_interval_minutes >= 15");
                t.HasCheckConstraint("ck_sources_version", "version > 0");
            });
            b.HasKey(x => x.Id);
            b.HasOne(x => x.Company).WithMany(x => x.Sources).HasForeignKey(x => x.CompanyId);
            b.HasAlternateKey(x => new { x.Id, x.CompanyId });
            b.HasIndex(x => new { x.CompanyId, x.CanonicalUrl }).IsUnique();
            b.HasIndex(x => new { x.NextPollAt, x.Id })
                .HasFilter("enabled = true AND deleted_at IS NULL AND validation_status = 'active'");
            b.HasIndex(x => new { x.CompanyId, x.CreatedAt, x.Id })
                .HasFilter("deleted_at IS NULL");
            b.Property(x => x.ValidationStatus).HasDefaultValue("pending");
            b.Property(x => x.Enabled).HasDefaultValue(true);
            b.Property(x => x.Version).HasDefaultValue(1L).IsConcurrencyToken();
            b.Property(x => x.PollIntervalMinutes).HasDefaultValue(60);
            b.Property(x => x.CreatedAt).HasDefaultValueSql("now()");
            b.Property(x => x.UpdatedAt).HasDefaultValueSql("now()");
        });

        model.Entity<NewsLink>(b =>
        {
            b.ToTable("news_links", t =>
            {
                t.HasCheckConstraint("ck_links_kind", "material_kind IN ('external_news', 'alpha_analysis')");
                t.HasCheckConstraint("ck_links_mode", "ingest_mode IN ('scheduled', 'one_time')");
                t.HasCheckConstraint("ck_links_mode_source", "(ingest_mode = 'one_time' AND news_source_id IS NULL) OR (ingest_mode = 'scheduled' AND news_source_id IS NOT NULL)");
                t.HasCheckConstraint("ck_links_extraction", "extraction_status IN ('pending', 'extracted', 'retry', 'unsupported', 'failed')");
                t.HasCheckConstraint("ck_links_send", "send_status IN ('pending', 'retry', 'accepted', 'skipped', 'failed')");
                t.HasCheckConstraint("ck_links_attempts", "send_attempts >= 0");
            });
            b.HasKey(x => x.Id);
            b.HasOne(x => x.Company).WithMany(x => x.Links).HasForeignKey(x => x.CompanyId);
            b.HasOne(x => x.NewsSource).WithMany(x => x.Links)
                .HasForeignKey(x => new { x.NewsSourceId, x.CompanyId })
                .HasPrincipalKey(x => new { x.Id, x.CompanyId });
            b.HasIndex(x => new { x.CompanyId, x.CanonicalUrl }).IsUnique();
            b.HasIndex(x => new { x.SendStatus, x.NextRetryAt, x.Id })
                .HasFilter("send_status IN ('pending', 'retry')");
            b.HasIndex(x => new { x.CompanyId, x.DiscoveredAt, x.Id });
            b.Property(x => x.MaterialKind).HasDefaultValue("external_news");
            b.Property(x => x.IngestMode).HasDefaultValue("one_time");
            b.Property(x => x.ExtractionStatus).HasDefaultValue("pending");
            b.Property(x => x.SendStatus).HasDefaultValue("pending");
            b.Property(x => x.SendAttempts).HasDefaultValue(0);
            b.Property(x => x.DiscoveredAt).HasDefaultValueSql("now()");
            b.Property(x => x.LastSeenAt).HasDefaultValueSql("now()");
        });

        foreach (var entity in model.Model.GetEntityTypes())
        foreach (var property in entity.GetProperties())
            property.SetColumnName(Regex.Replace(property.Name, "(?<!^)([A-Z])", "_$1").ToLowerInvariant());
    }
}
