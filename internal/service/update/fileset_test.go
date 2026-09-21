package update

import (
	"testing"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

func TestCanonicalFilesetIgnoresOrderAndSeparators(t *testing.T) {
	manifest := []model.ManifestEntry{
		{Path: "foo/bar", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Size: 10},
		{Path: "a/b", SHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Size: 20},
		{Path: "keep/me", SHA256: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", Size: 5, InstallPolicy: model.InstallPolicyKeepIfExists},
	}
	a := ResolveNeededFileset([]string{`foo\bar`, "a/b"}, manifest)
	b := ResolveNeededFileset([]string{"a/b", "foo/bar"}, manifest)
	c := ResolveNeededFileset([]string{"foo/bar", "a/b", "foo/bar", `a\b`}, manifest)
	if a.Unknown || b.Unknown || c.Unknown {
		t.Fatalf("known paths must not be unknown: %+v %+v %+v", a, b, c)
	}
	if a.SHA256 == "" || a.SHA256 != b.SHA256 || a.SHA256 != c.SHA256 {
		t.Fatalf("fileset hash must be order/separator invariant: %s %s %s", a.SHA256, b.SHA256, c.SHA256)
	}
	if len(a.Entries) != 2 || a.NeededSum != 30 {
		t.Fatalf("unique needed=%d sum=%d", len(a.Entries), a.NeededSum)
	}
}

func TestCanonicalFilesetNFCInvariant(t *testing.T) {
	nfc := "cafe\u00e9/a.txt"
	nfd := "cafe\u0065\u0301/a.txt"
	sha := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	manifest := []model.ManifestEntry{{Path: nfc, SHA256: sha, Size: 4}}
	a := ResolveNeededFileset([]string{nfc}, manifest)
	b := ResolveNeededFileset([]string{nfd}, manifest)
	if a.Unknown || b.Unknown {
		t.Fatalf("NFC/NFD paths must match stored Manifest: %+v %+v", a, b)
	}
	if a.SHA256 == "" || a.SHA256 != b.SHA256 {
		t.Fatalf("fileset hash must be NFC-invariant: %s %s", a.SHA256, b.SHA256)
	}
}

func TestCanonicalFilesetUnknownPath(t *testing.T) {
	manifest := []model.ManifestEntry{
		{Path: "foo/bar", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Size: 10},
	}
	res := ResolveNeededFileset([]string{"foo/bar", "not/there"}, manifest)
	if !res.Unknown {
		t.Fatal("missing manifest path must mark unknown")
	}
}

func TestExceedsUncompressedGates(t *testing.T) {
	// D6: needed >= max_bytes
	if !ExceedsUncompressedGates(512, 10000, 512) {
		t.Fatal("D6 inclusive max_bytes")
	}
	if ExceedsUncompressedGates(511, 10000, 512) {
		t.Fatal("below hard cap must pass")
	}
	// D7: needed >= 70% of ALL manifest sizes (including KEEP), not zip Size
	if !ExceedsUncompressedGates(70, 100, 10_000) {
		t.Fatal("D7 inclusive 70%")
	}
	if ExceedsUncompressedGates(69, 100, 10_000) {
		t.Fatal("below 70% must pass")
	}
}

func TestManifestUncompressedSumIncludesKeep(t *testing.T) {
	sum := ManifestUncompressedSum([]model.ManifestEntry{
		{Size: 10, InstallPolicy: model.InstallPolicyOverwrite},
		{Size: 90, InstallPolicy: model.InstallPolicyKeepIfExists},
	})
	if sum != 100 {
		t.Fatalf("sum=%d", sum)
	}
}
