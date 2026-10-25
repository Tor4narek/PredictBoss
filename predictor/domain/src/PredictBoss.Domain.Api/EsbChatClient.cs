using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Net.WebSockets;
using System.Text;
using System.Text.Json;

public sealed class EsbChatClient
{
    private readonly HttpClient http;
    private readonly string baseUrl;
    private readonly string apiKey;

    public EsbChatClient(IConfiguration configuration)
    {
        baseUrl = configuration["ESB_BASE_URL"]
            ?? throw new InvalidOperationException("ESB_BASE_URL is required");
        apiKey = configuration["ESB_API_KEY"]
            ?? throw new InvalidOperationException("ESB_API_KEY is required");
        http = new HttpClient { BaseAddress = new Uri(baseUrl), Timeout = TimeSpan.FromSeconds(40) };
    }

    public async Task<JsonElement> GetOrStartAnalysis(Guid companyId, bool waitForProfile, CancellationToken ct)
    {
        var retries = waitForProfile ? 30 : 0;
        for (var attempt = 0; ; attempt++)
        {
            using var request = new HttpRequestMessage(HttpMethod.Get, "/call?agent=konstantin")
            {
                Content = JsonContent.Create(new { companyId })
            };
            request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", apiKey);
            using var response = await http.SendAsync(request, HttpCompletionOption.ResponseHeadersRead, ct);
            var body = await response.Content.ReadAsStringAsync(ct);
            if (response.IsSuccessStatusCode || response.StatusCode == HttpStatusCode.Accepted)
            {
                using var document = JsonDocument.Parse(body);
                return document.RootElement.Clone();
            }

            var error = ErrorCode(body);
            if (attempt < retries && response.StatusCode == HttpStatusCode.Conflict
                && error is "profile_update_pending" or "company_profile_not_found")
            {
                await Task.Delay(TimeSpan.FromSeconds(1), ct);
                continue;
            }
            throw new EsbChatException((int)response.StatusCode, error, body);
        }
    }

    public async Task<JsonElement> Ask(Guid companyId, string conversationId, string analysisId,
        string text, CancellationToken ct)
    {
        var streamUri = new UriBuilder(baseUrl)
        {
            Scheme = baseUrl.StartsWith("https:", StringComparison.OrdinalIgnoreCase) ? "wss" : "ws",
            Path = "/stream",
            Query = $"companyId={companyId:D}&conversationId={Uri.EscapeDataString(conversationId)}"
        }.Uri;
        using var socket = new ClientWebSocket();
        socket.Options.SetRequestHeader("Authorization", $"Bearer {apiKey}");
        await socket.ConnectAsync(streamUri, ct);

        var requestId = Guid.NewGuid().ToString("D");
        var outgoing = JsonSerializer.SerializeToUtf8Bytes(new
        {
            kind = "question", requestId, companyId, conversationId, analysisId, text
        });
        await socket.SendAsync(outgoing, WebSocketMessageType.Text, true, ct);

        var buffer = new byte[16 * 1024];
        using var response = new MemoryStream();
        while (socket.State == WebSocketState.Open)
        {
            var frame = await socket.ReceiveAsync(buffer, ct);
            if (frame.MessageType == WebSocketMessageType.Close) break;
            response.Write(buffer, 0, frame.Count);
            if (!frame.EndOfMessage) continue;

            using var document = JsonDocument.Parse(response.ToArray());
            response.SetLength(0);
            var root = document.RootElement;
            var kind = String(root, "kind");
            if (kind == "accepted") continue;
            if (kind == "error")
                throw new EsbChatException(409, String(root, "error") ?? "esb_error", root.GetRawText());
            if (kind == "assistant_answer" && String(root, "requestId") == requestId)
                return root.Clone();
        }
        throw new EsbChatException(504, "assistant_response_failed", "ESB closed the chat stream before answering.");
    }

    private static string ErrorCode(string body)
    {
        try
        {
            using var document = JsonDocument.Parse(body);
            return String(document.RootElement, "error") ?? String(document.RootElement, "code") ?? "esb_error";
        }
        catch (JsonException) { return "esb_error"; }
    }

    private static string? String(JsonElement element, string name)
        => element.TryGetProperty(name, out var property) && property.ValueKind == JsonValueKind.String
            ? property.GetString()
            : null;
}

public sealed class EsbChatException(int statusCode, string code, string responseBody) : Exception(code)
{
    public int StatusCode { get; } = statusCode;
    public string Code { get; } = code;
    public string ResponseBody { get; } = responseBody;
}
