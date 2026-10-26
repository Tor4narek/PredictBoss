using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace PredictBoss.Domain.Infrastructure.Migrations
{
    /// <inheritdoc />
    public partial class InitialDomain : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.CreateTable(
                name: "users",
                columns: table => new
                {
                    id = table.Column<Guid>(type: "uuid", nullable: false),
                    external_subject = table.Column<string>(type: "text", nullable: false),
                    display_name = table.Column<string>(type: "text", nullable: true),
                    created_at = table.Column<DateTime>(type: "timestamp with time zone", nullable: false, defaultValueSql: "now()")
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_users", x => x.id);
                });

            migrationBuilder.CreateTable(
                name: "companies",
                columns: table => new
                {
                    id = table.Column<Guid>(type: "uuid", nullable: false),
                    owner_user_id = table.Column<Guid>(type: "uuid", nullable: false),
                    synthetic_u = table.Column<string>(type: "text", nullable: true),
                    org_type = table.Column<string>(type: "text", nullable: true),
                    ownership_form = table.Column<string>(type: "text", nullable: true),
                    company_age_gradation = table.Column<string>(type: "text", nullable: true),
                    development_stage = table.Column<string>(type: "text", nullable: true),
                    main_okved_code = table.Column<string>(type: "text", nullable: true),
                    main_okved_name = table.Column<string>(type: "text", nullable: true),
                    negative_mark = table.Column<bool>(type: "boolean", nullable: true),
                    workers_fns_gradation = table.Column<string>(type: "text", nullable: true),
                    workers_spark_gradation = table.Column<string>(type: "text", nullable: true),
                    client_type = table.Column<string>(type: "text", nullable: true),
                    has_rko = table.Column<bool>(type: "boolean", nullable: true),
                    has_guarantees = table.Column<bool>(type: "boolean", nullable: true),
                    has_loans = table.Column<bool>(type: "boolean", nullable: true),
                    has_leasing = table.Column<bool>(type: "boolean", nullable: true),
                    has_deposits = table.Column<bool>(type: "boolean", nullable: true),
                    has_payroll = table.Column<bool>(type: "boolean", nullable: true),
                    fts_flag = table.Column<bool>(type: "boolean", nullable: true),
                    fts_country_name = table.Column<string>(type: "text", nullable: true),
                    has_factoring = table.Column<bool>(type: "boolean", nullable: true),
                    revenue_company = table.Column<decimal>(type: "numeric(20,2)", precision: 20, scale: 2, nullable: true),
                    group_id = table.Column<string>(type: "text", nullable: true),
                    city = table.Column<string>(type: "text", nullable: true),
                    region = table.Column<string>(type: "text", nullable: true),
                    import_payload = table.Column<string>(type: "jsonb", nullable: true),
                    import_warnings = table.Column<string>(type: "jsonb", nullable: false, defaultValueSql: "'[]'::jsonb"),
                    profile_revision = table.Column<long>(type: "bigint", nullable: false, defaultValue: 1L),
                    valera_delivery_status = table.Column<string>(type: "text", nullable: false, defaultValue: "pending"),
                    valera_last_sent_revision = table.Column<long>(type: "bigint", nullable: true),
                    valera_attempts = table.Column<int>(type: "integer", nullable: false, defaultValue: 0),
                    valera_next_retry_at = table.Column<DateTime>(type: "timestamp with time zone", nullable: true),
                    valera_lease_owner = table.Column<string>(type: "text", nullable: true),
                    valera_lease_until = table.Column<DateTime>(type: "timestamp with time zone", nullable: true),
                    valera_last_error = table.Column<string>(type: "text", nullable: true),
                    created_at = table.Column<DateTime>(type: "timestamp with time zone", nullable: false, defaultValueSql: "now()"),
                    updated_at = table.Column<DateTime>(type: "timestamp with time zone", nullable: false, defaultValueSql: "now()")
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_companies", x => x.id);
                    table.CheckConstraint("ck_companies_client_type", "client_type IS NULL OR client_type IN ('PROSPECT', 'CLIENT', 'CLOSE_CLIENT')");
                    table.CheckConstraint("ck_companies_revenue", "revenue_company IS NULL OR revenue_company >= 0");
                    table.CheckConstraint("ck_companies_revision", "profile_revision > 0");
                    table.CheckConstraint("ck_companies_valera_revision", "valera_last_sent_revision IS NULL OR (valera_last_sent_revision > 0 AND valera_last_sent_revision <= profile_revision)");
                    table.CheckConstraint("ck_companies_valera_status", "valera_delivery_status IN ('pending', 'retry', 'accepted')");
                    table.ForeignKey(
                        name: "FK_companies_users_owner_user_id",
                        column: x => x.owner_user_id,
                        principalTable: "users",
                        principalColumn: "id",
                        onDelete: ReferentialAction.Cascade);
                });

            migrationBuilder.CreateTable(
                name: "news_sources",
                columns: table => new
                {
                    id = table.Column<Guid>(type: "uuid", nullable: false),
                    company_id = table.Column<Guid>(type: "uuid", nullable: false),
                    url = table.Column<string>(type: "text", nullable: false),
                    canonical_url = table.Column<string>(type: "text", nullable: false),
                    name = table.Column<string>(type: "text", nullable: true),
                    kind = table.Column<string>(type: "text", nullable: true),
                    validation_status = table.Column<string>(type: "text", nullable: false, defaultValue: "pending"),
                    enabled = table.Column<bool>(type: "boolean", nullable: false, defaultValue: true),
                    version = table.Column<long>(type: "bigint", nullable: false, defaultValue: 1L),
                    deleted_at = table.Column<DateTime>(type: "timestamp with time zone", nullable: true),
                    poll_interval_minutes = table.Column<int>(type: "integer", nullable: false, defaultValue: 60),
                    next_poll_at = table.Column<DateTime>(type: "timestamp with time zone", nullable: true),
                    last_success_at = table.Column<DateTime>(type: "timestamp with time zone", nullable: true),
                    lease_owner = table.Column<string>(type: "text", nullable: true),
                    lease_until = table.Column<DateTime>(type: "timestamp with time zone", nullable: true),
                    last_error = table.Column<string>(type: "text", nullable: true),
                    created_at = table.Column<DateTime>(type: "timestamp with time zone", nullable: false, defaultValueSql: "now()"),
                    updated_at = table.Column<DateTime>(type: "timestamp with time zone", nullable: false, defaultValueSql: "now()")
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_news_sources", x => x.id);
                    table.UniqueConstraint("AK_news_sources_id_company_id", x => new { x.id, x.company_id });
                    table.CheckConstraint("ck_sources_interval", "poll_interval_minutes >= 15");
                    table.CheckConstraint("ck_sources_kind", "kind IS NULL OR kind IN ('rss', 'atom', 'sitemap', 'html_index')");
                    table.CheckConstraint("ck_sources_validation", "validation_status IN ('pending', 'active', 'unsupported', 'blocked')");
                    table.CheckConstraint("ck_sources_version", "version > 0");
                    table.ForeignKey(
                        name: "FK_news_sources_companies_company_id",
                        column: x => x.company_id,
                        principalTable: "companies",
                        principalColumn: "id",
                        onDelete: ReferentialAction.Cascade);
                });

            migrationBuilder.CreateTable(
                name: "news_links",
                columns: table => new
                {
                    id = table.Column<Guid>(type: "uuid", nullable: false),
                    company_id = table.Column<Guid>(type: "uuid", nullable: false),
                    news_source_id = table.Column<Guid>(type: "uuid", nullable: true),
                    url = table.Column<string>(type: "text", nullable: false),
                    canonical_url = table.Column<string>(type: "text", nullable: false),
                    material_kind = table.Column<string>(type: "text", nullable: false, defaultValue: "external_news"),
                    ingest_mode = table.Column<string>(type: "text", nullable: false, defaultValue: "one_time"),
                    media_type = table.Column<string>(type: "text", nullable: true),
                    title = table.Column<string>(type: "text", nullable: true),
                    published_at = table.Column<DateTime>(type: "timestamp with time zone", nullable: true),
                    discovered_at = table.Column<DateTime>(type: "timestamp with time zone", nullable: false, defaultValueSql: "now()"),
                    last_seen_at = table.Column<DateTime>(type: "timestamp with time zone", nullable: false, defaultValueSql: "now()"),
                    content_hash = table.Column<string>(type: "text", nullable: true),
                    extraction_status = table.Column<string>(type: "text", nullable: false, defaultValue: "pending"),
                    send_status = table.Column<string>(type: "text", nullable: false, defaultValue: "pending"),
                    send_attempts = table.Column<int>(type: "integer", nullable: false),
                    next_retry_at = table.Column<DateTime>(type: "timestamp with time zone", nullable: true),
                    lease_owner = table.Column<string>(type: "text", nullable: true),
                    lease_until = table.Column<DateTime>(type: "timestamp with time zone", nullable: true),
                    last_error = table.Column<string>(type: "text", nullable: true)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_news_links", x => x.id);
                    table.CheckConstraint("ck_links_attempts", "send_attempts >= 0");
                    table.CheckConstraint("ck_links_extraction", "extraction_status IN ('pending', 'extracted', 'retry', 'unsupported', 'failed')");
                    table.CheckConstraint("ck_links_kind", "material_kind IN ('external_news', 'alpha_analysis')");
                    table.CheckConstraint("ck_links_mode", "ingest_mode IN ('scheduled', 'one_time')");
                    table.CheckConstraint("ck_links_mode_source", "(ingest_mode = 'one_time' AND news_source_id IS NULL) OR (ingest_mode = 'scheduled' AND news_source_id IS NOT NULL)");
                    table.CheckConstraint("ck_links_send", "send_status IN ('pending', 'retry', 'accepted', 'skipped', 'failed')");
                    table.ForeignKey(
                        name: "FK_news_links_companies_company_id",
                        column: x => x.company_id,
                        principalTable: "companies",
                        principalColumn: "id",
                        onDelete: ReferentialAction.Cascade);
                    table.ForeignKey(
                        name: "FK_news_links_news_sources_news_source_id_company_id",
                        columns: x => new { x.news_source_id, x.company_id },
                        principalTable: "news_sources",
                        principalColumns: new[] { "id", "company_id" });
                });

            migrationBuilder.CreateIndex(
                name: "IX_companies_owner_user_id",
                table: "companies",
                column: "owner_user_id",
                unique: true);

            migrationBuilder.CreateIndex(
                name: "IX_companies_synthetic_u",
                table: "companies",
                column: "synthetic_u",
                unique: true);

            migrationBuilder.CreateIndex(
                name: "IX_companies_valera_delivery_status_valera_next_retry_at_id",
                table: "companies",
                columns: new[] { "valera_delivery_status", "valera_next_retry_at", "id" },
                filter: "valera_delivery_status IN ('pending', 'retry')");

            migrationBuilder.CreateIndex(
                name: "IX_news_links_company_id_canonical_url",
                table: "news_links",
                columns: new[] { "company_id", "canonical_url" },
                unique: true);

            migrationBuilder.CreateIndex(
                name: "IX_news_links_company_id_discovered_at_id",
                table: "news_links",
                columns: new[] { "company_id", "discovered_at", "id" });

            migrationBuilder.CreateIndex(
                name: "IX_news_links_news_source_id_company_id",
                table: "news_links",
                columns: new[] { "news_source_id", "company_id" });

            migrationBuilder.CreateIndex(
                name: "IX_news_links_send_status_next_retry_at_id",
                table: "news_links",
                columns: new[] { "send_status", "next_retry_at", "id" },
                filter: "send_status IN ('pending', 'retry')");

            migrationBuilder.CreateIndex(
                name: "IX_news_sources_company_id_canonical_url",
                table: "news_sources",
                columns: new[] { "company_id", "canonical_url" },
                unique: true);

            migrationBuilder.CreateIndex(
                name: "IX_news_sources_company_id_created_at_id",
                table: "news_sources",
                columns: new[] { "company_id", "created_at", "id" },
                filter: "deleted_at IS NULL");

            migrationBuilder.CreateIndex(
                name: "IX_news_sources_next_poll_at_id",
                table: "news_sources",
                columns: new[] { "next_poll_at", "id" },
                filter: "enabled = true AND deleted_at IS NULL AND validation_status = 'active'");

            migrationBuilder.CreateIndex(
                name: "IX_users_external_subject",
                table: "users",
                column: "external_subject",
                unique: true);
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropTable(
                name: "news_links");

            migrationBuilder.DropTable(
                name: "news_sources");

            migrationBuilder.DropTable(
                name: "companies");

            migrationBuilder.DropTable(
                name: "users");
        }
    }
}
