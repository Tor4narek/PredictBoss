using Npgsql;

namespace PredictBoss.Domain.Worker;

public sealed class LeaseStore(NpgsqlDataSource dataSource)
{
    public async Task<Guid?> ClaimProfile(string owner, CancellationToken ct) => await Claim(
        """
        WITH candidate AS (
          SELECT id FROM companies
          WHERE valera_delivery_status IN ('pending','retry')
            AND (valera_next_retry_at IS NULL OR valera_next_retry_at <= now())
            AND (valera_lease_until IS NULL OR valera_lease_until < now())
          ORDER BY updated_at, id FOR UPDATE SKIP LOCKED LIMIT 1
        )
        UPDATE companies c SET valera_lease_owner = @owner, valera_lease_until = now() + interval '2 minutes'
        FROM candidate WHERE c.id = candidate.id RETURNING c.id
        """, owner, ct);

    public async Task<Guid?> ClaimSource(string owner, CancellationToken ct) => await Claim(
        """
        WITH candidate AS (
          SELECT id FROM news_sources
          WHERE enabled AND deleted_at IS NULL
            AND validation_status IN ('pending','active')
            AND (next_poll_at IS NULL OR next_poll_at <= now())
            AND (lease_until IS NULL OR lease_until < now())
          ORDER BY next_poll_at NULLS FIRST, id FOR UPDATE SKIP LOCKED LIMIT 1
        )
        UPDATE news_sources s SET lease_owner = @owner, lease_until = now() + interval '3 minutes'
        FROM candidate WHERE s.id = candidate.id RETURNING s.id
        """, owner, ct);

    public async Task<Guid?> ClaimLink(string owner, CancellationToken ct) => await Claim(
        """
        WITH candidate AS (
          SELECT id FROM news_links
          WHERE send_status IN ('pending','retry')
            AND (next_retry_at IS NULL OR next_retry_at <= now())
            AND (lease_until IS NULL OR lease_until < now())
          ORDER BY discovered_at, id FOR UPDATE SKIP LOCKED LIMIT 1
        )
        UPDATE news_links l SET lease_owner = @owner, lease_until = now() + interval '3 minutes'
        FROM candidate WHERE l.id = candidate.id RETURNING l.id
        """, owner, ct);

    private async Task<Guid?> Claim(string sql, string owner, CancellationToken ct)
    {
        await using var connection = await dataSource.OpenConnectionAsync(ct);
        await using var command = new NpgsqlCommand(sql, connection);
        command.Parameters.AddWithValue("owner", owner);
        var id = await command.ExecuteScalarAsync(ct);
        return id is Guid value ? value : null;
    }
}
