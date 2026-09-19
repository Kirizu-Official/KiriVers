using System.Text;
using System.Text.Json;

namespace Kirizu.KiriVers.Client;

/// <summary>
/// Parsed <c>{ "error": { "code", "message", "details" } }</c> envelope.
/// Unknown <see cref="Code"/> values are kept as opaque strings.
/// </summary>
public sealed class ApiException : Exception
{
    public ApiException(int statusCode, string code, string message, JsonElement? details, int? retryAfterSeconds)
        : base(message)
    {
        StatusCode = statusCode;
        Code = code;
        ErrorMessage = message;
        Details = details;
        RetryAfterSeconds = retryAfterSeconds;
    }

    public int StatusCode { get; }
    public string Code { get; }
    public string ErrorMessage { get; }
    public JsonElement? Details { get; }
    public int? RetryAfterSeconds { get; }

    internal static ApiException FromResponse(int statusCode, byte[] body, IReadOnlyDictionary<string, string> headers)
    {
        int? retryAfter = null;
        if (headers.TryGetValue("Retry-After", out var ra) && int.TryParse(ra, out var seconds))
        {
            retryAfter = seconds;
        }

        if (body.Length == 0)
        {
            return new ApiException(statusCode, FallbackCode(statusCode), $"HTTP {statusCode}", null, retryAfter);
        }

        try
        {
            var env = JsonSerializer.Deserialize<ErrorEnvelope>(body, JsonUtil.Options);
            var err = env?.Error;
            if (err is not null && (!string.IsNullOrEmpty(err.Code) || !string.IsNullOrEmpty(err.Message)))
            {
                JsonElement? details = err.Details.ValueKind is JsonValueKind.Undefined or JsonValueKind.Null
                    ? null
                    : err.Details;
                return new ApiException(
                    statusCode,
                    string.IsNullOrEmpty(err.Code) ? FallbackCode(statusCode) : err.Code,
                    string.IsNullOrEmpty(err.Message) ? $"HTTP {statusCode}" : err.Message,
                    details,
                    retryAfter);
            }
        }
        catch (JsonException)
        {
            // Fall through to opaque HTTP error; do not drop the status.
        }

        var preview = Encoding.UTF8.GetString(body);
        if (preview.Length > 200)
        {
            preview = preview[..200];
        }

        return new ApiException(statusCode, FallbackCode(statusCode), preview, null, retryAfter);
    }

    static string FallbackCode(int statusCode) => statusCode switch
    {
        400 => "INVALID_REQUEST",
        401 => "UNAUTHORIZED",
        403 => "FORBIDDEN",
        404 => "NOT_FOUND",
        409 => "CONFLICT",
        412 => "PRECONDITION_FAILED",
        429 => "RATE_LIMITED",
        500 => "INTERNAL_ERROR",
        _ => "HTTP_ERROR",
    };

    sealed class ErrorEnvelope
    {
        public ErrorBody? Error { get; set; }
    }

    sealed class ErrorBody
    {
        public string? Code { get; set; }
        public string? Message { get; set; }
        public JsonElement Details { get; set; }
    }
}

/// <summary>Thrown when a downloaded package or check signature does not match.</summary>
public sealed class VerifyException : Exception
{
    public VerifyException(string message) : base(message) { }
}

/// <summary>Thrown when a relative path escapes the install root or fails NFC normalization.</summary>
public sealed class PathException : Exception
{
    public PathException(string message) : base(message) { }
}
