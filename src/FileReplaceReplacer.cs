using System.Runtime.InteropServices;
using System.Runtime.Versioning;

namespace Kirizu.KiriVers.Client;

/// <summary>
/// Default desktop replacer: <see cref="File.Replace"/>, then Windows <c>MoveFileEx</c> if the destination is locked.
/// This is not an APK/MSI/one-click product installer.
/// </summary>
public sealed class FileReplaceReplacer : IReplacer
{
    const int MoveFileReplaceExisting = 0x1;
    const int MoveFileCopyAllowed = 0x2;
    const int MoveFileDelayUntilReboot = 0x4;
    const int MoveFileWriteThrough = 0x8;

    public void Replace(string stagedPath, string installPath)
    {
        if (string.IsNullOrEmpty(stagedPath) || string.IsNullOrEmpty(installPath))
        {
            throw new ArgumentException("staged and install paths are required");
        }

        stagedPath = Path.GetFullPath(stagedPath);
        installPath = Path.GetFullPath(installPath);
        var destDir = Path.GetDirectoryName(installPath);
        if (!string.IsNullOrEmpty(destDir))
        {
            Directory.CreateDirectory(destDir);
        }

        try
        {
            if (File.Exists(installPath))
            {
                var backup = installPath + ".bak";
                File.Replace(stagedPath, installPath, backup, ignoreMetadataErrors: true);
            }
            else
            {
                File.Move(stagedPath, installPath);
            }

            return;
        }
        catch (IOException) when (OperatingSystem.IsWindows())
        {
            TryMoveFileEx(stagedPath, installPath);
        }
        catch (UnauthorizedAccessException) when (OperatingSystem.IsWindows())
        {
            TryMoveFileEx(stagedPath, installPath);
        }
    }

    [SupportedOSPlatform("windows")]
    static void TryMoveFileEx(string stagedPath, string installPath)
    {
        var flags = MoveFileReplaceExisting | MoveFileCopyAllowed | MoveFileWriteThrough;
        if (MoveFileEx(stagedPath, installPath, flags))
        {
            return;
        }

        // Destination is likely a running image; schedule replacement for the next reboot.
        var delayed = MoveFileReplaceExisting | MoveFileDelayUntilReboot;
        if (!MoveFileEx(stagedPath, installPath, delayed))
        {
            throw new IOException($"MoveFileEx failed (Win32 {Marshal.GetLastWin32Error()})");
        }

        throw new IOException("install file is in use; replacement scheduled for the next reboot");
    }

    [DllImport("kernel32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
    [return: MarshalAs(UnmanagedType.Bool)]
    [SupportedOSPlatform("windows")]
    static extern bool MoveFileEx(string lpExistingFileName, string? lpNewFileName, int dwFlags);
}
