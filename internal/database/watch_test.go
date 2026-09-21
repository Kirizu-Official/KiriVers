package database

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

func TestIsUnavailable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "not found", err: gorm.ErrRecordNotFound, want: false},
		{name: "canceled", err: context.Canceled, want: false},
		{name: "deadline", err: context.DeadlineExceeded, want: false},
		{name: "wrapped not found", err: fmt.Errorf("lookup: %w", gorm.ErrRecordNotFound), want: false},
		{name: "bad conn", err: driver.ErrBadConn, want: true},
		{name: "refused", err: errors.New("dial tcp 127.0.0.1:5432: connect: connection refused"), want: true},
		{name: "connectex", err: errors.New("dial tcp: connectex: No connection could be made"), want: true},
		{name: "generic", err: errors.New("duplicate key"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsUnavailable(tc.err); got != tc.want {
				t.Fatalf("IsUnavailable(%v)=%v want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestIsUnavailableNetError(t *testing.T) {
	t.Parallel()
	op := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	if !IsUnavailable(op) {
		t.Fatal("net.OpError without timeout must be unavailable")
	}
}

func TestWatcherMarksDownAndReconnects(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	pingErr := errors.New("connection refused")
	w := &Watcher{
		ping: func(context.Context) error {
			mu.Lock()
			defer mu.Unlock()
			return pingErr
		},
		interval: 15 * time.Millisecond,
		timeout:  time.Second,
		log:      zerolog.Nop(),
	}
	w.ok.Store(true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.loop(ctx)

	waitUntil(t, time.Second, func() bool { return !w.Available() })

	mu.Lock()
	pingErr = nil
	mu.Unlock()
	waitUntil(t, time.Second, func() bool { return w.Available() })
}

func TestWatcherNilAvailable(t *testing.T) {
	t.Parallel()
	var w *Watcher
	if !w.Available() {
		t.Fatal("nil watcher must be treated as available")
	}
}

func waitUntil(t *testing.T, timeout time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("timeout waiting for watcher state")
}
