package cmd

import "testing"

func TestListenMode(t *testing.T) {
	if got := listenMode("cert.pem", "key.pem"); got != "tls" {
		t.Fatalf("both files: %s", got)
	}
	if got := listenMode("", ""); got != "plain" {
		t.Fatalf("empty: %s", got)
	}
	if got := listenMode("cert.pem", ""); got != "plain" {
		t.Fatalf("cert only: %s", got)
	}
	if got := listenMode("", "key.pem"); got != "plain" {
		t.Fatalf("key only: %s", got)
	}
}
