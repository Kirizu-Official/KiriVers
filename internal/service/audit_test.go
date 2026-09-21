package service

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/google/uuid"
)

// failingAuditStore 是始终写失败的 AuditStore 测试桩：锁定 Record 的吞错契约
// （§11.2：审计失败只记日志，绝不阻塞业务请求、绝不向调用方返回错误）。
type failingAuditStore struct{ calls int }

func (f *failingAuditStore) Insert(_ context.Context, _ *model.AuditEvent) error {
	f.calls++
	return errors.New("injected store failure")
}

func (f *failingAuditStore) ListByProject(_ context.Context, _ uuid.UUID, _ string, _ int) ([]model.AuditEvent, string, error) {
	return nil, "", nil
}

// TestAuditRecordSwallowsStoreFailure 审计写失败不阻塞业务（§11.2 / design §5）：
// Record 无返回值、不 panic；调用方（controller）无法因审计失败中断响应。
func TestAuditRecordSwallowsStoreFailure(t *testing.T) {
	store := &failingAuditStore{}
	svc := NewAuditService(store, zerolog.Nop())
	svc.SetClock(func() time.Time { return time.Unix(1700000000, 0).UTC() })

	projID := uuid.New()
	// Record 必须正常返回（无 error 可断言——契约即「无错误通道」），且确实
	// 尝试了一次写入（失败被吞掉并只记日志）。
	svc.Record(context.Background(), AuditEntry{
		ActorType:        model.AuditActorAdmin,
		ActorFingerprint: "root",
		ProjectID:        &projID,
		Action:           model.AuditActionVersionPublish,
		ResourceType:     "version",
		ResourceID:       uuid.NewString(),
	})
	if store.calls != 1 {
		t.Fatalf("Record must attempt exactly one insert, got %d", store.calls)
	}
}

func TestAuditRecordFailureUsesInjectedLogger(t *testing.T) {
	store := &failingAuditStore{}
	var buf bytes.Buffer
	svc := NewAuditService(store, zerolog.New(&buf))
	svc.Record(context.Background(), AuditEntry{
		ActorType:        model.AuditActorAdmin,
		ActorFingerprint: "root",
		Action:           model.AuditActionVersionPublish,
		ResourceType:     "version",
		ResourceID:       uuid.NewString(),
	})
	if !strings.Contains(buf.String(), "audit record write failed") {
		t.Fatalf("injected logger must receive the failure: %s", buf.String())
	}
}
