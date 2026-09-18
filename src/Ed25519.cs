using System.Numerics;
using System.Security.Cryptography;

namespace Kirizu.KiriVers.Client;

/// <summary>
/// RFC 8032 Ed25519 verification using only BCL math (net8.0 has no Ed25519 type).
/// </summary>
internal static class Ed25519Core
{
    static readonly BigInteger P = BigInteger.Pow(2, 255) - 19;
    static readonly BigInteger L = BigInteger.Pow(2, 252) + BigInteger.Parse("27742317777372353535851937790883648493");
    static readonly BigInteger D = Mod(-121665 * Inv(121666));
    static readonly BigInteger I = BigInteger.ModPow(2, (P - 1) / 4, P);
    static readonly Point B = ComputeBase();

    public static bool Verify(ReadOnlySpan<byte> publicKey, ReadOnlySpan<byte> message, ReadOnlySpan<byte> signature)
    {
        if (publicKey.Length != 32 || signature.Length != 64)
        {
            return false;
        }

        if (!TryDecompress(publicKey, out var a))
        {
            return false;
        }

        if (!TryDecompress(signature[..32], out var r))
        {
            return false;
        }

        var s = FromLe(signature[32..]);
        if (s >= L || s < 0)
        {
            return false;
        }

        var hashInput = new byte[64 + message.Length];
        signature[..32].CopyTo(hashInput);
        publicKey.CopyTo(hashInput.AsSpan(32));
        message.CopyTo(hashInput.AsSpan(64));
        var h = Sha512ModL(hashInput);

        var sB = Mul(B, s);
        var hA = Mul(a, h);
        var rha = Add(r, hA);
        return Equal(sB, rha);
    }

    static Point ComputeBase()
    {
        var gy = Mod(4 * Inv(5));
        var gx = RecoverX(gy, 0) ?? throw new InvalidOperationException("Ed25519 base point");
        return new Point(gx, gy, BigInteger.One, Mod(gx * gy));
    }

    static BigInteger Sha512ModL(byte[] data)
    {
        var digest = SHA512.HashData(data);
        return ModL(FromLe(digest));
    }

    static bool TryDecompress(ReadOnlySpan<byte> raw, out Point point)
    {
        point = default;
        if (raw.Length != 32)
        {
            return false;
        }

        var y = FromLe(raw);
        var sign = (int)(y >> 255);
        y &= (BigInteger.One << 255) - 1;
        var x = RecoverX(y, sign);
        if (x is null)
        {
            return false;
        }

        point = new Point(x.Value, y, BigInteger.One, Mod(x.Value * y));
        return true;
    }

    static BigInteger? RecoverX(BigInteger y, int sign)
    {
        var y2 = Mod(y * y);
        var x2 = Mod((y2 - 1) * Inv(Mod(D * y2 + 1)));
        if (x2.IsZero)
        {
            return sign == 0 ? BigInteger.Zero : null;
        }

        var x = BigInteger.ModPow(x2, (P + 3) / 8, P);
        if (Mod(x * x - x2) != 0)
        {
            x = Mod(x * I);
        }

        if (Mod(x * x - x2) != 0)
        {
            return null;
        }

        if ((int)(x & 1) != sign)
        {
            x = P - x;
        }

        return x;
    }

    static Point Add(Point p, Point q)
    {
        var a = Mod((p.Y - p.X) * (q.Y - q.X));
        var b = Mod((p.Y + p.X) * (q.Y + q.X));
        var c = Mod(2 * p.T * q.T * D);
        var d = Mod(2 * p.Z * q.Z);
        var e = b - a;
        var f = d - c;
        var g = d + c;
        var h = b + a;
        return new Point(Mod(e * f), Mod(g * h), Mod(f * g), Mod(e * h));
    }

    static Point Mul(Point p, BigInteger s)
    {
        var q = new Point(BigInteger.Zero, BigInteger.One, BigInteger.One, BigInteger.Zero);
        var addend = p;
        for (var i = 0; i < 256; i++)
        {
            if (!((s >> i) & 1).IsZero)
            {
                q = Add(q, addend);
            }

            addend = Add(addend, addend);
        }

        return q;
    }

    static bool Equal(Point p, Point q)
    {
        if (Mod(p.X * q.Z - q.X * p.Z) != 0)
        {
            return false;
        }

        return Mod(p.Y * q.Z - q.Y * p.Z) == 0;
    }

    static BigInteger Inv(BigInteger x) => BigInteger.ModPow(Mod(x), P - 2, P);

    static BigInteger Mod(BigInteger a)
    {
        var r = a % P;
        return r < 0 ? r + P : r;
    }

    static BigInteger ModL(BigInteger a)
    {
        var r = a % L;
        return r < 0 ? r + L : r;
    }

    static BigInteger FromLe(ReadOnlySpan<byte> bytes) => new(bytes, isUnsigned: true, isBigEndian: false);

    readonly struct Point
    {
        public Point(BigInteger x, BigInteger y, BigInteger z, BigInteger t)
        {
            X = x;
            Y = y;
            Z = z;
            T = t;
        }

        public BigInteger X { get; }
        public BigInteger Y { get; }
        public BigInteger Z { get; }
        public BigInteger T { get; }
    }
}
