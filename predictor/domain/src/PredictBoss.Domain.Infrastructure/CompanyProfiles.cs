using System.Globalization;
using System.Text.Json;
using CsvHelper;
using CsvHelper.Configuration;
using PredictBoss.Domain.Core;

namespace PredictBoss.Domain.Infrastructure;

public sealed class CompanyDataset
{
    private static readonly string[] Headers = [
        "SYNTHETIC_U", "ORG_TYPE", "OKFS_NAME", "COMPANY_AGE_YEARS_GRADATION", "ECO_MARK_LABEL",
        "MAINOKVED_CODE", "MAINOKVED_NAME", "NEGATIVEMARK", "AMOUNT_WORKERS_FNS_GRADATION",
        "AMOUNT_WORKERS_SPARK_GRADATION", "CL_TYPE", "CL_PRODUCT_RKO_MARK", "CL_PRODUCT_GAR_MARK",
        "CL_PRODUCT_LOA_MARK", "CL_PRODUCT_LEA_MARK", "CL_PRODUCT_DEP_MARK", "CL_PRODUCT_ZPP_MARK",
        "FTS_FLAG", "FTS_COUNTRY_NAME", "FACTORING_MARK", "REVENUE_COMPANY", "GROUP_ID", "CITY", "REGION"
    ];
    private readonly Dictionary<string, string[]> rows = new(StringComparer.Ordinal);
    private readonly HashSet<string> ambiguous = new(StringComparer.Ordinal);

    public CompanyDataset(string path)
    {
        if (!File.Exists(path)) throw new FileNotFoundException("Company CSV is required", path);
        using var input = new StreamReader(path);
        using var csv = new CsvReader(input, new CsvConfiguration(CultureInfo.InvariantCulture) { Delimiter = ";" });
        if (!csv.Read() || !csv.ReadHeader() || !Headers.SequenceEqual(csv.HeaderRecord))
            throw new InvalidDataException("Company CSV header does not match the 24-field contract");
        while (csv.Read())
        {
            var row = Headers.Select(h => csv.GetField(h) ?? "").ToArray();
            var key = row[0];
            if (string.IsNullOrWhiteSpace(key)) continue;
            if (rows.ContainsKey(key)) { rows.Remove(key); ambiguous.Add(key); }
            else if (!ambiguous.Contains(key)) rows.Add(key, row);
        }
    }

    public bool IsAmbiguous(string syntheticU) => ambiguous.Contains(syntheticU);
    public bool TryGet(string syntheticU, out string[]? row) => rows.TryGetValue(syntheticU, out row);

    public static Company ToCompany(string[] row)
    {
        string? S(int i) => string.IsNullOrWhiteSpace(row[i]) ? null : row[i];
        bool? B(int i) => S(i) switch { "0" or "0.0" => false, "1" or "1.0" => true, _ => null };
        decimal? revenue = null;
        if (decimal.TryParse(S(20), NumberStyles.Number, CultureInfo.InvariantCulture, out var parsed)
            && parsed >= 0 && parsed == decimal.Truncate(parsed)) revenue = parsed;
        return new Company
        {
            SyntheticU = S(0), OrgType = S(1), OwnershipForm = S(2), CompanyAgeGradation = S(3),
            DevelopmentStage = S(4), MainOkvedCode = S(5), MainOkvedName = S(6), NegativeMark = B(7),
            WorkersFnsGradation = S(8), WorkersSparkGradation = S(9), ClientType = S(10),
            HasRko = B(11), HasGuarantees = B(12), HasLoans = B(13), HasLeasing = B(14),
            HasDeposits = B(15), HasPayroll = B(16), FtsFlag = B(17), FtsCountryName = S(18),
            HasFactoring = B(19), RevenueCompany = revenue, GroupId = S(21), City = S(22), Region = S(23),
            ImportPayload = JsonSerializer.Serialize(Headers.Zip(row).ToDictionary(x => x.First, x => x.Second)),
            ImportWarnings = JsonSerializer.Serialize(Enumerable.Range(0, row.Length)
                .Where(i => (new[] { 7, 11, 12, 13, 14, 15, 16, 17, 19 }.Contains(i) && S(i) is not null && B(i) is null)
                    || (i == 20 && S(i) is not null && revenue is null))
                .Select(i => $"Invalid value for {Headers[i]}: {row[i]}").ToArray())
        };
    }
}

public static class CompanyProfiles
{
    public static void Apply(Company company, JsonElement profile)
    {
        if (profile.ValueKind != JsonValueKind.Object) throw new ArgumentException("profile must be an object");
        foreach (var field in profile.EnumerateObject())
        {
            var v = field.Value;
            string? S() => v.ValueKind == JsonValueKind.Null ? null : v.GetString();
            bool? B() => v.ValueKind == JsonValueKind.Null ? null : v.GetBoolean();
            switch (field.Name)
            {
                case "orgType": company.OrgType = S(); break;
                case "ownershipForm": company.OwnershipForm = S(); break;
                case "companyAgeGradation": company.CompanyAgeGradation = S(); break;
                case "developmentStage": company.DevelopmentStage = S(); break;
                case "mainOkvedCode": company.MainOkvedCode = S(); break;
                case "mainOkvedName": company.MainOkvedName = S(); break;
                case "negativeMark": company.NegativeMark = B(); break;
                case "workersFnsGradation": company.WorkersFnsGradation = S(); break;
                case "workersSparkGradation": company.WorkersSparkGradation = S(); break;
                case "clientType": company.ClientType = S(); break;
                case "hasRko": company.HasRko = B(); break;
                case "hasGuarantees": company.HasGuarantees = B(); break;
                case "hasLoans": company.HasLoans = B(); break;
                case "hasLeasing": company.HasLeasing = B(); break;
                case "hasDeposits": company.HasDeposits = B(); break;
                case "hasPayroll": company.HasPayroll = B(); break;
                case "ftsFlag": company.FtsFlag = B(); break;
                case "ftsCountryName": company.FtsCountryName = S(); break;
                case "hasFactoring": company.HasFactoring = B(); break;
                case "revenueCompany": company.RevenueCompany = v.ValueKind == JsonValueKind.Null ? null : v.GetDecimal(); break;
                case "groupId": company.GroupId = S(); break;
                case "city": company.City = S(); break;
                case "region": company.Region = S(); break;
                default: throw new ArgumentException($"Unknown or immutable profile field: {field.Name}");
            }
        }
        if (company.RevenueCompany < 0 || company.RevenueCompany != decimal.Truncate(company.RevenueCompany ?? 0))
            throw new ArgumentException("revenueCompany must be a non-negative whole number of rubles");
        if (company.ClientType is not null and not ("PROSPECT" or "CLIENT" or "CLOSE_CLIENT"))
            throw new ArgumentException("Unsupported clientType");
        if (!HasContent(company)) throw new ArgumentException("Company profile is empty");
    }

    public static bool HasContent(Company c) =>
        c.OrgType is not null || c.OwnershipForm is not null || c.MainOkvedCode is not null ||
        c.MainOkvedName is not null || c.City is not null || c.Region is not null ||
        c.RevenueCompany is not null || c.DevelopmentStage is not null || c.ClientType is not null ||
        c.CompanyAgeGradation is not null || c.NegativeMark is not null ||
        c.WorkersFnsGradation is not null || c.WorkersSparkGradation is not null ||
        c.HasRko is not null || c.HasGuarantees is not null || c.HasLoans is not null ||
        c.HasLeasing is not null || c.HasDeposits is not null || c.HasPayroll is not null ||
        c.FtsFlag is not null || c.FtsCountryName is not null || c.HasFactoring is not null ||
        c.GroupId is not null;

    public static object ToResponse(Company c) => new
    {
        companyId = c.Id, c.ProfileRevision, c.ValeraDeliveryStatus,
        profile = ToProfile(c)
    };

    public static object ToProfile(Company c) => new
        {
            c.SyntheticU, c.OrgType, c.OwnershipForm, c.CompanyAgeGradation, c.DevelopmentStage,
            c.MainOkvedCode, c.MainOkvedName, c.NegativeMark, c.WorkersFnsGradation,
            c.WorkersSparkGradation, c.ClientType, c.HasRko, c.HasGuarantees, c.HasLoans,
            c.HasLeasing, c.HasDeposits, c.HasPayroll, c.FtsFlag, c.FtsCountryName,
            c.HasFactoring, c.RevenueCompany, c.GroupId, c.City, c.Region
        };
}
