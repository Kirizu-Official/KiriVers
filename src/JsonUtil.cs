using System.Text.Json;
using System.Text.Json.Serialization;

namespace Kirizu.KiriVers.Client;

internal static class JsonUtil
{
    internal static readonly JsonSerializerOptions Options = Create();

    static JsonSerializerOptions Create()
    {
        var o = new JsonSerializerOptions
        {
            PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower,
            DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
            PropertyNameCaseInsensitive = true,
            ReadCommentHandling = JsonCommentHandling.Skip,
            AllowTrailingCommas = true,
        };
        o.Converters.Add(new JsonStringEnumConverter(JsonNamingPolicy.SnakeCaseLower));
        return o;
    }

    internal static byte[] Serialize<T>(T value) => JsonSerializer.SerializeToUtf8Bytes(value, Options);

    internal static T Deserialize<T>(ReadOnlySpan<byte> utf8)
    {
        var value = JsonSerializer.Deserialize<T>(utf8, Options);
        if (value is null)
        {
            throw new ApiException(0, "INVALID_RESPONSE", "response JSON was empty or null", null, null);
        }

        return value;
    }

    internal static T Deserialize<T>(string json)
    {
        var value = JsonSerializer.Deserialize<T>(json, Options);
        if (value is null)
        {
            throw new ApiException(0, "INVALID_RESPONSE", "response JSON was empty or null", null, null);
        }

        return value;
    }
}
