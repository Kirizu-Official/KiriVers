package service

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func TestHashRootZipMembersAreHex(t *testing.T) {
	sha := strings.Repeat("ab", 32)
	data, err := BuildHashRootZip([]ArchiveMember{
		{Path: "foo/bar.txt", SHA256: sha, Body: []byte("hello")},
		{Path: "dup.txt", SHA256: sha, Body: []byte("hello")},
	})
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(zr.File) != 1 {
		t.Fatalf("duplicate hash must collapse, got %d members", len(zr.File))
	}
	if zr.File[0].Name != sha {
		t.Fatalf("member name=%q want hex", zr.File[0].Name)
	}
	got, err := ExtractHashRootMember(data, sha)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Fatalf("body=%q", got)
	}
}

func TestPathZipUsesManifestPaths(t *testing.T) {
	sha := strings.Repeat("cd", 32)
	data, err := BuildPathZip([]ArchiveMember{
		{Path: `foo\bar.txt`, SHA256: sha, Body: []byte("x")},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := ExtractZipMember(data, "foo/bar.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "x" {
		t.Fatalf("body=%q", got)
	}
}
