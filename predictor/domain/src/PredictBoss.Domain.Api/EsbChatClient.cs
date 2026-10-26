using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Net.WebSockets;
using System.Text;
using System.Text.Json;

public sealed class EsbChatClient
{
    private const int CancelRequestAttempts = 6;
    private static readonly TimeSpan CancelRequestRetryDelay = TimeSpan.FromMilliseconds(100);
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
        string requestId, string text, CancellationToken ct)
    {
        using var socket = await ConnectStream(companyId, conversationId, null, ct);

        var outgoing = JsonSerializer.SerializeToUtf8Bytes(new
        {
            kind = "question", requestId, companyId, conversationId, analysisId, text
        });
        await socket.SendAsync(outgoing, WebSocketMessageType.Text, true, ct);

        while (socket.State == WebSocketState.Open)
        {
            var root = await ReceiveFrame(socket, ct);
            var kind = String(root, "kind");
            if (kind is "accepted" or "cancel_requested") continue;
            var responseRequestId = String(root, "requestId");
            if (responseRequestId is not null && responseRequestId != requestId) continue;
            if (kind == "error")
                throw new EsbChatException(409, String(root, "error") ?? "esb_error", root.GetRawText());
            if (kind is "assistant_answer" or "assistant_cancelled")
                return root.Clone();
        }
        throw new EsbChatException(504, "assistant_response_failed", "ESB closed the chat stream before answering.");
    }

    public async Task<string> CancelQuestion(Guid companyId, string conversationId, string requestId,
        CancellationToken ct)
    {
        using var socket = await ConnectStream(companyId, conversationId, requestId, ct);
        var outgoing = JsonSerializer.SerializeToUtf8Bytes(new { kind = "cancel", requestId });

        for (var attempt = 0; attempt < CancelRequestAttempts; attempt++)
        {
            await socket.SendAsync(outgoing, WebSocketMessageType.Text, true, ct);
            var notFound = false;
            while (socket.State == WebSocketState.Open)
            {
                var frame = await ReceiveFrame(socket, ct);
                var responseRequestId = String(frame, "requestId");
                if (responseRequestId is not null && responseRequestId != requestId) continue;

                var kind = String(frame, "kind");
                if (kind == "assistant_cancelled") return "cancelled";
                if (kind is "assistant_answer" or "analysis_result") return "completed";
                if (kind == "cancel_requested") continue;
                if (kind == "cancel_failed")
                    throw new EsbChatException(409, "cancel_failed", frame.GetRawText());
                if (kind != "error") continue;

                var code = String(frame, "error") ?? "esb_error";
                if (code == "request_not_found")
                {
                    notFound = true;
                    break;
                }
                throw new EsbChatException(409, code, frame.GetRawText());
            }

            if (!notFound || attempt + 1 == CancelRequestAttempts) break;
            await Task.Delay(CancelRequestRetryDelay, ct);
        }

        throw new EsbChatException(404, "request_not_found", "The Konstantin request was not found.");
    }

    private async Task<ClientWebSocket> ConnectStream(Guid companyId, string conversationId, string? requestId,
        CancellationToken ct)
    {
        var query = $"companyId={Uri.EscapeDataString(companyId.ToString("D"))}" +
                    $"&conversationId={Uri.EscapeDataString(conversationId)}";
        if (!string.IsNullOrWhiteSpace(requestId)) query += $"&requestId={Uri.EscapeDataString(requestId)}";
        var streamUri = new UriBuilder(baseUrl)
        {
            Scheme = baseUrl.StartsWith("https:", StringComparison.OrdinalIgnoreCase) ? "wss" : "ws",
            Path = "/stream",
            Query = query
        }.Uri;
        var socket = new ClientWebSocket();
        socket.Options.SetRequestHeader("Authorization", $"Bearer {apiKey}");
        try
        {
            await socket.ConnectAsync(streamUri, ct);
            return socket;
        }
        catch
        {
            socket.Dispose();
            throw;
        }
    }

    private static async Task<JsonElement> ReceiveFrame(ClientWebSocket socket, CancellationToken ct)
    {
        var buffer = new byte[16 * 1024];
        using var response = new MemoryStream();
        while (true)
        {
            var frame = await socket.ReceiveAsync(buffer, ct);
            if (frame.MessageType == WebSocketMessageType.Close)
                throw new EsbChatException(504, "assistant_response_failed", "ESB closed the chat stream before answering.");
            if (frame.MessageType != WebSocketMessageType.Text)
                throw new EsbChatException(502, "invalid_esb_chat_response", "ESB returned a non-text chat frame.");
            response.Write(buffer, 0, frame.Count);
            if (!frame.EndOfMessage) continue;

            using var document = JsonDocument.Parse(response.ToArray());
            return document.RootElement.Clone();
        }
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
