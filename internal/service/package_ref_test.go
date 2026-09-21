package service

import "testing"

func TestParsePackageRef(t *testing.T) {
	hex := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	sha, suffix, ok := ParsePackageRef(hex + ".exe.blockmap")
	if !ok || sha != hex || suffix != ".exe.blockmap" {
		t.Fatalf("decorated ref: sha=%q suffix=%q ok=%v", sha, suffix, ok)
	}
	upper := "ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789"
	sha, suffix, ok = ParsePackageRef(upper)
	if !ok || sha != "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789" || suffix != "" {
		t.Fatalf("uppercase hex: sha=%q suffix=%q ok=%v", sha, suffix, ok)
	}
	if _, _, ok := ParsePackageRef("myprog.exe"); ok {
		t.Fatal("filename must not parse as a package ref")
	}
}

func TestDecoratedPackageName(t *testing.T) {
	hex := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if got := DecoratedPackageName(hex, "app.exe"); got != hex+".exe" {
		t.Fatalf("got %q", got)
	}
}
