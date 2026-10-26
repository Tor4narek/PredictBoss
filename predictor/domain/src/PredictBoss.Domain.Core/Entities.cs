namespace PredictBoss.Domain.Core;

public sealed class User
{
    public Guid Id { get; set; } = Guid.NewGuid();
    public required string ExternalSubject { get; set; }
    public string? DisplayName { get; set; }
    public string? Login { get; set; }
    public string? NormalizedLogin { get; set; }
    public string? PasswordHash { get; set; }
    public DateTime CreatedAt { get; set; } = DateTime.UtcNow;
    public Company? Company { get; set; }
}

public sealed class Company
{
    public Guid Id { get; set; } = Guid.NewGuid();
    public Guid OwnerUserId { get; set; }
    public User Owner { get; set; } = null!;
    public string? SyntheticU { get; set; }
    public string? OrgType { get; set; }
    public string? OwnershipForm { get; set; }
    public string? CompanyAgeGradation { get; set; }
    public string? DevelopmentStage { get; set; }
    public string? MainOkvedCode { get; set; }
    public string? MainOkvedName { get; set; }
    public bool? NegativeMark { get; set; }
    public string? WorkersFnsGradation { get; set; }
    public string? WorkersSparkGradation { get; set; }
    public string? ClientType { get; set; }
    public bool? HasRko { get; set; }
    public bool? HasGuarantees { get; set; }
    public bool? HasLoans { get; set; }
    public bool? HasLeasing { get; set; }
    public bool? HasDeposits { get; set; }
    public bool? HasPayroll { get; set; }
    public bool? FtsFlag { get; set; }
    public string? FtsCountryName { get; set; }
    public bool? HasFactoring { get; set; }
    public decimal? RevenueCompany { get; set; }
    public string? GroupId { get; set; }
    public string? City { get; set; }
    public string? Region { get; set; }
    public string? ImportPayload { get; set; }
    public string ImportWarnings { get; set; } = "[]";
    public long ProfileRevision { get; set; } = 1;
    public string ValeraDeliveryStatus { get; set; } = "pending";
    public long? ValeraLastSentRevision { get; set; }
    public int ValeraAttempts { get; set; }
    public DateTime? ValeraNextRetryAt { get; set; }
    public string? ValeraLeaseOwner { get; set; }
    public DateTime? ValeraLeaseUntil { get; set; }
    public string? ValeraLastError { get; set; }
    public DateTime CreatedAt { get; set; } = DateTime.UtcNow;
    public DateTime UpdatedAt { get; set; } = DateTime.UtcNow;
    public List<NewsSource> Sources { get; set; } = [];
    public List<NewsLink> Links { get; set; } = [];
}

public sealed class NewsSource
{
    public Guid Id { get; set; } = Guid.NewGuid();
    public Guid CompanyId { get; set; }
    public Company Company { get; set; } = null!;
    public required string Url { get; set; }
    public required string CanonicalUrl { get; set; }
    public string? Name { get; set; }
    public string? Kind { get; set; }
    public string ValidationStatus { get; set; } = "pending";
    public bool Enabled { get; set; } = true;
    public long Version { get; set; } = 1;
    public DateTime? DeletedAt { get; set; }
    public int PollIntervalMinutes { get; set; } = 60;
    public DateTime? NextPollAt { get; set; }
    public DateTime? LastSuccessAt { get; set; }
    public string? LeaseOwner { get; set; }
    public DateTime? LeaseUntil { get; set; }
    public string? LastError { get; set; }
    public DateTime CreatedAt { get; set; } = DateTime.UtcNow;
    public DateTime UpdatedAt { get; set; } = DateTime.UtcNow;
    public List<NewsLink> Links { get; set; } = [];
}

public sealed class NewsLink
{
    public Guid Id { get; set; } = Guid.NewGuid();
    public Guid CompanyId { get; set; }
    public Company Company { get; set; } = null!;
    public Guid? NewsSourceId { get; set; }
    public NewsSource? NewsSource { get; set; }
    public required string Url { get; set; }
    public required string CanonicalUrl { get; set; }
    public string MaterialKind { get; set; } = "external_news";
    public string IngestMode { get; set; } = "one_time";
    public string? MediaType { get; set; }
    public string? Title { get; set; }
    public DateTime? PublishedAt { get; set; }
    public DateTime DiscoveredAt { get; set; } = DateTime.UtcNow;
    public DateTime LastSeenAt { get; set; } = DateTime.UtcNow;
    public string? ContentHash { get; set; }
    public string ExtractionStatus { get; set; } = "pending";
    public string SendStatus { get; set; } = "pending";
    public int SendAttempts { get; set; }
    public DateTime? NextRetryAt { get; set; }
    public string? LeaseOwner { get; set; }
    public DateTime? LeaseUntil { get; set; }
    public string? LastError { get; set; }
}
