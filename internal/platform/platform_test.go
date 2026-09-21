package platform

import (
	"maps"
	"slices"
	"testing"
)

// docs/app-init.md §4.1–4.2 全表；缺一项即 C03-1 失败。
var appInitPresetOS = []string{
	"windows", "macos", "linux", "freebsd", "openbsd", "netbsd", "dragonfly",
	"solaris", "illumos", "aix", "haiku",
	"android", "ios", "ipados", "tvos", "watchos", "visionos", "harmonyos", "chromeos",
	"embedded", "rtos", "zephyr", "freertos", "wasm", "fuchsia",
}

var appInitPresetArch = []string{
	"x86", "x86_64", "arm", "armv6", "armv7", "arm64",
	"riscv32", "riscv64", "loongarch64",
	"mips", "mipsel", "mips64", "mips64el",
	"ppc", "ppc64", "ppc64le", "s390x", "sparc64",
	"wasm32", "thumb", "universal", "any",
}

var appInitOSAliases = map[string]string{
	"darwin": OSMacOS,
	"osx":    OSMacOS,
}

var appInitArchAliases = map[string]string{
	"amd64":   ArchX86_64,
	"i386":    ArchX86,
	"i686":    ArchX86,
	"aarch64": ArchARM64,
	"armv7l":  ArchARMv7,
	"armv7a":  ArchARMv7,
}

func TestAppInitPresetTablesComplete(t *testing.T) {
	t.Parallel()
	if !slices.Equal(PresetOS, appInitPresetOS) {
		t.Fatalf("PresetOS drift:\n got %v\nwant %v", PresetOS, appInitPresetOS)
	}
	if !slices.Equal(PresetArch, appInitPresetArch) {
		t.Fatalf("PresetArch drift:\n got %v\nwant %v", PresetArch, appInitPresetArch)
	}
	if !maps.Equal(OSAliases, appInitOSAliases) {
		t.Fatalf("OSAliases=%v want %v", OSAliases, appInitOSAliases)
	}
	if !maps.Equal(ArchAliases, appInitArchAliases) {
		t.Fatalf("ArchAliases=%v want %v", ArchAliases, appInitArchAliases)
	}
	for alias, canon := range appInitOSAliases {
		if got := CanonicalOS(nil, alias); got != canon {
			t.Errorf("CanonicalOS(%q)=%q want %q", alias, got, canon)
		}
		if got := CanonicalOSWrite(alias); got != canon {
			t.Errorf("CanonicalOSWrite(%q)=%q want %q", alias, got, canon)
		}
	}
	for alias, canon := range appInitArchAliases {
		if got := CanonicalArch(alias); got != canon {
			t.Errorf("CanonicalArch(%q)=%q want %q", alias, got, canon)
		}
	}
}

func TestCanonicalOSAliases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw  string
		want string
	}{
		{"darwin", OSMacOS},
		{"Darwin", OSMacOS},
		{"osx", OSMacOS},
		{"OSX", OSMacOS},
		{"macos", OSMacOS},
		{"windows", "windows"},
		{"linux", "linux"},
		{"  IOS ", OSiOS},
		{"my-custom-os", "my-custom-os"},
	}
	for _, tc := range cases {
		if got := CanonicalOS(nil, tc.raw); got != tc.want {
			t.Errorf("CanonicalOS(nil, %q)=%q want %q", tc.raw, got, tc.want)
		}
		if got := CanonicalOSWrite(tc.raw); got != tc.want {
			t.Errorf("CanonicalOSWrite(%q)=%q want %q", tc.raw, got, tc.want)
		}
	}
}

func TestCanonicalOSIPadOS(t *testing.T) {
	t.Parallel()
	if got := CanonicalOS(nil, "ipados"); got != OSiOS {
		t.Fatalf("default ipados=%q want ios", got)
	}
	if got := CanonicalOS([]string{"ios", "macos"}, "ipados"); got != OSiOS {
		t.Fatalf("without ipados in matrix=%q", got)
	}
	if got := CanonicalOS([]string{"ipados"}, "ipados"); got != OSiPadOS {
		t.Fatalf("enabled ipados=%q", got)
	}
	if got := CanonicalOS([]string{"iPadOS"}, "IPADOS"); got != OSiPadOS {
		t.Fatalf("fold enabled ipados=%q", got)
	}
	if got := CanonicalOSWrite("ipados"); got != OSiPadOS {
		t.Fatalf("write ipados=%q want ipados", got)
	}
}

func TestCanonicalArchAliases(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"amd64":   ArchX86_64,
		"AMD64":   ArchX86_64,
		"x86_64":  ArchX86_64,
		"aarch64": ArchARM64,
		"arm64":   ArchARM64,
		"i386":    ArchX86,
		"i686":    ArchX86,
		"x86":     ArchX86,
		"armv7l":  ArchARMv7,
		"armv7a":  ArchARMv7,
		"armv7":   ArchARMv7,
		"riscv64": "riscv64",
		"my_arch": "my_arch",
	}
	for raw, want := range cases {
		if got := CanonicalArch(raw); got != want {
			t.Errorf("CanonicalArch(%q)=%q want %q", raw, got, want)
		}
	}
}

func TestValidOSArchSlug(t *testing.T) {
	t.Parallel()
	if !ValidOSArchSlug("macos") || !ValidOSArchSlug("x86_64") || !ValidOSArchSlug("my-os") {
		t.Fatal("expected valid slugs")
	}
	if ValidOSArchSlug("ab") || ValidOSArchSlug("Has Space") || ValidOSArchSlug("") {
		t.Fatal("expected invalid slugs")
	}
}

func TestValidHwRevSlug(t *testing.T) {
	t.Parallel()
	if !ValidHwRevSlug("v1") || !ValidHwRevSlug("a") || !ValidHwRevSlug("rev-b") {
		t.Fatal("expected valid hw-rev slugs")
	}
	if ValidHwRevSlug("") || ValidHwRevSlug("Has Space") || ValidHwRevSlug("V1") {
		t.Fatal("expected invalid hw-rev slugs")
	}
	if ValidOSArchSlug("v1") {
		t.Fatal("OS/Arch must still reject v1")
	}
}

func TestCatalogContainsPresetsAndAliases(t *testing.T) {
	t.Parallel()
	osCat := OSCatalog()
	archCat := ArchCatalog()
	osBySlug := map[string][]string{}
	for _, e := range osCat {
		osBySlug[e.Slug] = e.Aliases
	}
	archBySlug := map[string][]string{}
	for _, e := range archCat {
		archBySlug[e.Slug] = e.Aliases
	}
	for alias, canon := range appInitOSAliases {
		if !slices.Contains(osBySlug[canon], alias) {
			t.Fatalf("catalog os %s missing alias %s: %v", canon, alias, osBySlug[canon])
		}
	}
	for alias, canon := range appInitArchAliases {
		if !slices.Contains(archBySlug[canon], alias) {
			t.Fatalf("catalog arch %s missing alias %s: %v", canon, alias, archBySlug[canon])
		}
	}
	if _, ok := osBySlug[OSiPadOS]; !ok {
		t.Fatal("catalog missing ipados")
	}
	if len(osCat) != len(PresetOS) || len(archCat) != len(PresetArch) {
		t.Fatalf("catalog size os=%d arch=%d", len(osCat), len(archCat))
	}
}
