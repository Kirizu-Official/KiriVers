package service

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
)

type fakeJobStore struct {
	mu       sync.Mutex
	claimN   int
	releaseN int
	once     *model.Job
	claimErr error
}

func (f *fakeJobStore) Claim(ctx context.Context, _ repository.ClaimFilter) (*model.Job, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.claimN++
	if f.claimErr != nil {
		return nil, f.claimErr
	}
	if f.once != nil {
		j := f.once
		f.once = nil
		return j, nil
	}
	return nil, nil
}

func (f *fakeJobStore) Release(context.Context, uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.releaseN++
	return nil
}

func (f *fakeJobStore) MarkSucceeded(context.Context, uuid.UUID) error { return nil }

func (f *fakeJobStore) MarkSucceededWithResult(context.Context, uuid.UUID, []byte) error { return nil }

func (f *fakeJobStore) MarkFailed(context.Context, uuid.UUID, string) error { return nil }

func (f *fakeJobStore) MarkFailedWithResult(context.Context, uuid.UUID, string, []byte) error {
	return nil
}

func (f *fakeJobStore) Create(context.Context, *model.Job) error { return nil }

func (f *fakeJobStore) GetByID(context.Context, uuid.UUID) (*model.Job, error) { return nil, nil }

func (f *fakeJobStore) Update(context.Context, *model.Job) error { return nil }

func (f *fakeJobStore) GetByIdempotencyKey(context.Context, uuid.UUID, string) (*model.Job, error) {
	return nil, nil
}

func (f *fakeJobStore) RequeueAfter(context.Context, uuid.UUID, time.Duration) error { return nil }

func TestJobWorkerClaimsThenStops(t *testing.T) {
	store := &fakeJobStore{}
	ctx, cancel := context.WithCancel(t.Context())
	stop := StartJobWorkers(ctx, store, 2, zerolog.Nop())
	time.Sleep(50 * time.Millisecond)
	cancel()
	stop()
	store.mu.Lock()
	n := store.claimN
	store.mu.Unlock()
	if n == 0 {
		t.Fatal("expected SKIP LOCKED claim loop to run")
	}
}

func TestJobWorkerReleasesUnknownType(t *testing.T) {
	store := &fakeJobStore{
		once: &model.Job{ID: uuid.New(), Type: "no-handler", Status: model.JobStatusRunning},
	}
	ctx, cancel := context.WithCancel(t.Context())
	stop := StartJobWorkers(ctx, store, 1, zerolog.Nop())
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		store.mu.Lock()
		n := store.releaseN
		store.mu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	stop()
	store.mu.Lock()
	n := store.releaseN
	store.mu.Unlock()
	if n < 1 {
		t.Fatal("expected unknown job type to be released back to queued")
	}
}

func TestStartJobWorkersNoop(t *testing.T) {
	stop := StartJobWorkers(t.Context(), nil, 2, zerolog.Nop())
	stop()
	stop = StartJobWorkers(t.Context(), &fakeJobStore{}, 0, zerolog.Nop())
	stop()
}

func TestJobWorkerLogsNonCancelClaimError(t *testing.T) {
	store := &fakeJobStore{claimErr: errors.New("db down")}
	var buf bytes.Buffer
	ctx, cancel := context.WithCancel(t.Context())
	stop := StartJobWorkers(ctx, store, 1, zerolog.New(&buf))
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		store.mu.Lock()
		n := store.claimN
		store.mu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	stop()
	if !strings.Contains(buf.String(), "job claim failed") || !strings.Contains(buf.String(), "db down") {
		t.Fatalf("expected claim error in logs: %s", buf.String())
	}
}

func TestShouldLogClaimErr(t *testing.T) {
	if shouldLogClaimErr(nil) {
		t.Fatal("nil")
	}
	if shouldLogClaimErr(context.Canceled) {
		t.Fatal("canceled")
	}
	if shouldLogClaimErr(context.DeadlineExceeded) {
		t.Fatal("deadline")
	}
	if !shouldLogClaimErr(errors.New("db down")) {
		t.Fatal("unexpected error must be logged")
	}
}

func TestJobWorkerLogsHandlerError(t *testing.T) {
	id := uuid.New()
	store := &fakeJobStore{once: &model.Job{ID: id, Type: "boom", Status: model.JobStatusRunning}}
	var buf bytes.Buffer
	w := NewJobWorker(store, 1, zerolog.New(&buf))
	w.RegisterHandler("boom", func(context.Context, string, []byte) error {
		return errors.New("handler exploded")
	})
	ctx, cancel := context.WithCancel(t.Context())
	stop := w.Start(ctx)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		store.mu.Lock()
		n := store.claimN
		store.mu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	stop()
	if !strings.Contains(buf.String(), "job handler failed") || !strings.Contains(buf.String(), "handler exploded") {
		t.Fatalf("expected handler error in logs: %s", buf.String())
	}
}
