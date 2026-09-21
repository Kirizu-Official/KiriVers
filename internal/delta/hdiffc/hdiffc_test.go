package hdiffc

import (
	"bytes"
	"sync"
	"testing"
)

func TestCreateApplyRoundTrip(t *testing.T) {
	oldData := bytes.Repeat([]byte("the quick brown fox jumps over the lazy dog\n"), 64)
	newData := append([]byte{}, oldData...)
	copy(newData[20:], []byte("THE QUICK BROWN FOX"))
	newData = append(newData, []byte(" extra tail")...)

	diff, err := Create(oldData, newData)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !bytes.HasPrefix(diff, Magic) {
		t.Fatalf("diff must start with HDIFF13&, got %q", diff[:min(8, len(diff))])
	}
	got, err := Apply(oldData, diff)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !bytes.Equal(got, newData) {
		t.Fatalf("roundtrip mismatch: %d vs %d bytes", len(got), len(newData))
	}
}

func TestCreateApplyEmpty(t *testing.T) {
	cases := []struct {
		name string
		old  []byte
		new  []byte
	}{
		{"empty old", nil, []byte("brand new")},
		{"empty new", []byte("drop me"), nil},
		{"both empty", nil, nil},
		{"identical", []byte("same"), []byte("same")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diff, err := Create(tc.old, tc.new)
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			if !bytes.HasPrefix(diff, Magic) {
				t.Fatalf("missing HDIFF13& magic, got %q", diff[:min(8, len(diff))])
			}
			got, err := Apply(tc.old, diff)
			if err != nil {
				t.Fatalf("apply: %v", err)
			}
			if !bytes.Equal(got, tc.new) && !(len(got) == 0 && len(tc.new) == 0) {
				t.Fatalf("got %q want %q", got, tc.new)
			}
		})
	}
}

func TestApplyRejectsGarbage(t *testing.T) {
	if _, err := Apply([]byte("old"), []byte("not a delta")); err == nil {
		t.Fatal("garbage must be rejected")
	}
}

// TestApplyRejectsCompressedType 压缩 HDIFF13（compressType 非空）必须报错，不链接 zlib。
func TestApplyRejectsCompressedType(t *testing.T) {
	oldData := []byte("old-bytes")
	newData := []byte("new-bytes")
	diff, err := Create(oldData, newData)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(diff, Magic) {
		t.Fatal("need HDIFF13& fixture")
	}
	// 未压缩容器是 "HDIFF13&" + 空 compressType + NUL。插入假类型后头仍可解析。
	spliced := append([]byte{}, Magic...)
	spliced = append(spliced, []byte("zstd")...)
	spliced = append(spliced, diff[len(Magic):]...)
	_, err = Apply(oldData, spliced)
	if err == nil {
		t.Fatal("compressed HDIFF13 must be rejected")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("compressed HDIFF13 is unsupported")) {
		t.Fatalf("want compressed-unsupported error, got %v", err)
	}
}

func TestCreateParallel(t *testing.T) {
	oldData := bytes.Repeat([]byte{0x11, 0x22, 0x33, 0x44}, 1024)
	newData := bytes.Repeat([]byte{0x11, 0x22, 0x55, 0x44}, 1024)
	var wg sync.WaitGroup
	errCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, err := Create(oldData, newData)
			if err != nil {
				errCh <- err
				return
			}
			got, err := Apply(oldData, d)
			if err != nil {
				errCh <- err
				return
			}
			if !bytes.Equal(got, newData) {
				errCh <- errMismatch
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
}

var errMismatch = errString("parallel roundtrip mismatch")

type errString string

func (e errString) Error() string { return string(e) }
