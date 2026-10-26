using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Design;

namespace PredictBoss.Domain.Infrastructure;

public sealed class DesignTimeDbContextFactory : IDesignTimeDbContextFactory<DomainDbContext>
{
    public DomainDbContext CreateDbContext(string[] args)
    {
        var connection = Environment.GetEnvironmentVariable("DOMAIN_DATABASE_URL")
            ?? "Host=localhost;Port=5433;Database=predictor_domain;Username=predictor_domain;Password=local-domain-password";
        return new DomainDbContext(new DbContextOptionsBuilder<DomainDbContext>().UseNpgsql(connection).Options);
    }
}
