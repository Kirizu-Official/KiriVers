package semver

import "testing"

func TestCanonicalStripsVAndBuild(t *testing.T) {
	got, err := Canonical("v1.2.3+20260912")
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.2.3" {
		t.Fatalf("got %s", got)
	}
}

func TestCanonicalRejectsIncomplete(t *testing.T) {
	if _, err := Canonical("1.2"); err == nil {
		t.Fatal("expected error")
	}
}
