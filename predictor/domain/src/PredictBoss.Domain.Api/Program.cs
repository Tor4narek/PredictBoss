using System.IdentityModel.Tokens.Jwt;
using System.Security.Claims;
using System.Text;
using System.Text.Json;
using System.Text.RegularExpressions;
using System.Threading.RateLimiting;
using Microsoft.AspNetCore.Authentication.JwtBearer;
using Microsoft.AspNetCore.Identity;
using Microsoft.AspNetCore.RateLimiting;
using Microsoft.EntityFrameworkCore;
using Microsoft.IdentityModel.Tokens;
using Microsoft.OpenApi;
using Npgsql;
using PredictBoss.Domain.Core;
using PredictBoss.Domain.Infrastructure;

var builder = WebApplication.CreateBuilder(args);
var connection = builder.Configuration["DOMAIN_DATABASE_URL"]
    ?? throw new InvalidOperationException("DOMAIN_DATABASE_URL is required");
var jwtKey = builder.Configuration["DOMAIN_JWT_KEY"]
    ?? throw new InvalidOperationException("DOMAIN_JWT_KEY is required");
if (Encoding.UTF8.GetByteCount(jwtKey) < 32) throw new InvalidOperationException("DOMAIN_JWT_KEY must be at least 32 bytes");
var issuer = builder.Configuration["DOMAIN_JWT_ISSUER"] ?? "predictor-domain";
var audience = builder.Configuration["DOMAIN_JWT_AUDIENCE"] ?? "predictor-domain";
builder.Services.AddDbContext<DomainDbContext>(x => x.UseNpgsql(connection));
builder.Services.AddSingleton<EsbChatClient>();
builder.Services.AddScoped<IPasswordHasher<User>, PasswordHasher<User>>();
builder.Services.AddSingleton(new CompanyDataset(builder.Configuration["DOMAIN_CSV_PATH"]
    ?? throw new InvalidOperationException("DOMAIN_CSV_PATH is required")));
builder.Services.AddAuthentication(JwtBearerDefaults.AuthenticationScheme).AddJwtBearer(options =>
{
    options.MapInboundClaims = false;
    options.TokenValidationParameters = new TokenValidationParameters
    {
        ValidateIssuer = true, ValidIssuer = issuer,
        ValidateAudience = true, ValidAudience = audience,
        ValidateIssuerSigningKey = true,
        IssuerSigningKey = new SymmetricSecurityKey(Encoding.UTF8.GetBytes(jwtKey)),
        ValidateLifetime = true,
        ClockSkew = TimeSpan.FromMinutes(1)
    };
});
builder.Services.AddAuthorization();
builder.Services.AddRateLimiter(options =>
{
    options.RejectionStatusCode = StatusCodes.Status429TooManyRequests;
    options.AddPolicy("auth", context => RateLimitPartition.GetFixedWindowLimiter(
        context.Connection.RemoteIpAddress?.ToString() ?? "unknown",
        _ => new FixedWindowRateLimiterOptions
        {
            PermitLimit = 10,
            Window = TimeSpan.FromMinutes(1),
            QueueLimit = 0,
            AutoReplenishment = true
        }));
});
builder.Services.AddProblemDetails();
builder.Services.AddEndpointsApiExplorer();
builder.Services.AddSwaggerGen(options =>
{
    options.SwaggerDoc("v1", new OpenApiInfo
    {
        Title = "PredictBoss Domain API",
        Version = "v1",
        Description = "Профиль компании, источники и метаданные новостей. Текст новостей и анализ хранятся вне Domain."
    });
    options.AddSecurityDefinition("DomainJwt", new OpenApiSecurityScheme
    {
        Type = SecuritySchemeType.Http,
        Scheme = "bearer",
        BearerFormat = "JWT",
        Description = "JWT пользователя с sub, iss и aud. Введите токен без префикса Bearer."
    });
    options.OperationFilter<SwaggerSecurityOperationFilter>();
});

var app = builder.Build();
if (app.Environment.IsDevelopment() || builder.Configuration.GetValue<bool>("SWAGGER_ENABLED"))
{
    app.UseSwagger();
    app.UseSwaggerUI(options => options.SwaggerEndpoint("/swagger/v1/swagger.json", "Domain API v1"));
}
app.UseExceptionHandler();
app.UseRouting();
app.UseWebSockets();
app.UseRateLimiter();
app.UseAuthentication();
app.UseAuthorization();
using (var scope = app.Services.CreateScope())
    await scope.ServiceProvider.GetRequiredService<DomainDbContext>().Database.MigrateAsync();

app.MapGet("/health", () => Results.Ok(new { status = "ok" })).WithTags("Health");
var api = app.MapGroup("/api/v1").RequireAuthorization().WithTags("Domain");

api.MapPost("/companies", async (CreateCompanyRequest request, ClaimsPrincipal principal, CompanyDataset dataset,
    DomainDbContext db, HttpContext http, CancellationToken ct) =>
{
    var subject = principal.FindFirstValue("sub");
    if (string.IsNullOrWhiteSpace(subject)) return Error(401, "invalid_subject");
    var user = await db.Users.SingleOrDefaultAsync(x => x.ExternalSubject == subject, ct);
    if (user is null) return Error(401, "invalid_subject");
    if (await db.Companies.AnyAsync(x => x.OwnerUserId == user.Id, ct))
        return Error(409, "company_already_exists");

    Company company;
    if (request.Mode == "dataset")
    {
        if (string.IsNullOrWhiteSpace(request.SyntheticU)) return Error(400, "synthetic_u_required");
        if (dataset.IsAmbiguous(request.SyntheticU)) return Error(409, "ambiguous_synthetic_u");
        if (!dataset.TryGet(request.SyntheticU, out var row) || row is null) return Error(404, "synthetic_u_not_found");
        company = CompanyDataset.ToCompany(row);
        if (request.ProfileOverrides is { ValueKind: JsonValueKind.Object } overrides)
            try { CompanyProfiles.Apply(company, overrides); }
            catch (Exception e) when (e is ArgumentException or InvalidOperationException or FormatException)
            { return Error(400, "invalid_profile", e.Message); }
    }
    else if (request.Mode == "manual")
    {
        company = new Company();
        try { CompanyProfiles.Apply(company, request.Profile); }
        catch (Exception e) when (e is ArgumentException or InvalidOperationException or FormatException)
        { return Error(400, "invalid_profile", e.Message); }
    }
    else return Error(400, "invalid_mode");

    company.Owner = user;
    db.Companies.Add(company);
    await db.SaveChangesAsync(ct);
    http.Response.Headers.ETag = $"\"{company.ProfileRevision}\"";
    return Results.Created($"/api/v1/companies/{company.Id}", CompanyProfiles.ToResponse(company));
});

api.MapGet("/companies", async (ClaimsPrincipal principal, DomainDbContext db, CancellationToken ct) =>
{
    var subject = principal.FindFirstValue("sub");
    var companies = await db.Companies.AsNoTracking().Where(x => x.Owner.ExternalSubject == subject).ToListAsync(ct);
    return Results.Ok(new { items = companies.Select(CompanyProfiles.ToResponse), nextCursor = (string?)null });
});

api.MapGet("/companies/{companyId:guid}", async (Guid companyId, ClaimsPrincipal principal,
    DomainDbContext db, HttpContext http, CancellationToken ct) =>
{
    var c = await Owned(db, companyId, principal, ct);
    if (c is null) return Error(404, "company_not_found");
    http.Response.Headers.ETag = $"\"{c.ProfileRevision}\"";
    return Results.Ok(CompanyProfiles.ToResponse(c));
});

api.MapPatch("/companies/{companyId:guid}", async (Guid companyId, JsonElement profile,
    ClaimsPrincipal principal, DomainDbContext db, HttpContext http, CancellationToken ct) =>
{
    var c = await Owned(db, companyId, principal, ct);
    if (c is null) return Error(404, "company_not_found");
    if (http.Request.Headers.IfMatch != $"\"{c.ProfileRevision}\"") return Error(412, "revision_mismatch");
    try { CompanyProfiles.Apply(c, profile); }
    catch (Exception e) when (e is ArgumentException or InvalidOperationException or FormatException)
    { return Error(400, "invalid_profile", e.Message); }
    c.ProfileRevision++;
    c.ValeraDeliveryStatus = "pending";
    c.ValeraNextRetryAt = null;
    c.ValeraLeaseOwner = null;
    c.ValeraLeaseUntil = null;
    c.ValeraAttempts = 0;
    c.UpdatedAt = DateTime.UtcNow;
    try { await db.SaveChangesAsync(ct); }
    catch (DbUpdateConcurrencyException) { return Error(412, "revision_mismatch"); }
    http.Response.Headers.ETag = $"\"{c.ProfileRevision}\"";
    return Results.Ok(CompanyProfiles.ToResponse(c));
});

api.MapPost("/companies/{companyId:guid}/sources", async (Guid companyId, UrlRequest request,
    ClaimsPrincipal principal, DomainDbContext db, CancellationToken ct) =>
{
    if (await Owned(db, companyId, principal, ct) is null) return Error(404, "company_not_found");
    if (!Urls.TryCanonical(request.Url, out var canonical)) return Error(400, "invalid_url");
    var source = await db.NewsSources.SingleOrDefaultAsync(x => x.CompanyId == companyId && x.CanonicalUrl == canonical, ct);
    if (source is not null)
    {
        if (source.DeletedAt is not null)
        {
            source.DeletedAt = null;
            source.Enabled = true;
            source.ValidationStatus = "pending";
            source.Version++;
            source.UpdatedAt = DateTime.UtcNow;
            try { await db.SaveChangesAsync(ct); }
            catch (DbUpdateConcurrencyException) { return Error(412, "version_mismatch"); }
        }
        return Results.Ok(SourceResponse(source));
    }
    source = new NewsSource { CompanyId = companyId, Url = request.Url, CanonicalUrl = canonical };
    db.NewsSources.Add(source);
    try { await db.SaveChangesAsync(ct); }
    catch (DbUpdateConcurrencyException) { return Error(412, "version_mismatch"); }
    return Results.Created($"/api/v1/companies/{companyId}/sources/{source.Id}", SourceResponse(source));
});

api.MapGet("/companies/{companyId:guid}/sources", async (Guid companyId, ClaimsPrincipal principal,
    DomainDbContext db, CancellationToken ct) =>
{
    if (await Owned(db, companyId, principal, ct) is null) return Error(404, "company_not_found");
    var rows = await db.NewsSources.AsNoTracking().Where(x => x.CompanyId == companyId && x.DeletedAt == null)
        .OrderByDescending(x => x.CreatedAt).ThenByDescending(x => x.Id).Take(100).ToListAsync(ct);
    return Results.Ok(new { items = rows.Select(SourceResponse), nextCursor = (string?)null });
});

api.MapPatch("/companies/{companyId:guid}/sources/{sourceId:guid}", async (Guid companyId, Guid sourceId,
    SourcePatch request, ClaimsPrincipal principal, DomainDbContext db, HttpContext http, CancellationToken ct) =>
{
    if (await Owned(db, companyId, principal, ct) is null) return Error(404, "company_not_found");
    var s = await db.NewsSources.SingleOrDefaultAsync(x => x.CompanyId == companyId && x.Id == sourceId && x.DeletedAt == null, ct);
    if (s is null) return Error(404, "source_not_found");
    if (http.Request.Headers.IfMatch != $"\"{s.Version}\"") return Error(412, "version_mismatch");
    s.Enabled = request.Enabled;
    s.Version++;
    s.UpdatedAt = DateTime.UtcNow;
    try { await db.SaveChangesAsync(ct); }
    catch (DbUpdateConcurrencyException) { return Error(412, "version_mismatch"); }
    http.Response.Headers.ETag = $"\"{s.Version}\"";
    return Results.Ok(SourceResponse(s));
});

api.MapDelete("/companies/{companyId:guid}/sources/{sourceId:guid}", async (Guid companyId, Guid sourceId,
    ClaimsPrincipal principal, DomainDbContext db, CancellationToken ct) =>
{
    if (await Owned(db, companyId, principal, ct) is null) return Error(404, "company_not_found");
    var s = await db.NewsSources.SingleOrDefaultAsync(x => x.CompanyId == companyId && x.Id == sourceId, ct);
    if (s is null) return Error(404, "source_not_found");
    if (s.DeletedAt is null)
    {
        s.DeletedAt = DateTime.UtcNow;
        s.Enabled = false;
        s.Version++;
        await db.SaveChangesAsync(ct);
    }
    return Results.NoContent();
});

api.MapPost("/companies/{companyId:guid}/news-links", async (Guid companyId, OneTimeLinkRequest request,
    ClaimsPrincipal principal, DomainDbContext db, CancellationToken ct) =>
{
    if (await Owned(db, companyId, principal, ct) is null) return Error(404, "company_not_found");
    if (!Urls.TryCanonical(request.Url, out var canonical)) return Error(400, "invalid_url");
    if (request.MaterialKind is not ("external_news" or "alpha_analysis")) return Error(400, "invalid_material_kind");
    if (request.MaterialKind == "alpha_analysis" && !Urls.IsAlfa(canonical)) return Error(400, "alpha_domain_required");
    var link = await db.NewsLinks.SingleOrDefaultAsync(x => x.CompanyId == companyId && x.CanonicalUrl == canonical, ct);
    if (link is not null)
    {
        if (link.MaterialKind != request.MaterialKind) return Error(409, "material_kind_conflict");
        return Results.Ok(LinkResponse(link));
    }
    link = new NewsLink { CompanyId = companyId, Url = request.Url, CanonicalUrl = canonical, MaterialKind = request.MaterialKind };
    db.NewsLinks.Add(link);
    await db.SaveChangesAsync(ct);
    return Results.Created($"/api/v1/companies/{companyId}/news/{link.Id}", LinkResponse(link));
});

api.MapGet("/companies/{companyId:guid}/news", async (Guid companyId, ClaimsPrincipal principal,
    string? from, string? to, int? limit, DomainDbContext db, CancellationToken ct) =>
{
    if (await Owned(db, companyId, principal, ct) is null) return Error(404, "company_not_found");
    var pageSize = limit ?? 100;
    if (pageSize is < 1 or > 100) return Error(400, "invalid_limit");
    DateTime? fromDate = ParseDateBoundary(from, endOfDay: false);
    DateTime? toDate = ParseDateBoundary(to, endOfDay: true);
    if ((from is not null && fromDate is null) || (to is not null && toDate is null)
        || (fromDate is not null && toDate is not null && fromDate > toDate))
        return Error(400, "invalid_date_range");
    var query = db.NewsLinks.AsNoTracking().Where(x => x.CompanyId == companyId);
    if (fromDate is not null) query = query.Where(x => (x.PublishedAt ?? x.DiscoveredAt) >= fromDate.Value);
    if (toDate is not null) query = query.Where(x => (x.PublishedAt ?? x.DiscoveredAt) <= toDate.Value);
    var rows = await query.OrderByDescending(x => x.PublishedAt ?? x.DiscoveredAt).ThenByDescending(x => x.Id)
        .Take(pageSize + 1).ToListAsync(ct);
    var hasMore = rows.Count > pageSize;
    if (hasMore) rows.RemoveAt(rows.Count - 1);
    return Results.Ok(new { items = rows.Select(LinkResponse), hasMore });
});

api.MapGet("/companies/{companyId:guid}/news/{articleId:guid}", async (Guid companyId, Guid articleId,
    ClaimsPrincipal principal, DomainDbContext db, CancellationToken ct) =>
{
    if (await Owned(db, companyId, principal, ct) is null) return Error(404, "company_not_found");
    var link = await db.NewsLinks.AsNoTracking().SingleOrDefaultAsync(x => x.CompanyId == companyId && x.Id == articleId, ct);
    return link is null ? Error(404, "article_not_found") : Results.Ok(LinkResponse(link));
});

api.MapGet("/chat", async (ClaimsPrincipal principal, DomainDbContext db, EsbChatClient esb, CancellationToken ct) =>
{
    var company = await OwnedCompany(db, principal, ct);
    if (company is null) return Results.Ok(EmptyChat(hasProfile: false));
    try
    {
        var snapshot = await esb.GetOrStartAnalysis(company.Id, waitForProfile: false, ct);
        return Results.Ok(await ChatResponse(db, company, snapshot, ct));
    }
    catch (EsbChatException e) when (e.Code is "profile_update_pending" or "company_profile_not_found")
    {
        var hasSources = await db.NewsSources.AnyAsync(x => x.CompanyId == company.Id && x.DeletedAt == null, ct);
        return Results.Ok(EmptyChat(hasProfile: true, hasSources, processing: true));
    }
    catch (EsbChatException e) { return Error(e.StatusCode, e.Code); }
});

api.MapPost("/chat/analysis", async (ClaimsPrincipal principal, DomainDbContext db, EsbChatClient esb, CancellationToken ct) =>
{
    var company = await OwnedCompany(db, principal, ct);
    if (company is null) return Error(409, "company_profile_required");
    try
    {
        var snapshot = await esb.GetOrStartAnalysis(company.Id, waitForProfile: true, ct);
        return Results.Ok(await ChatResponse(db, company, snapshot, ct));
    }
    catch (EsbChatException e) { return Error(e.StatusCode, e.Code); }
});

api.MapPost("/chat/messages", async (ChatMessageRequest request, ClaimsPrincipal principal,
    DomainDbContext db, EsbChatClient esb, CancellationToken ct) =>
{
    var text = request.Text?.Trim();
    if (string.IsNullOrWhiteSpace(text) || Encoding.UTF8.GetByteCount(text) > 1_000_000)
        return Error(400, "invalid_message");
    var company = await OwnedCompany(db, principal, ct);
    if (company is null) return Error(409, "company_profile_required");
    try
    {
        var isAnalysisCommand = Regex.IsMatch(text, @"\b(запусти|начни|обнови|проведи)\s+анализ\b|\bпроанализируй\b", RegexOptions.IgnoreCase | RegexOptions.CultureInvariant);
        var snapshot = await esb.GetOrStartAnalysis(company.Id, waitForProfile: isAnalysisCommand, ct);
        if (isAnalysisCommand) return Results.Ok(await ChatResponse(db, company, snapshot, ct));
        if (ReadString(snapshot, "status") != "ready") return Error(409, "analysis_not_ready");

        var conversationId = ReadString(snapshot, "conversationId");
        var analysisId = ReadString(snapshot, "analysisId");
        if (!Guid.TryParse(conversationId, out _) || !Guid.TryParse(analysisId, out _))
            return Error(502, "invalid_esb_chat_response");
        await esb.Ask(company.Id, conversationId!, analysisId!, text, ct);
        var updated = await esb.GetOrStartAnalysis(company.Id, waitForProfile: false, ct);
        return Results.Ok(await ChatResponse(db, company, updated, ct));
    }
    catch (EsbChatException e) { return Error(e.StatusCode, e.Code); }
});

app.MapPost("/auth/register", async (RegisterRequest request, DomainDbContext db,
    IPasswordHasher<User> passwordHasher, IConfiguration config, CancellationToken ct) =>
{
    var login = request.Login?.Trim();
    var normalizedLogin = NormalizeLogin(login);
    if (normalizedLogin is null)
        return Error(400, "invalid_login", "Логин должен содержать 3–128 символов без пробелов.");
    if (string.IsNullOrEmpty(request.Password) || request.Password.Length < 12 || request.Password.Length > 256)
        return Error(400, "invalid_password", "Пароль должен содержать от 12 до 256 символов.");
    if (await db.Users.AnyAsync(x => x.NormalizedLogin == normalizedLogin, ct))
        return Error(409, "login_already_exists");

    var userId = Guid.NewGuid();
    var user = new User
    {
        Id = userId,
        ExternalSubject = userId.ToString("D"),
        Login = login,
        NormalizedLogin = normalizedLogin,
        DisplayName = string.IsNullOrWhiteSpace(request.DisplayName) ? null : request.DisplayName.Trim()
    };
    user.PasswordHash = passwordHasher.HashPassword(user, request.Password);
    db.Users.Add(user);
    try { await db.SaveChangesAsync(ct); }
    catch (DbUpdateException e) when (e.InnerException is PostgresException
        { SqlState: PostgresErrorCodes.UniqueViolation })
    { return Error(409, "login_already_exists"); }

    return Results.Ok(CreateTokenResponse(user, config));
}).WithTags("Auth").AllowAnonymous().RequireRateLimiting("auth");

app.MapPost("/auth/login", async (LoginRequest request, DomainDbContext db,
    IPasswordHasher<User> passwordHasher, IConfiguration config, CancellationToken ct) =>
{
    var normalizedLogin = NormalizeLogin(request.Login?.Trim());
    if (normalizedLogin is null || string.IsNullOrEmpty(request.Password))
        return Results.Unauthorized();

    var user = await db.Users.SingleOrDefaultAsync(x => x.NormalizedLogin == normalizedLogin, ct);
    if (user?.PasswordHash is null) return Results.Unauthorized();
    var verification = passwordHasher.VerifyHashedPassword(user, user.PasswordHash, request.Password);
    if (verification == PasswordVerificationResult.Failed)
        return Results.Unauthorized();

    if (verification == PasswordVerificationResult.SuccessRehashNeeded)
    {
        user.PasswordHash = passwordHasher.HashPassword(user, request.Password);
        await db.SaveChangesAsync(ct);
    }

    return Results.Ok(CreateTokenResponse(user, config));
}).WithTags("Auth").AllowAnonymous().RequireRateLimiting("auth");

app.Run();

static async Task<Company?> Owned(DomainDbContext db, Guid id, ClaimsPrincipal principal, CancellationToken ct)
    => await db.Companies.SingleOrDefaultAsync(x => x.Id == id && x.Owner.ExternalSubject == principal.FindFirstValue("sub"), ct);
static Task<Company?> OwnedCompany(DomainDbContext db, ClaimsPrincipal principal, CancellationToken ct)
    => db.Companies.SingleOrDefaultAsync(x => x.Owner.ExternalSubject == principal.FindFirstValue("sub"), ct);
static IResult Error(int status, string code, string? message = null)
    => Results.Json(new { code, message = message ?? code }, statusCode: status);
static object SourceResponse(NewsSource s) => new
{
    sourceId = s.Id, s.Url, s.Name, s.Kind, s.ValidationStatus, s.Enabled,
    s.PollIntervalMinutes, s.LastSuccessAt, s.LastError, s.Version
};
static object LinkResponse(NewsLink l) => new
{
    articleId = l.Id, l.CanonicalUrl, l.MaterialKind, l.MediaType, l.Title,
    l.PublishedAt, l.NewsSourceId, l.DiscoveredAt,
    processingStatus = l.ExtractionStatus is "unsupported" or "failed" ? l.ExtractionStatus : l.SendStatus
};

static string? NormalizeLogin(string? login)
    => login is { Length: >= 3 and <= 128 } && !login.Any(char.IsWhiteSpace)
        ? login.ToUpperInvariant()
        : null;

static object CreateTokenResponse(User user, IConfiguration config)
{
    var jwtKey = config["DOMAIN_JWT_KEY"] ?? throw new InvalidOperationException("DOMAIN_JWT_KEY is required");
    var issuer = config["DOMAIN_JWT_ISSUER"] ?? "predictor-domain";
    var audience = config["DOMAIN_JWT_AUDIENCE"] ?? "predictor-domain";
    var expiresAt = DateTime.UtcNow.AddHours(1);
    var key = new SymmetricSecurityKey(Encoding.UTF8.GetBytes(jwtKey));
    var credentials = new SigningCredentials(key, SecurityAlgorithms.HmacSha256);
    var claims = new[]
    {
        new Claim(ClaimTypes.NameIdentifier, user.ExternalSubject),
        new Claim("sub", user.ExternalSubject),
        new Claim("login", user.Login ?? string.Empty)
    };
    var token = new JwtSecurityToken(issuer, audience, claims, DateTime.UtcNow, expiresAt, credentials);
    return new
    {
        access_token = new JwtSecurityTokenHandler().WriteToken(token),
        token_type = "Bearer",
        expires_in = 3600,
        user = new { userId = user.Id, login = user.Login, displayName = user.DisplayName }
    };
}

static DateTime? ParseDateBoundary(string? raw, bool endOfDay)
{
    if (raw is null) return null;
    if (!DateOnly.TryParseExact(raw, "yyyy-MM-dd", out var date)) return null;
    var value = date.ToDateTime(TimeOnly.MinValue, DateTimeKind.Utc);
    return endOfDay ? value.AddDays(1).AddTicks(-1) : value;
}

static object EmptyChat(bool hasProfile, bool hasSources = false, bool processing = false)
    => new { lastAnalysisAt = (DateTime?)null, hasSources, hasProfile, processing, messages = Array.Empty<object>() };

static async Task<object> ChatResponse(DomainDbContext db, Company company, JsonElement snapshot, CancellationToken ct)
{
    var history = snapshot.TryGetProperty("history", out var items) && items.ValueKind == JsonValueKind.Array
        ? items.EnumerateArray().ToList()
        : [];
    var currentMessageId = ReadString(snapshot, "messageId");
    if (ReadString(snapshot, "status") == "ready" && currentMessageId is not null
        && history.All(x => ReadString(x, "messageId") != currentMessageId))
        history.Add(snapshot);

    var sourceIds = history.SelectMany(x => ReadEvidence(x).Select(e => e.SourceId))
        .Where(x => Guid.TryParse(x, out _)).Select(Guid.Parse).Distinct().ToArray();
    var links = await db.NewsLinks.AsNoTracking().Where(x => x.CompanyId == company.Id && sourceIds.Contains(x.Id))
        .ToDictionaryAsync(x => x.Id, ct);
    var messages = new List<object>();
    DateTime? lastAnalysisAt = null;
    foreach (var item in history)
    {
        var role = ReadString(item, "role");
        var kind = ReadString(item, "kind") ?? "";
        var id = ReadString(item, "messageId") ?? Guid.NewGuid().ToString("D");
        var text = ReadString(item, "text") ?? "";
        var createdAt = ReadString(item, "createdAt") ?? DateTime.UtcNow.ToString("O");
        var evidence = ReadEvidence(item).Select((entry, index) =>
        {
            links.TryGetValue(Guid.TryParse(entry.SourceId, out var sourceId) ? sourceId : Guid.Empty, out var link);
            return new
            {
                index = index + 1,
                sourceId = entry.SourceId,
                title = link?.Title ?? link?.CanonicalUrl ?? entry.SourceId,
                url = link?.CanonicalUrl ?? "",
                note = entry.OriginKind == "ai_inference" ? "AI-вывод" : (string?)null
            };
        }).ToArray();

        if (role == "user")
        {
            messages.Add(new { id, role = "user", createdAt, text });
            continue;
        }

        var analysis = kind == "analysis_result"
            ? new { title = "Анализ новостей", subtitle = $"Результат от {createdAt}", text, evidence }
            : null;
        if (analysis is not null && DateTime.TryParse(createdAt, out var parsed)) lastAnalysisAt = parsed;
        messages.Add(new { id, role = "analyst", createdAt, text = analysis is null ? text : (string?)null, analysis, evidence, note = (string?)null });
    }

    var hasSources = await db.NewsSources.AnyAsync(x => x.CompanyId == company.Id && x.DeletedAt == null, ct);
    return new
    {
        lastAnalysisAt,
        hasSources,
        hasProfile = true,
        processing = ReadString(snapshot, "status") == "processing",
        messages
    };
}

static string? ReadString(JsonElement element, string name)
    => element.ValueKind == JsonValueKind.Object && element.TryGetProperty(name, out var value)
        && value.ValueKind == JsonValueKind.String ? value.GetString() : null;

static List<EsbEvidence> ReadEvidence(JsonElement element)
{
    if (!element.TryGetProperty("evidence", out var evidence) || evidence.ValueKind != JsonValueKind.Array) return [];
    return evidence.EnumerateArray().Select(x => new EsbEvidence(
        ReadString(x, "sourceId") ?? "", ReadString(x, "originKind") ?? "")).Where(x => x.SourceId != "").ToList();
}

public static class Urls
{
    public static bool TryCanonical(string? raw, out string canonical)
    {
        canonical = "";
        if (!Uri.TryCreate(raw, UriKind.Absolute, out var uri) || uri.Scheme is not ("https" or "http")
            || string.IsNullOrWhiteSpace(uri.Host) || !string.IsNullOrEmpty(uri.UserInfo)
            || uri.Host.Equals("localhost", StringComparison.OrdinalIgnoreCase)
            || System.Net.IPAddress.TryParse(uri.Host, out _)) return false;
        canonical = new UriBuilder(uri) { Fragment = "", Host = uri.Host.ToLowerInvariant() }.Uri.AbsoluteUri.TrimEnd('/');
        return true;
    }

    public static bool IsAlfa(string canonical)
    {
        var host = new Uri(canonical).Host;
        return host.Equals("alfabank.ru", StringComparison.OrdinalIgnoreCase)
            || host.EndsWith(".alfabank.ru", StringComparison.OrdinalIgnoreCase);
    }
}

public sealed record CreateCompanyRequest(string Mode, string? SyntheticU, JsonElement Profile, JsonElement? ProfileOverrides);
public sealed record UrlRequest(string Url);
public sealed record OneTimeLinkRequest(string Url, string MaterialKind);
public sealed record SourcePatch(bool Enabled);
public sealed record RegisterRequest(string Login, string Password, string? DisplayName);
public sealed record LoginRequest(string Login, string Password);
public sealed record ChatMessageRequest(string Text);
public sealed record EsbEvidence(string SourceId, string OriginKind);
