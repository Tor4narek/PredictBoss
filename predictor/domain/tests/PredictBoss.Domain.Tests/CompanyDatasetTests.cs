using PredictBoss.Domain.Infrastructure;

namespace PredictBoss.Domain.Tests;

public sealed class CompanyDatasetTests
{
    private static readonly Lazy<CompanyDataset> Dataset = new(() => new CompanyDataset(Path.GetFullPath(
        Path.Combine(AppContext.BaseDirectory, "../../../../../data/df_Uid_data_hack.csv"))));

    [Fact]
    public void ImportsTheActualAlfaRowWithoutLosingMoneyOrUnknownValues()
    {
        Assert.True(Dataset.Value.TryGet("U_84", out var row));
        var company = CompanyDataset.ToCompany(row!);
        Assert.Equal(1331906317m, company.RevenueCompany);
        Assert.Equal("46.90", company.MainOkvedCode);
        Assert.Null(company.WorkersSparkGradation);
        Assert.Null(company.FtsCountryName);
        Assert.False(company.FtsFlag);
        Assert.True(company.HasLoans);
    }

    [Fact]
    public void RejectsAmbiguousSyntheticIdentifier()
    {
        Assert.True(Dataset.Value.IsAmbiguous("U_428697"));
        Assert.False(Dataset.Value.TryGet("U_428697", out _));
    }
}
