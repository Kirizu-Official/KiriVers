package database

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

func TestGormLoggerWritesWarnAndError(t *testing.T) {
	var buf bytes.Buffer
	gl := newGormLogger(zerolog.New(&buf))
	gl.Warn(context.Background(), "slow %s", "select 1")
	gl.Error(context.Background(), "fail %s", "boom")
	gl.Info(context.Background(), "debug sql %s", "select password from users")
	s := buf.String()
	if !strings.Contains(s, "slow select 1") || !strings.Contains(s, "fail boom") {
		t.Fatalf("missing warn/error: %s", s)
	}
	if strings.Contains(s, "debug sql") {
		t.Fatalf("GORM Info must be ignored at Warn threshold: %s", s)
	}
	if strings.Contains(s, "password") {
		t.Fatalf("must not log secrets: %s", s)
	}
}

func TestGormLoggerIgnoresRecordNotFound(t *testing.T) {
	var buf bytes.Buffer
	gl := newGormLogger(zerolog.New(&buf))
	gl.Trace(context.Background(), time.Now(), func() (string, int64) {
		return "SELECT * FROM projects", 0
	}, gorm.ErrRecordNotFound)
	if buf.Len() != 0 {
		t.Fatalf("record not found must be ignored: %s", buf.String())
	}
}

func TestGormLoggerLogsSlowAndSkipsFast(t *testing.T) {
	var buf bytes.Buffer
	gl := newGormLogger(zerolog.New(&buf))
	gl.Trace(context.Background(), time.Now(), func() (string, int64) {
		return "SELECT 1", 1
	}, nil)
	if buf.Len() != 0 {
		t.Fatalf("fast SQL must not be logged: %s", buf.String())
	}
	gl.Trace(context.Background(), time.Now().Add(-300*time.Millisecond), func() (string, int64) {
		return "SELECT pg_sleep", 1
	}, nil)
	if !strings.Contains(buf.String(), "slow sql") || !strings.Contains(buf.String(), "SELECT pg_sleep") {
		t.Fatalf("slow SQL missing: %s", buf.String())
	}
}

func TestOpenEmptyDSN(t *testing.T) {
	if _, err := Open("", zerolog.Nop()); err == nil {
		t.Fatal("empty dsn must fail")
	}
}
