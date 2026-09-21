package hashutil

import (
	"strings"
	"testing"
)

func TestSHA256Hex(t *testing.T) {
	got, err := SHA256Hex(strings.NewReader("kiri"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 64 {
		t.Fatalf("len=%d", len(got))
	}
}

func TestMD5Hex(t *testing.T) {
	got, err := MD5Hex(strings.NewReader("kiri"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 32 {
		t.Fatalf("len=%d", len(got))
	}
}

func TestSHA512Hex(t *testing.T) {
	got, err := SHA512Hex(strings.NewReader("kiri"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 128 {
		t.Fatalf("len=%d", len(got))
	}
}

func TestMultiHasher(t *testing.T) {
	mh := NewMultiHasher(true)
	data := "hello world multi hasher"
	n, err := mh.Write([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if int64(n) != mh.Size() || mh.Size() != int64(len(data)) {
		t.Fatalf("expected size %d, got %d", len(data), mh.Size())
	}

	expSha256, _ := SHA256Hex(strings.NewReader(data))
	expMd5, _ := MD5Hex(strings.NewReader(data))
	expSha512, _ := SHA512Hex(strings.NewReader(data))

	if mh.SHA256() != expSha256 {
		t.Fatalf("sha256 mismatch: %s != %s", mh.SHA256(), expSha256)
	}
	if mh.MD5() != expMd5 {
		t.Fatalf("md5 mismatch: %s != %s", mh.MD5(), expMd5)
	}
	if mh.SHA512() != expSha512 {
		t.Fatalf("sha512 mismatch: %s != %s", mh.SHA512(), expSha512)
	}
}
