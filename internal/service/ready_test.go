package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Kirizu-Official/KiriVers/internal/storage"
)

type stubStore struct {
	headErr error
}

func (s stubStore) Put(context.Context, string, io.Reader, int64, string) error { return nil }
func (s stubStore) Get(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil
}
func (s stubStore) Range(context.Context, string, int64, int64) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(nil)), nil
}
func (s stubStore) Head(context.Context, string) (int64, string, error) { return 2, "ok", s.headErr }
func (s stubStore) Delete(context.Context, string) error                { return nil }
func (s stubStore) PresignPut(context.Context, string, time.Duration, string) (string, error) {
	return "http://example.invalid/put", nil
}
func (s stubStore) PresignGet(context.Context, string, time.Duration) (string, error) {
	return "http://example.invalid/get", nil
}

func TestCheckReadyMissingDBOrStorage(t *testing.T) {
	ctx := t.Context()
	err := CheckReady(ctx, nil, stubStore{})
	if err == nil || !strings.Contains(err.Error(), "database") {
		t.Fatalf("nil db should fail, got %v", err)
	}
	err = CheckReady(ctx, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "database") || !strings.Contains(err.Error(), "storage") {
		t.Fatalf("expected database and storage errors, got %v", err)
	}
}

func TestCheckReadyStorageHeadFails(t *testing.T) {
	err := CheckReady(t.Context(), nil, stubStore{headErr: errors.New("head failed")})
	if err == nil {
		t.Fatal("expected not ready")
	}
	if !strings.Contains(err.Error(), "storage") || !strings.Contains(err.Error(), "head failed") {
		t.Fatalf("expected storage head error, got %v", err)
	}
	_ = storage.ReadyProbeKey
}
