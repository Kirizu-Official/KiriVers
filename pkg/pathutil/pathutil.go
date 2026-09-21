package pathutil

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/text/unicode/norm"
)

var (
	// ErrInvalidPath 表示相对路径违反了安全性或格式规则（C06-2, §13.5）。
	ErrInvalidPath = errors.New("invalid path")
)

var (
	// driveLetterRegex 匹配 Windows 盘符（如 C:、D:）。
	driveLetterRegex = regexp.MustCompile(`^[a-zA-Z]:`)
	// consecutiveSlashesRegex 匹配多个连续正斜杠。
	consecutiveSlashesRegex = regexp.MustCompile(`/+`)
)

// NormalizeAndValidatePath 对客户端/服务端传入的相对文件路径进行校验与 Unicode NFC 规范化（C06-2, §13.5）。
//
// 规则：
// 1. 拒绝包含 NUL (`\x00`) 或 ASCII 控制字符 (< 0x20)；
// 2. 拒绝包含 Windows 盘符（如 `C:`）；
// 3. 拒绝原始路径具有前导正斜杠 `/` 或反斜杠 `\`（相对路径禁止绝对前缀）；
// 4. 将所有反斜杠 `\` 转换为正斜杠 `/`；
// 5. 规范化为 Unicode NFC（消除 macOS NFD 与 Windows NFC 之间的字符表示差异）；
// 6. 消除连续重复的正斜杠，并去除首尾空白及正斜杠；
// 7. 严格禁止任何段名等于 `..` 或 `.`，禁止路径穿越；
// 8. 规范化后路径不可为空。
func NormalizeAndValidatePath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("%w: path is empty", ErrInvalidPath)
	}

	// 1. 检查 NUL 字节与 ASCII 控制字符 (< 0x20)
	for i := 0; i < len(trimmed); i++ {
		b := trimmed[i]
		if b < 0x20 {
			return "", fmt.Errorf("%w: contains control character 0x%02x", ErrInvalidPath, b)
		}
	}

	// 2. 检查前导斜杠（禁止以 / 或 \ 开头）
	if strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "\\") {
		return "", fmt.Errorf("%w: path cannot start with leading slash", ErrInvalidPath)
	}

	// 3. 检查盘符
	if driveLetterRegex.MatchString(trimmed) {
		return "", fmt.Errorf("%w: path cannot contain drive letter", ErrInvalidPath)
	}

	// 4. 统一正斜杠
	slashUnified := strings.ReplaceAll(trimmed, "\\", "/")

	// 再次检查是否有盘符（如 foo/C:/bar）
	segments := strings.Split(slashUnified, "/")
	for _, seg := range segments {
		if driveLetterRegex.MatchString(seg) {
			return "", fmt.Errorf("%w: segment cannot contain drive letter", ErrInvalidPath)
		}
		// 7. 检查 segment 是否为 . 或 ..
		if seg == "." || seg == ".." {
			return "", fmt.Errorf("%w: path traversal segment %q is forbidden", ErrInvalidPath, seg)
		}
	}

	// 5. Unicode NFC 规范化
	nfcNormalized := norm.NFC.String(slashUnified)

	// 6. 去除连续重复的 /
	cleaned := consecutiveSlashesRegex.ReplaceAllString(nfcNormalized, "/")
	cleaned = strings.Trim(cleaned, "/")

	if cleaned == "" {
		return "", fmt.Errorf("%w: path normalized to empty", ErrInvalidPath)
	}

	// 最终分段校验保证安全
	for _, seg := range strings.Split(cleaned, "/") {
		if seg == "." || seg == ".." || seg == "" {
			return "", fmt.Errorf("%w: invalid segment in normalized path %q", ErrInvalidPath, seg)
		}
	}

	return cleaned, nil
}

// RootHashItem 参与 Root Hash 计算的条目抽象接口。
type RootHashItem interface {
	GetPath() string
	GetSize() int64
	GetSHA256() string
	GetInstallPolicy() string
}

// EmptyRootHash 是空 Manifest 的 Root Hash（空字节流的 SHA-256）。
const EmptyRootHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// CalculateRootHash 计算多文件 Manifest 的 Root Hash（C06-3, §6.3）。
//
// 算法：
// 1. 每条相对路径必须经过 NFC 与正斜杠规范化；
// 2. 所有条目按规范化 Path 的 UTF-8 字节序严格升序排列；
// 3. 对每个排好序的条目，按照以下格式拼接字符串并送入 SHA-256 哈希器：
//    {path}\n{size}\n{sha256_hex}\n{install_policy}\n
//    其中 sha256_hex 为 64 字节小写十六进制；MD5 不参与计算；
// 4. 若条目列表为空，返回 EmptyRootHash；
// 5. 最终返回 64 字节小写十六进制字符串。
func CalculateRootHash[T RootHashItem](entries []T) (string, error) {
	if len(entries) == 0 {
		return EmptyRootHash, nil
	}

	// 拷贝切片进行排序，避免修改入参切片顺序
	sorted := make([]T, len(entries))
	copy(sorted, entries)

	// 按 UTF-8 字节升序排列 (Go 语言原生字符串 < 即按 UTF-8 字节序比较)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].GetPath() < sorted[j].GetPath()
	})

	hasher := sha256.New()
	for _, entry := range sorted {
		path := entry.GetPath()
		normPath, err := NormalizeAndValidatePath(path)
		if err != nil {
			return "", fmt.Errorf("invalid entry path %q: %w", path, err)
		}

		sha256Hex := strings.ToLower(strings.TrimSpace(entry.GetSHA256()))
		if len(sha256Hex) != 64 {
			return "", fmt.Errorf("entry path %q has invalid sha256 length %d", path, len(sha256Hex))
		}

		policy := strings.ToUpper(strings.TrimSpace(entry.GetInstallPolicy()))
		if policy == "" {
			policy = "OVERWRITE"
		}

		line := fmt.Sprintf("%s\n%d\n%s\n%s\n", normPath, entry.GetSize(), sha256Hex, policy)
		hasher.Write([]byte(line))
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
