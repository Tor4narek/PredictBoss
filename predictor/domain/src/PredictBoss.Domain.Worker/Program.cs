using PredictBoss.Domain.Worker;
using Microsoft.EntityFrameworkCore;
using Npgsql;
using PredictBoss.Domain.Infrastructure;

if (args is ["--install-browser"])
{
    Environment.ExitCode = Microsoft.Playwright.Program.Main(["install", "--with-deps", "chromium"]);
    return;
}

var builder = Host.CreateApplicationBuilder(args);
var connection = builder.Configuration["DOMAIN_DATABASE_URL"]
    ?? throw new InvalidOperationException("DOMAIN_DATABASE_URL is required");
builder.Services.AddDbContextFactory<DomainDbContext>(x => x.UseNpgsql(connection));
builder.Services.AddSingleton(NpgsqlDataSource.Create(connection));
builder.Services.AddSingleton<LeaseStore>();
builder.Services.AddSingleton<MaterialReader>();
builder.Services.AddSingleton<EsbClient>();
builder.Services.AddHostedService<IngestionWorker>();

var host = builder.Build();
await host.RunAsync();
