package pathutil_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/unicode/norm"

	"github.com/Kirizu-Official/KiriVers/pkg/pathutil"
)

type mockEntry struct {
	Path          string
	Size          int64
	SHA256        string
	InstallPolicy string
}

func (m mockEntry) GetPath() string          { return m.Path }
func (m mockEntry) GetSize() int64           { return m.Size }
func (m mockEntry) GetSHA256() string        { return m.SHA256 }
func (m mockEntry) GetInstallPolicy() string { return m.InstallPolicy }

func TestNormalizeAndValidatePath_Valid(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple file",
			input:    "app.exe",
			expected: "app.exe",
		},
		{
			name:     "nested forward slash",
			input:    "bin/resources/config.json",
			expected: "bin/resources/config.json",
		},
		{
			name:     "windows backslash conversion",
			input:    `bin\resources\config.json`,
			expected: "bin/resources/config.json",
		},
		{
			name:     "mixed slashes and duplicate slashes",
			input:    `bin//sub\\dir///file.txt`,
			expected: "bin/sub/dir/file.txt",
		},
		{
			name:     "trailing slash stripped",
			input:    "folder/subfolder/",
			expected: "folder/subfolder",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := pathutil.NormalizeAndValidatePath(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestNormalizeAndValidatePath_UnicodeNFDvsNFC(t *testing.T) {
	// "café": NFD has 'e' + combining acute accent (\u0301)
	nfdStr := "caf\u0065\u0301.txt"
	nfcStr := "caf\u00e9.txt"

	// Verify they are different in raw byte representation
	assert.NotEqual(t, []byte(nfdStr), []byte(nfcStr))

	normNFD, err := pathutil.NormalizeAndValidatePath(nfdStr)
	require.NoError(t, err)

	normNFC, err := pathutil.NormalizeAndValidatePath(nfcStr)
	require.NoError(t, err)

	assert.Equal(t, normNFC, normNFD)
	assert.Equal(t, norm.NFC.String(nfdStr), normNFD)
}

func TestNormalizeAndValidatePath_InvalidPaths(t *testing.T) {
	invalidCases := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"whitespace only", "   "},
		{"leading slash", "/etc/passwd"},
		{"leading backslash", `\Windows\System32`},
		{"drive letter C:", "C:/Program Files/app.exe"},
		{"drive letter d:", `d:\data\file.bin`},
		{"drive letter in middle", "foo/C:/bar"},
		{"dot segment root", "."},
		{"dot segment prefix", "./file.txt"},
		{"dot segment middle", "dir/./file.txt"},
		{"dotdot segment root", ".."},
		{"dotdot prefix", "../file.txt"},
		{"dotdot middle", "dir/../file.txt"},
		{"dotdot end", "dir/.."},
		{"NUL byte", "dir/file\x00.txt"},
		{"bell control char", "dir/file\x07.txt"},
		{"newline control char", "dir/file\n.txt"},
		{"cr control char", "dir/file\r.txt"},
		{"esc control char", "dir/file\x1b.txt"},
	}

	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := pathutil.NormalizeAndValidatePath(tc.input)
			require.Error(t, err)
			assert.True(t, errors.Is(err, pathutil.ErrInvalidPath), "expected ErrInvalidPath, got %v", err)
		})
	}
}

func TestCalculateRootHash(t *testing.T) {
	sha1 := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	sha2 := "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"

	t.Run("empty entries returns EmptyRootHash", func(t *testing.T) {
		h, err := pathutil.CalculateRootHash([]mockEntry{})
		require.NoError(t, err)
		assert.Equal(t, pathutil.EmptyRootHash, h)
	})

	t.Run("deterministic sorting: order of inputs does not affect root hash", func(t *testing.T) {
		entries1 := []mockEntry{
			{Path: "bin/z_app", Size: 100, SHA256: sha1, InstallPolicy: "OVERWRITE"},
			{Path: "bin/a_app", Size: 200, SHA256: sha2, InstallPolicy: "OVERWRITE"},
		}
		entries2 := []mockEntry{
			{Path: "bin/a_app", Size: 200, SHA256: sha2, InstallPolicy: "OVERWRITE"},
			{Path: "bin/z_app", Size: 100, SHA256: sha1, InstallPolicy: "OVERWRITE"},
		}

		h1, err1 := pathutil.CalculateRootHash(entries1)
		require.NoError(t, err1)

		h2, err2 := pathutil.CalculateRootHash(entries2)
		require.NoError(t, err2)

		assert.Equal(t, h1, h2)
		assert.Len(t, h1, 64)
	})

	t.Run("NFD and backslash produces same Root Hash as NFC and forward slash", func(t *testing.T) {
		// Entry 1 with Windows backslash and NFD unicode
		e1 := []mockEntry{
			{Path: "data\\caf\u0065\u0301.txt", Size: 50, SHA256: sha1, InstallPolicy: "OVERWRITE"},
		}
		// Entry 2 with forward slash and NFC unicode
		e2 := []mockEntry{
			{Path: "data/caf\u00e9.txt", Size: 50, SHA256: sha1, InstallPolicy: "OVERWRITE"},
		}

		h1, err1 := pathutil.CalculateRootHash(e1)
		require.NoError(t, err1)

		h2, err2 := pathutil.CalculateRootHash(e2)
		require.NoError(t, err2)

		assert.Equal(t, h1, h2)
	})

	t.Run("different install policies result in different root hashes", func(t *testing.T) {
		e1 := []mockEntry{
			{Path: "cfg.json", Size: 10, SHA256: sha1, InstallPolicy: "OVERWRITE"},
		}
		e2 := []mockEntry{
			{Path: "cfg.json", Size: 10, SHA256: sha1, InstallPolicy: "KEEP_IF_EXISTS"},
		}

		h1, err1 := pathutil.CalculateRootHash(e1)
		require.NoError(t, err1)

		h2, err2 := pathutil.CalculateRootHash(e2)
		require.NoError(t, err2)

		assert.NotEqual(t, h1, h2)
	})
}
