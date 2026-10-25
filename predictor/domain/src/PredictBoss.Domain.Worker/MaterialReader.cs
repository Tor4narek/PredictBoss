using System.Net;
using System.Text;
using System.Text.RegularExpressions;
using System.Xml.Linq;
using AngleSharp.Html.Parser;
using Microsoft.Playwright;
using UglyToad.PdfPig;

namespace PredictBoss.Domain.Worker;

public sealed class MaterialReader : IAsyncDisposable
{
    private const int MaxBytes = 10 * 1024 * 1024;
    private readonly HttpClient http = new(new HttpClientHandler { AllowAutoRedirect = false })
    {
        Timeout = TimeSpan.FromSeconds(25)
    };
    private readonly SemaphoreSlim browserGate = new(1, 1);
    private IPlaywright? playwright;
    private IBrowser? browser;

    public async Task<SourceResult> Discover(string url, CancellationToken ct)
    {
        var (bytes, mediaType, finalUrl) = await Fetch(url, ct);
        var document = Encoding.UTF8.GetString(bytes);
        if (mediaType.Contains("xml") || document.TrimStart().StartsWith("<?xml", StringComparison.Ordinal))
        {
            var xml = XDocument.Parse(document);
            var root = xml.Root?.Name.LocalName;
            IEnumerable<string> links = root switch
            {
                "rss" => xml.Descendants().Where(x => x.Name.LocalName == "item")
                    .SelectMany(x => x.Elements().Where(e => e.Name.LocalName == "link")).Select(x => x.Value),
                "feed" => xml.Descendants().Where(x => x.Name.LocalName == "entry")
                    .SelectMany(x => x.Elements().Where(e => e.Name.LocalName == "link"))
                    .Select(x => x.Attribute("href")?.Value ?? x.Value),
                "urlset" => xml.Descendants().Where(x => x.Name.LocalName == "url")
                    .SelectMany(x => x.Elements().Where(e => e.Name.LocalName == "loc")).Select(x => x.Value),
                _ => []
            };
            if (root is not ("rss" or "feed" or "urlset")) throw new NotSupportedException("Unsupported XML source");
            return new SourceResult(root switch { "rss" => "rss", "feed" => "atom", _ => "sitemap" },
                links.Select(x => Resolve(finalUrl, x)).Where(x => x is not null).Cast<string>()
                    .Distinct(StringComparer.OrdinalIgnoreCase).Take(100).ToArray());
        }
        if (!mediaType.Contains("html")) throw new NotSupportedException("Only RSS, Atom, sitemap and HTML indexes are supported");
        var parser = new HtmlParser();
        var html = await parser.ParseDocumentAsync(document, ct);
        if (html.QuerySelectorAll("a[href]").Length < 3)
            html = await parser.ParseDocumentAsync(await RenderWithBrowser(finalUrl, ct), ct);
        var sourceHost = new Uri(finalUrl).Host;
        var linksFromPage = html.QuerySelectorAll("a[href]")
            .Select(x => new { Url = Resolve(finalUrl, x.GetAttribute("href")), Text = Clean(x.TextContent) })
            .Where(x => x.Url is not null)
            .Where(x => new Uri(x.Url!).Host.Equals(sourceHost, StringComparison.OrdinalIgnoreCase))
            .Where(x => !x.Url!.Equals(finalUrl, StringComparison.OrdinalIgnoreCase))
            .Where(x => Regex.IsMatch(new Uri(x.Url!).AbsolutePath, @"/(news|article|press|media|blog|analysis|20\d\d)/", RegexOptions.IgnoreCase)
                || (x.Text.Length >= 30 && new Uri(x.Url!).Segments.Length >= 3))
            .Select(x => x.Url!)
            .Distinct(StringComparer.OrdinalIgnoreCase).Take(100).ToArray();
        return new SourceResult("html_index", linksFromPage);
    }

    public async Task<MaterialResult> ReadMaterial(string url, CancellationToken ct)
    {
        var (bytes, mediaType, finalUrl) = await Fetch(url, ct);
        if (mediaType.Contains("pdf") || finalUrl.EndsWith(".pdf", StringComparison.OrdinalIgnoreCase))
        {
            using var pdf = PdfDocument.Open(bytes);
            var text = Clean(string.Join(' ', pdf.GetPages().Select(p => p.Text)));
            if (text.Length < 100) throw new NotSupportedException("PDF has no extractable text layer");
            return new MaterialResult(text, "application/pdf", null);
        }
        if (!mediaType.Contains("html")) throw new NotSupportedException("Unsupported material type");
        var html = Encoding.UTF8.GetString(bytes);
        var extracted = await ExtractHtml(html, ct);
        if (extracted.Text.Length < 200)
        {
            html = await RenderWithBrowser(finalUrl, ct);
            extracted = await ExtractHtml(html, ct);
        }
        if (extracted.Text.Length < 100) throw new NotSupportedException("No article text found");
        return new MaterialResult(extracted.Text, "text/html", extracted.Title);
    }

    private static async Task<(string Text, string? Title)> ExtractHtml(string html, CancellationToken ct)
    {
        var doc = await new HtmlParser().ParseDocumentAsync(html, ct);
        foreach (var node in doc.QuerySelectorAll("script,style,nav,header,footer,aside,form,svg")) node.Remove();
        var title = Clean(doc.QuerySelector("title")?.TextContent ?? "");
        var body = doc.QuerySelector("article") ?? doc.QuerySelector("main") ?? doc.Body;
        return (Clean(body?.TextContent ?? ""), title.Length == 0 ? null : title);
    }

    private async Task<string> RenderWithBrowser(string url, CancellationToken ct)
    {
        await browserGate.WaitAsync(ct);
        try
        {
            playwright ??= await Playwright.CreateAsync();
            browser ??= await playwright.Chromium.LaunchAsync(new BrowserTypeLaunchOptions { Headless = true });
        }
        finally { browserGate.Release(); }
        await using var context = await browser!.NewContextAsync();
        var page = await context.NewPageAsync();
        await page.RouteAsync("**/*", async route =>
        {
            if (await IsPublicUrl(route.Request.Url, ct)) await route.ContinueAsync();
            else await route.AbortAsync();
        });
        await page.GotoAsync(url, new PageGotoOptions { WaitUntil = WaitUntilState.DOMContentLoaded, Timeout = 20_000 });
        return await page.ContentAsync();
    }

    private async Task<(byte[] Bytes, string MediaType, string FinalUrl)> Fetch(string url, CancellationToken ct)
    {
        for (var redirect = 0; redirect < 4; redirect++)
        {
            if (!await IsPublicUrl(url, ct)) throw new InvalidOperationException("Private or invalid URL");
            using var response = await http.GetAsync(url, HttpCompletionOption.ResponseHeadersRead, ct);
            if ((int)response.StatusCode is >= 300 and < 400 && response.Headers.Location is not null)
            {
                url = new Uri(new Uri(url), response.Headers.Location).AbsoluteUri;
                continue;
            }
            response.EnsureSuccessStatusCode();
            if (response.Content.Headers.ContentLength > MaxBytes) throw new NotSupportedException("Material exceeds 10 MiB");
            await using var input = await response.Content.ReadAsStreamAsync(ct);
            using var output = new MemoryStream();
            var buffer = new byte[8192];
            while (true)
            {
                var count = await input.ReadAsync(buffer, ct);
                if (count == 0) break;
                if (output.Length + count > MaxBytes) throw new NotSupportedException("Material exceeds 10 MiB");
                output.Write(buffer, 0, count);
            }
            return (output.ToArray(), response.Content.Headers.ContentType?.MediaType ?? "application/octet-stream", url);
        }
        throw new InvalidOperationException("Too many redirects");
    }

    private static string? Resolve(string baseUrl, string? href)
    {
        if (string.IsNullOrWhiteSpace(href) || !Uri.TryCreate(new Uri(baseUrl), href, out var uri)
            || uri.Scheme is not ("https" or "http")) return null;
        return uri.AbsoluteUri;
    }

    private static string Clean(string value) => Regex.Replace(value, @"\s+", " ").Trim();

    private static async Task<bool> IsPublicUrl(string raw, CancellationToken ct)
    {
        if (!Uri.TryCreate(raw, UriKind.Absolute, out var uri) || uri.Scheme is not ("https" or "http")
            || uri.UserInfo.Length > 0 || uri.Host.Equals("localhost", StringComparison.OrdinalIgnoreCase)) return false;
        try
        {
            var addresses = await Dns.GetHostAddressesAsync(uri.Host, ct);
            return addresses.Length > 0 && addresses.All(ip =>
            {
                if (IPAddress.IsLoopback(ip)) return false;
                var bytes = ip.GetAddressBytes();
                return ip.AddressFamily == System.Net.Sockets.AddressFamily.InterNetwork
                    ? bytes[0] != 0 && bytes[0] != 10 && bytes[0] != 127 && bytes[0] < 224
                      && !(bytes[0] == 169 && bytes[1] == 254)
                      && !(bytes[0] == 172 && bytes[1] is >= 16 and <= 31)
                      && !(bytes[0] == 192 && bytes[1] == 168)
                    : !ip.IsIPv6LinkLocal && !ip.IsIPv6SiteLocal && !ip.IsIPv6Multicast
                      && (bytes[0] & 0xfe) != 0xfc;
            });
        }
        catch { return false; }
    }

    public async ValueTask DisposeAsync()
    {
        if (browser is not null) await browser.DisposeAsync();
        playwright?.Dispose();
        http.Dispose();
        browserGate.Dispose();
    }
}

public sealed record SourceResult(string Kind, string[] Links);
public sealed record MaterialResult(string Text, string MediaType, string? Title);
