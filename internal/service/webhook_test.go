package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/storage"
	"github.com/Kirizu-Official/KiriVers/pkg/webhook"
)

// webhookHarness 装配带 webhook 依赖的服务。
type webhookHarness struct {
	svc       *ProjectService
	store     repository.ProjectStore
	webhooks  *repository.MemoryWebhookStore
	jobs      *repository.MemoryJobRepo
	hookCalls int
	mu        sync.Mutex
}

// newWebhookHarness 构造服务并注册 webhook 投递仓储与任务仓储。
func newWebhookHarness(t *testing.T, handler http.HandlerFunc) (*webhookHarness, *httptest.Server) {
	t.Helper()
	store := repository.NewMemoryProjectStore()
	backend, err := storage.NewLocalFS(t.TempDir())
	if err != nil {
		t.Fatalf("setup storage: %v", err)
	}
	svc := NewProjectService(store, backend)
	h := &webhookHarness{
		svc:      svc,
		store:    store,
		webhooks: repository.NewMemoryWebhookStore(),
		jobs:     repository.NewMemoryJobRepo(),
	}
	svc.SetWebhookStore(h.webhooks)
	svc.SetJobStore(h.jobs)

	var ts *httptest.Server
	if handler != nil {
		ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h.mu.Lock()
			h.hookCalls++
			h.mu.Unlock()
			handler(w, r)
		}))
		t.Cleanup(ts.Close)
	}
	return h, ts
}

// publishWithWebhook 创建项目（可带 webhook URL）、版本、产物并发布，返回项目与版本。
func (h *webhookHarness) publishWithWebhook(t *testing.T, webhookURL string) (*model.Project, *model.Version) {
	t.Helper()
	ctx := context.Background()
	slug := "hook-proj"
	p, _, err := h.svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if webhookURL != "" {
		patched, _, err := h.svc.Patch(ctx, p.Slug, PatchProjectInput{WebhookURL: &webhookURL})
		if err != nil {
			t.Fatalf("configure webhook: %v", err)
		}
		p = patched
	}
	if p.WebhookSecret == "" && webhookURL != "" {
		t.Fatalf("webhook secret must be generated when url configured")
	}

	if _, _, err := h.svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatalf("put version: %v", err)
	}
	body := []byte("payload")
	if _, err := h.svc.UploadArtifact(ctx, p.Slug, "1.0.0", "windows", "x86_64", UploadArtifactInput{
		Filename: "app.exe",
		Size:     int64(len(body)),
	}, bytes.NewReader(body)); err != nil {
		t.Fatalf("upload: %v", err)
	}
	v, err := h.svc.PublishVersion(ctx, p.ID, "1.0.0")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	return p, v
}

// drainJobs 模拟 worker：不断 claim 并处理任务直到队列为空。webhook_deliver
// 任务执行投递并计数；auto_delta 等其它任务（不在本测试面）直接标成功跳过。
func (h *webhookHarness) drainJobs(t *testing.T, max int) int {
	t.Helper()
	ctx := context.Background()
	n := 0
	// 已见的 webhook 任务（退避中的任务对 Claim 不可见；测试里直接清除
	// next_attempt_at 以立即执行下一次尝试）。
	seen := map[uuid.UUID]bool{}
	for n < max {
		job, err := h.jobs.Claim(ctx, repository.ClaimFilter{})
		if err != nil {
			t.Fatalf("claim job: %v", err)
		}
		if job == nil {
			// 无可抢行：把处于退避期的 webhook 任务唤醒继续测试。
			progressed := false
			for id := range seen {
				j, gerr := h.jobs.GetByID(ctx, id)
				if gerr != nil || j.NextAttemptAt == nil {
					continue
				}
				j.NextAttemptAt = nil
				if uerr := h.jobs.Update(ctx, j); uerr != nil {
					t.Fatalf("clear backoff: %v", uerr)
				}
				progressed = true
				break
			}
			if !progressed {
				return n
			}
			continue
		}
		if job.Type != webhookDeliverJobType {
			// 其它任务类型（auto_delta 等）不在 webhook 测试面，直接完成以出队。
			if err := h.jobs.MarkSucceeded(ctx, job.ID); err != nil {
				t.Fatalf("skip job: %v", err)
			}
			continue
		}
		seen[job.ID] = true
		var p webhookDeliverJobPayload
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if err := h.svc.ExecuteWebhookDelivery(ctx, job.ID, p.DeliveryID); err != nil {
			t.Fatalf("execute delivery: %v", err)
		}
		n++
	}
	return n
}

// TestWebhook_Endpoint500PublishStillSucceeds 验收 4 + 5：webhook 端点 500 时
// Publish 仍成功；delivery 重试 3 次后终态 failed。
func TestWebhook_Endpoint500PublishStillSucceeds(t *testing.T) {
	h, ts := newWebhookHarness(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	p, _ := h.publishWithWebhook(t, ts.URL)

	// Publish 不受 webhook 端点故障影响（§11.5）：版本已 Published，HTTP 层面即 200。
	v, err := h.svc.ResolveVersion(context.Background(), p.ID, "1.0.0")
	if err != nil || v.Status != model.VersionStatusPublished {
		t.Fatalf("version must be published despite webhook failure: %v %s", err, v.Status)
	}

	// delivery 行已建（pending），任务已入队。
	deliveries, err := h.webhooks.ListDeliveries(context.Background(), p.ID, nil, 10)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery row, got %d (%v)", len(deliveries), err)
	}
	d := deliveries[0]
	if d.Event != model.WebhookEventVersionPublished || d.Status != model.WebhookDeliveryStatusPending {
		t.Fatalf("unexpected delivery: %+v", d)
	}

	// 3 次尝试（首投 + 2 次退避重试）后终态 failed。
	n := h.drainJobs(t, 10)
	if n != model.WebhookDeliveryMaxAttempts {
		t.Fatalf("expected %d attempts, got %d", model.WebhookDeliveryMaxAttempts, n)
	}
	final, err := h.webhooks.GetDelivery(context.Background(), d.ID)
	if err != nil {
		t.Fatalf("get delivery: %v", err)
	}
	if final.Status != model.WebhookDeliveryStatusFailed {
		t.Fatalf("delivery must be failed after retries, got %s", final.Status)
	}
	if final.Attempts != model.WebhookDeliveryMaxAttempts {
		t.Fatalf("attempts mismatch: %d", final.Attempts)
	}
	if final.LastStatusCode != http.StatusInternalServerError {
		t.Fatalf("last status code must be 500, got %d", final.LastStatusCode)
	}
	h.mu.Lock()
	calls := h.hookCalls
	h.mu.Unlock()
	if calls != model.WebhookDeliveryMaxAttempts {
		t.Fatalf("endpoint hit %d times, want %d", calls, model.WebhookDeliveryMaxAttempts)
	}
}

// TestWebhook_DeliverySuccessSignatureHeader 成功投递：签名头可用项目密钥对
// 原始 body 校验通过；篡改 body 后验签失败（与 pkg/webhook 单测互补的端到端面）。
func TestWebhook_DeliverySuccessSignatureHeader(t *testing.T) {
	var mu sync.Mutex
	var gotSig string
	var gotBody []byte
	h, ts := newWebhookHarness(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotSig = r.Header.Get(webhook.HeaderName)
		gotBody, _ = io.ReadAll(r.Body)
		if r.Header.Get("User-Agent") != webhookUserAgent {
			t.Errorf("missing custom user agent")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("missing json content type")
		}
		w.WriteHeader(http.StatusOK)
	})

	p, _ := h.publishWithWebhook(t, ts.URL)
	if n := h.drainJobs(t, 5); n != 1 {
		t.Fatalf("expected exactly 1 delivery attempt, got %d", n)
	}

	deliveries, _ := h.webhooks.ListDeliveries(context.Background(), p.ID, nil, 10)
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}
	d := deliveries[0]
	if d.Status != model.WebhookDeliveryStatusDelivered || d.DeliveredAt == nil {
		t.Fatalf("delivery must be delivered: %+v", d)
	}
	if d.LastStatusCode != http.StatusOK {
		t.Fatalf("last status code must be 200, got %d", d.LastStatusCode)
	}

	// 签名校验：正确密钥 + 原始 body 通过。
	if gotSig == "" {
		t.Fatalf("missing signature header")
	}
	if !webhook.Verify(p.WebhookSecret, gotBody, gotSig) {
		t.Fatalf("signature must verify with project secret")
	}
	// 信封语义。
	var env struct {
		Event     string          `json:"event"`
		ProjectID string          `json:"project_id"`
		VersionID string          `json:"version_id"`
		Data      json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(gotBody, &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if env.Event != model.WebhookEventVersionPublished || env.VersionID != d.VersionID.String() {
		t.Fatalf("unexpected envelope: %+v", env)
	}
}

// TestWebhook_TamperedBodyFailsVerify 接收方视角：篡改 body 后用同一密钥验签失败。
func TestWebhook_TamperedBodyFailsVerify(t *testing.T) {
	var mu sync.Mutex
	var gotSig string
	var gotBody []byte
	h, ts := newWebhookHarness(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotSig = r.Header.Get(webhook.HeaderName)
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	})

	p, _ := h.publishWithWebhook(t, ts.URL)
	h.drainJobs(t, 5)

	// 中途篡改 payload 再验签必须失败。
	tampered := append([]byte{}, gotBody...)
	tampered[len(tampered)-2] ^= 0xFF
	if webhook.Verify(p.WebhookSecret, tampered, gotSig) {
		t.Fatalf("tampered body must fail verification")
	}
}

// TestWebhook_NoURLZeroOverhead 未配置 webhook URL 时零开销：无 delivery 行、无任务。
func TestWebhook_NoURLZeroOverhead(t *testing.T) {
	h, _ := newWebhookHarness(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("endpoint must not be called without webhook url")
	})

	h.publishWithWebhook(t, "")

	deliveries, _ := h.webhooks.ListDeliveries(context.Background(), uuid.Nil, nil, 10)
	if len(deliveries) != 0 {
		t.Fatalf("no delivery rows expected, got %d", len(deliveries))
	}
	// 队列里可能存在 auto_delta 等任务，但绝不应有 webhook_deliver 任务。
	for {
		job, _ := h.jobs.Claim(context.Background(), repository.ClaimFilter{})
		if job == nil {
			break
		}
		if job.Type == webhookDeliverJobType {
			t.Fatalf("no webhook_deliver job expected without webhook url")
		}
		_ = h.jobs.MarkSucceeded(context.Background(), job.ID)
	}
}

// TestWebhook_RevokedAndLineReadyEvents 吊销与已发布版本补平台线就绪事件均会
// 入队并成功投递。
func TestWebhook_RevokedAndLineReadyEvents(t *testing.T) {
	var mu sync.Mutex
	var events []string
	h, ts := newWebhookHarness(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var env struct {
			Event string `json:"event"`
		}
		_ = json.Unmarshal(body, &env)
		mu.Lock()
		events = append(events, env.Event)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})

	p, v := h.publishWithWebhook(t, ts.URL)
	ctx := context.Background()

	// 补平台：已发布版本新增 linux/x86_64 线（复用产物直接 ready）。
	arts, _ := h.store.ListArtifactsByVersionID(ctx, v.ID)
	if len(arts) == 0 {
		t.Fatalf("source artifact missing")
	}
	srcID := arts[0].ID
	if _, err := h.svc.ReuseArtifacts(ctx, p.Slug, "1.0.0", "linux", "x86_64", ReuseArtifactsInput{
		OS:     "linux",
		Arch:   "x86_64",
		Source: ReuseSource{ArtifactID: &srcID},
	}); err != nil {
		t.Fatalf("reuse for new platform: %v", err)
	}

	// 吊销。
	if _, err := h.svc.RevokeVersion(ctx, p.ID, "1.0.0"); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	// 首个 published 事件 + line.ready + revoked = 3 条投递。
	if n := h.drainJobs(t, 10); n != 3 {
		t.Fatalf("expected 3 deliveries, got %d", n)
	}
	mu.Lock()
	defer mu.Unlock()
	want := map[string]int{
		model.WebhookEventVersionPublished: 1,
		model.WebhookEventVersionLineReady: 1,
		model.WebhookEventVersionRevoked:   1,
	}
	got := map[string]int{}
	for _, e := range events {
		got[e]++
	}
	for ev, n := range want {
		if got[ev] != n {
			t.Fatalf("event %s delivered %d times, want %d (got %v)", ev, got[ev], n, got)
		}
	}
}
