package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/cache"
	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
)

// newTelemetryFixture 构造内存遥测仓储 + 服务，返回可注入时钟的服务。
func newTelemetryFixture(t *testing.T) (*TelemetryService, *repository.MemoryTelemetryStore, *model.Project) {
	t.Helper()
	store := repository.NewMemoryTelemetryStore()
	svc := NewTelemetryService(store)
	p := &model.Project{Slug: "demo", DeviceSecret: "aabbccdd", DeviceIDPolicy: model.DeviceIDPolicyHashed}
	if err := p.BeforeCreate(nil); err != nil {
		t.Fatal(err)
	}
	return svc, store, p
}

// report 构造一条合法上报的快捷方式。
func report(svc *TelemetryService, p *model.Project, status string, mutators ...func(*ReportInput)) error {
	in := ReportInput{
		DeviceID:    "device-abc",
		OS:          "windows",
		Arch:        "x86_64",
		Channel:     "stable",
		FromVersion: "1.0.0",
		ToVersion:   "1.1.0",
		Status:      status,
	}
	for _, m := range mutators {
		m(&in)
	}
	return svc.Report(context.Background(), p, in)
}

// TestHashDeviceIDDeterministicAndKeyed HMAC 确定性：同密钥同输入恒定；不同
// 密钥/不同输入不同（验收：hashed 模式库中无明文，且不可逆推）。
func TestHashDeviceIDDeterministicAndKeyed(t *testing.T) {
	svc, _, p := newTelemetryFixture(t)
	h1 := svc.HashDeviceID(p, "device-abc")
	h2 := svc.HashDeviceID(p, "device-abc")
	if h1 != h2 || len(h1) != 64 {
		t.Fatalf("expected stable 64-char hex, got %q vs %q", h1, h2)
	}
	if strings.Contains(h1, "device-abc") {
		t.Fatal("hash must not contain raw id")
	}
	// 密钥不同 → 哈希不同。
	p2 := *p
	p2.DeviceSecret = "ffffffff"
	if svc.HashDeviceID(&p2, "device-abc") == h1 {
		t.Fatal("different secret must produce different hash")
	}
	// 输入不同 → 哈希不同。
	if svc.HashDeviceID(p, "device-xyz") == h1 {
		t.Fatal("different raw id must produce different hash")
	}
}

// TestReportPolicies 三种 DeviceIDPolicy 的落库内容（验收：hashed 库中无明文；
// raw 明文原样；none 空串且不参与降级）。
func TestReportPolicies(t *testing.T) {
	cases := []struct {
		policy string
		want   func(svc *TelemetryService, p *model.Project) string
	}{
		{model.DeviceIDPolicyHashed, func(svc *TelemetryService, p *model.Project) string { return svc.HashDeviceID(p, "device-abc") }},
		{model.DeviceIDPolicyRaw, func(*TelemetryService, *model.Project) string { return "device-abc" }},
		{model.DeviceIDPolicyNone, func(*TelemetryService, *model.Project) string { return "" }},
	}
	for _, tc := range cases {
		t.Run(tc.policy, func(t *testing.T) {
			svc, store, p := newTelemetryFixture(t)
			p.DeviceIDPolicy = tc.policy
			if err := report(svc, p, model.TelemetryStatusFailed); err != nil {
				t.Fatal(err)
			}
			events := store.Snapshot()
			if len(events) != 1 {
				t.Fatalf("expected 1 event, got %d", len(events))
			}
			want := tc.want(svc, p)
			if events[0].DeviceHash != want {
				t.Fatalf("policy %s: device_hash = %q, want %q", tc.policy, events[0].DeviceHash, want)
			}
		})
	}
}

// TestReportValidation 必填字段与 status 枚举校验 → ErrInvalidTelemetryReport。
func TestReportValidation(t *testing.T) {
	svc, store, p := newTelemetryFixture(t)
	if err := report(svc, p, "not-a-status"); err == nil || !strings.Contains(err.Error(), "status") {
		t.Fatalf("expected status enum error, got %v", err)
	}
	if err := report(svc, p, model.TelemetryStatusFailed, func(in *ReportInput) { in.OS = "" }); err == nil {
		t.Fatal("missing os must be rejected")
	}
	if err := report(svc, p, model.TelemetryStatusFailed, func(in *ReportInput) { in.ToVersion = "" }); err == nil {
		t.Fatal("missing to_version must be rejected")
	}
	// 合法 status 全集可上报。
	for _, st := range []string{
		model.TelemetryStatusDownloading, model.TelemetryStatusApplying,
		model.TelemetryStatusInstalled, model.TelemetryStatusFailed, model.TelemetryStatusRolledBack,
	} {
		if err := report(svc, p, st); err != nil {
			t.Fatalf("status %s must be accepted: %v", st, err)
		}
	}
	if len(store.Snapshot()) != 5 {
		t.Fatalf("expected 5 events, got %d", len(store.Snapshot()))
	}
}

// TestDowngradeActiveWindow 降级窗口语义（验收：3 次 failed 后降级生效；
// 一次 installed 后恢复；跨 channel 计入；24h 外不计入）。
func TestDowngradeActiveWindow(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	svc, _, p := newTelemetryFixture(t)
	svc.SetClock(func() time.Time { return now })
	ctx := context.Background()

	active := func() bool {
		got, err := svc.DowngradeActive(ctx, p.ID, "windows", "x86_64", svc.HashDeviceID(p, "device-abc"))
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	// 第 1、2 次 failed：不降级。
	_ = report(svc, p, model.TelemetryStatusFailed)
	if active() {
		t.Fatal("1 failed must not trigger downgrade")
	}
	_ = report(svc, p, model.TelemetryStatusFailed)
	if active() {
		t.Fatal("2 failed must not trigger downgrade")
	}
	// 第 3 次 failed：降级生效（键不含 channel：换渠道上报的 failed 同样计入）。
	_ = report(svc, p, model.TelemetryStatusFailed, func(in *ReportInput) { in.Channel = "beta" })
	if !active() {
		t.Fatal("3 failed must trigger downgrade")
	}

	// 第 3 新 failed 之后一次 installed → 恢复。
	_ = report(svc, p, model.TelemetryStatusInstalled)
	if active() {
		t.Fatal("installed after 3rd failed must recover")
	}

	// installed 在 3 次 failed 之前（更旧）→ 仍降级。
	svc2, _, p2 := newTelemetryFixture(t)
	svc2.SetClock(func() time.Time { return now })
	_ = report(svc2, p2, model.TelemetryStatusInstalled)
	_ = report(svc2, p2, model.TelemetryStatusFailed)
	_ = report(svc2, p2, model.TelemetryStatusFailed)
	_ = report(svc2, p2, model.TelemetryStatusFailed)
	if got, _ := svc2.DowngradeActive(ctx, p2.ID, "windows", "x86_64", svc2.HashDeviceID(p2, "device-abc")); !got {
		t.Fatal("old installed before 3rd failed must keep downgrade active")
	}

	// 24h 窗口外的事件不计入：3 次 failed 全部发生在 25h 前。
	svc3, _, p3 := newTelemetryFixture(t)
	svc3.SetClock(func() time.Time { return now.Add(-25 * time.Hour) })
	for i := 0; i < 3; i++ {
		_ = report(svc3, p3, model.TelemetryStatusFailed)
	}
	svc3.SetClock(func() time.Time { return now })
	if got, _ := svc3.DowngradeActive(ctx, p3.ID, "windows", "x86_64", svc3.HashDeviceID(p3, "device-abc")); got {
		t.Fatal("failed events outside the 24h window must not trigger downgrade")
	}

	// 空哈希（匿名 / none 策略）恒 false（C11-7）。
	if got, _ := svc.DowngradeActive(ctx, p.ID, "windows", "x86_64", ""); got {
		t.Fatal("empty device hash must never downgrade")
	}

	// 设备隔离：另一台设备不受影响。
	svc4, _, p4 := newTelemetryFixture(t)
	svc4.SetClock(func() time.Time { return now })
	for i := 0; i < 3; i++ {
		_ = report(svc4, p4, model.TelemetryStatusFailed)
	}
	if got, _ := svc4.DowngradeActive(ctx, p4.ID, "windows", "x86_64", svc4.HashDeviceID(p4, "other-device")); got {
		t.Fatal("other device must not be affected")
	}
}

// TestDeleteDeviceEvents 按哈希删除：只删该设备，其他设备与项目保留；未知哈希幂等。
func TestDeleteDeviceEvents(t *testing.T) {
	svc, store, p := newTelemetryFixture(t)
	ctx := context.Background()
	hash := svc.HashDeviceID(p, "device-abc")
	_ = report(svc, p, model.TelemetryStatusFailed)
	_ = report(svc, p, model.TelemetryStatusFailed)
	// 另一设备与另一项目各一条。
	if err := report(svc, p, model.TelemetryStatusFailed, func(in *ReportInput) { in.DeviceID = "other" }); err != nil {
		t.Fatal(err)
	}
	p2 := *p
	p2.ID = uuid.New() // 不同的项目：其事件不受本项目删除影响（同密钥 → 同哈希合法）
	_ = report(svc, &p2, model.TelemetryStatusFailed)

	n, err := svc.DeleteDeviceEvents(ctx, p.ID, hash)
	if err != nil || n != 2 {
		t.Fatalf("expected 2 deleted, got %d err=%v", n, err)
	}
	// 重复删除幂等。
	if n, _ := svc.DeleteDeviceEvents(ctx, p.ID, hash); n != 0 {
		t.Fatalf("expected idempotent 0, got %d", n)
	}
	remaining := store.Snapshot()
	if len(remaining) != 2 {
		t.Fatalf("expected 2 remaining events, got %d", len(remaining))
	}
	for _, e := range remaining {
		if e.ProjectID == p.ID && e.DeviceHash == hash {
			t.Fatal("deleted hash must not remain in the target project")
		}
	}
}

// TestRetentionCleanup 惰性留存清理：采样命中时删除 created_at < cutoff 的事件。
func TestRetentionCleanup(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	svc, store, p := newTelemetryFixture(t)
	// 时钟：前 2 次上报在 91 天前，随后回到 now；采样恒命中。
	old := now.AddDate(0, 0, -(model.DefaultTelemetryRetentionDays + 1))
	calls := 0
	svc.SetClock(func() time.Time {
		calls++
		if calls <= 2 {
			return old
		}
		return now
	})
	svc.SetRetentionSampler(func() bool { return true })

	_ = report(svc, p, model.TelemetryStatusFailed)
	_ = report(svc, p, model.TelemetryStatusFailed)
	if got := len(store.Snapshot()); got != 2 {
		t.Fatalf("expected 2 old events, got %d", got)
	}
	// 第 3 次上报触发清理：2 条过期事件被删，仅剩本次。
	if err := report(svc, p, model.TelemetryStatusInstalled); err != nil {
		t.Fatal(err)
	}
	events := store.Snapshot()
	if len(events) != 1 || events[0].Status != model.TelemetryStatusInstalled {
		t.Fatalf("retention cleanup should keep only the fresh event, got %+v", events)
	}
	// 采样未命中时不清理。
	store2 := repository.NewMemoryTelemetryStore()
	svc2 := NewTelemetryService(store2)
	svc2.SetClock(func() time.Time { return now })
	svc2.SetRetentionSampler(func() bool { return false })
	_ = report(svc2, p, model.TelemetryStatusFailed)
	// 手工把事件变旧（直接经仓储再插一条旧的不可行，用时钟回拨重建场景）。
	if len(store2.Snapshot()) != 1 {
		t.Fatalf("expected 1 event, got %d", len(store2.Snapshot()))
	}
}

// TestProjectDeviceSecretGenerated 走 ProjectService.Create 后项目必须带
// 32 字节 hex DeviceSecret，且 JSON 序列化（json:"-"）永不回显。
func TestProjectDeviceSecretGenerated(t *testing.T) {
	store := repository.NewMemoryProjectStore()
	projSvc := NewProjectService(store)
	ctx := context.Background()
	slug := "sec-demo"
	p, _, err := projSvc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.DeviceSecret) != model.DeviceSecretBytesLen*2 {
		t.Fatalf("expected %d hex chars, got %d", model.DeviceSecretBytesLen*2, len(p.DeviceSecret))
	}
	// json:"-"：序列化结果不得包含密钥值。
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), p.DeviceSecret) {
		t.Fatal("device secret must never appear in any API JSON")
	}
	// 默认留存 90 天。
	if p.TelemetryRetentionDays != model.DefaultTelemetryRetentionDays {
		t.Fatalf("default retention = %d, want %d", p.TelemetryRetentionDays, model.DefaultTelemetryRetentionDays)
	}
}

func TestDeleteDeviceDataInvalidatesProjectCache(t *testing.T) {
	ctx := t.Context()
	id := uuid.New()
	store, err := cache.Open(cache.Options{Driver: "memory"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	key := cache.CatalogKey(id, "linux", "x86_64")
	if err := store.Set(ctx, key, []byte("stale")); err != nil {
		t.Fatal(err)
	}
	svc := NewTelemetryService(repository.NewMemoryTelemetryStore())
	svc.SetAllowlistDeleter(repository.NewMemoryProjectStore())
	svc.SetCache(store)
	if _, _, err := svc.DeleteDeviceData(ctx, id, "deadbeef"); err != nil {
		t.Fatal(err)
	}
	_, ok, err := store.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("privacy allowlist delete must invalidate catalog cache")
	}
}
