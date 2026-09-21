package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/pkg/webhook"
)

// 本文件实现 Publish webhook（C14-3..C14-5，docs/app-init.md §5.8 / §11.5）：
//
//   - 事件：version.published（PublishVersion）、version_line.ready（已发布版本的
//     新平台切片就绪）、version.revoked（RevokeVersion）；
//   - 触发点只做「建 delivery 行 + 入队」两步毫秒级操作，永不阻塞发版 HTTP；
//     入队失败静默忽略（与 notifyLineReady 同策略，§11.5：容器出站被墙时发版仍成功）；
//   - 投递 Job webhook_deliver：拉取 delivery 行 → 组装信封 JSON → 带
//     X-KiriVers-Signature（HMAC-SHA256(Project.WebhookSecret, rawBody)）POST 到
//     Project.WebhookURL（10s 超时，自定义 UA）；
//   - 重试：最多 WebhookDeliveryMaxAttempts 次尝试，失败按 1m / 5m 退避
//     （Job.NextAttemptAt + JobStore.RequeueAfter），用尽后 delivery 终态 failed。
//     webhook 失败只影响投递记录，不影响 Version 已 Published 状态。

const (
	// webhookDeliverJobType 是 webhook 投递异步任务类型（父设计 job 类型清单既有）。
	webhookDeliverJobType = "webhook_deliver"
	// webhookHTTPTimeout 单次投递的 HTTP 超时（§11.5：出站被墙时不能拖垮 worker）。
	webhookHTTPTimeout = 10 * time.Second
	// webhookUserAgent 投递请求的自定义 UA，便于接收方审计。
	webhookUserAgent = "KiriVers-Webhook/1.0"
)

// webhookRetryBackoff 是第 1/2 次投递失败后的退避时长；第 3 次失败即终态 failed。
var webhookRetryBackoff = [2]time.Duration{time.Minute, 5 * time.Minute}

// webhookDeliverJobPayload 是 webhook_deliver 任务载荷。JobID 同时写入载荷与
// 行本身：handler 需要在失败重试时对同一行执行 RequeueAfter（退避），
// 而 JobHandler 签名不携带 job 行，故以载荷回传。
type webhookDeliverJobPayload struct {
	JobID      uuid.UUID `json:"job_id"`
	DeliveryID uuid.UUID `json:"delivery_id"`
}

// SetWebhookStore 设置 webhook 投递记录仓储。
func (s *ProjectService) SetWebhookStore(ws repository.WebhookStore) {
	s.webhooks = ws
}

// webhookVersionPayload 是事件 data 载荷的公共字段。
type webhookVersionPayload struct {
	VersionRef  string `json:"version_ref,omitempty"`
	Channel     string `json:"channel,omitempty"`
	Status      string `json:"status,omitempty"`
	PublishTime string `json:"publish_time,omitempty"`
}

// buildWebhookEventPayload 组装事件 data 载荷（失败返回 nil，触发点忽略）。
func (s *ProjectService) buildWebhookEventPayload(ctx context.Context, event string, versionID uuid.UUID, lineID *uuid.UUID) []byte {
	v, err := s.store.GetVersionByID(ctx, versionID)
	if err != nil {
		return nil
	}
	p := webhookVersionPayload{
		Channel: v.ChannelSlug,
		Status:  v.Status,
	}
	if v.VersionSemver != nil && *v.VersionSemver != "" {
		p.VersionRef = *v.VersionSemver
	} else if v.VersionInteger != nil {
		p.VersionRef = fmt.Sprintf("%d", *v.VersionInteger)
	}
	if event == model.WebhookEventVersionPublished && v.PublishTime != nil {
		p.PublishTime = v.PublishTime.UTC().Format(time.RFC3339)
	}
	if event == model.WebhookEventVersionLineReady && lineID != nil {
		// 追加平台切片字段：os / arch / root_hash（多文件线）。
		lines, lerr := s.store.ListVersionLines(ctx, versionID)
		if lerr == nil {
			for i := range lines {
				if lines[i].ID != *lineID {
					continue
				}
				merged, merr := json.Marshal(struct {
					webhookVersionPayload
					OS       string `json:"os"`
					Arch     string `json:"arch"`
					RootHash string `json:"root_hash,omitempty"`
				}{p, lines[i].OS, lines[i].Arch, lines[i].RootHash})
				if merr == nil {
					return merged
				}
				break
			}
		}
	}
	out, err := json.Marshal(p)
	if err != nil {
		return nil
	}
	return out
}

// NotifyWebhookEvent 为项目触发一次 webhook 事件（C14-3）：建 delivery 行并入队
// webhook_deliver 任务。任何错误都只影响投递，绝不向调用方传播（§11.5）。
// 项目未配置 WebhookURL 时零开销直接返回（无行、无任务）。
func (s *ProjectService) NotifyWebhookEvent(ctx context.Context, projectID uuid.UUID, event string, versionID uuid.UUID, lineID *uuid.UUID) {
	s.notifyWebhookEvent(ctx, projectID, event, versionID, lineID)
}

// notifyWebhookEvent 是 NotifyWebhookEvent 的内部实现，错误静默吞掉。
func (s *ProjectService) notifyWebhookEvent(ctx context.Context, projectID uuid.UUID, event string, versionID uuid.UUID, lineID *uuid.UUID) {
	if s.webhooks == nil || s.jobs == nil {
		return
	}
	proj, err := s.store.GetByID(ctx, projectID)
	if err != nil || proj == nil || proj.WebhookURL == nil || *proj.WebhookURL == "" {
		// 未配置 webhook：零开销路径（§11.5 附加能力，缺省不发）。
		return
	}
	payload := s.buildWebhookEventPayload(ctx, event, versionID, lineID)
	if payload == nil {
		return
	}

	vid := versionID
	var linePtr *uuid.UUID
	if lineID != nil {
		l := *lineID
		linePtr = &l
	}
	delivery := &model.WebhookDelivery{
		ProjectID: projectID,
		Event:     event,
		VersionID: &vid,
		LineID:    linePtr,
		Payload:   payload,
		Status:    model.WebhookDeliveryStatusPending,
	}
	if err := s.webhooks.CreateDelivery(ctx, delivery); err != nil {
		// 入队失败仅吞掉（§11.5：webhook 是附加能力，失败不影响发版主流程）。
		return
	}

	jobID := uuid.New()
	jobPayload, err := json.Marshal(webhookDeliverJobPayload{JobID: jobID, DeliveryID: delivery.ID})
	if err != nil {
		return
	}
	job := &model.Job{
		ID:          jobID,
		Type:        webhookDeliverJobType,
		Status:      model.JobStatusQueued,
		Payload:     jobPayload,
		ProjectID:   &projectID,
		OwnerNodeID: s.ownerNodePtr(),
	}
	// 投递任务不做 24h 幂等去重：事件语义是「至多一次记录 + 至少一次尝试」，
	// 重复投递由接收方按签名体内的 event/version 组合自行幂等。
	if err := s.jobs.Create(ctx, job); err != nil {
		return
	}
}

// notifyLineReadyWebhook 在「已发布版本的新平台切片就绪」（补平台场景）时触发
// version_line.ready 事件（C14-3，§5.8）。Draft 版本的线就绪不发：
// 齐套后 PublishVersion 的 version.published 事件已覆盖该语义。
func (s *ProjectService) notifyLineReadyWebhook(ctx context.Context, projectID uuid.UUID, v *model.Version, line *model.VersionLine) {
	if v == nil || line == nil || v.Status != model.VersionStatusPublished {
		return
	}
	s.notifyWebhookEvent(ctx, projectID, model.WebhookEventVersionLineReady, v.ID, &line.ID)
}

// ListWebhookDeliveries 按项目游标分页列出投递记录，供管理端排障查询（C14-4）。
func (s *ProjectService) ListWebhookDeliveries(ctx context.Context, projectID uuid.UUID, cursor *uuid.UUID, limit int) ([]model.WebhookDelivery, error) {
	if s.webhooks == nil {
		return nil, fmt.Errorf("%w: webhook store is not configured", ErrStorageUnavailable)
	}
	return s.webhooks.ListDeliveries(ctx, projectID, cursor, limit)
}

// RegisterWebhookDeliverJobHandler 为 JobWorker 注册 webhook_deliver 处理器。
// 处理器自行管理重试与终态：可重试失败 → RequeueAfter 退避并返回 nil；
// 终态失败 → delivery 标 failed 并返回 nil（任务行不算失败，避免与投递状态双重记账）；
// 仅基础设施错误（delivery/项目缺失、载荷损坏）返回非 nil 交由 worker MarkFailed。
func (s *ProjectService) RegisterWebhookDeliverJobHandler(w *JobWorker) {
	if w == nil {
		return
	}
	w.RegisterHandler(webhookDeliverJobType, func(ctx context.Context, jobType string, payload []byte) error {
		var p webhookDeliverJobPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return err
		}
		if p.JobID == uuid.Nil {
			return fmt.Errorf("webhook_deliver payload missing job_id")
		}
		return s.runWebhookDeliver(ctx, p.JobID, p.DeliveryID)
	})
}

// runWebhookDeliver 执行一次投递尝试并决定重试/终态。
func (s *ProjectService) runWebhookDeliver(ctx context.Context, jobID, deliveryID uuid.UUID) error {
	if s.webhooks == nil {
		return fmt.Errorf("webhook store is not configured")
	}
	delivery, err := s.webhooks.GetDelivery(ctx, deliveryID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: %s", ErrWebhookDeliveryNotFound, deliveryID)
		}
		return err
	}
	// 终态行被重复投递（幂等防御）：直接成功返回，不再发送。
	if delivery.Status != model.WebhookDeliveryStatusPending {
		return nil
	}
	proj, err := s.store.GetByID(ctx, delivery.ProjectID)
	if err != nil {
		return err
	}
	if proj.WebhookURL == nil || *proj.WebhookURL == "" {
		// URL 已被移除：记录终态 failed，避免行永久 pending。
		delivery.Status = model.WebhookDeliveryStatusFailed
		delivery.LastError = "webhook url removed before delivery"
		return s.webhooks.UpdateDelivery(ctx, delivery)
	}

	attemptErr := s.postWebhookDelivery(ctx, proj, delivery)

	delivery.Attempts++
	if attemptErr == nil {
		now := time.Now().UTC()
		delivery.Status = model.WebhookDeliveryStatusDelivered
		delivery.LastError = ""
		delivery.DeliveredAt = &now
		return s.webhooks.UpdateDelivery(ctx, delivery)
	}

	delivery.LastError = attemptErr.Error()
	if delivery.Attempts >= model.WebhookDeliveryMaxAttempts {
		// 重试用尽（C14-4）：投递终态 failed；发版状态不受影响（§11.5）。
		delivery.Status = model.WebhookDeliveryStatusFailed
		return s.webhooks.UpdateDelivery(ctx, delivery)
	}
	if err := s.webhooks.UpdateDelivery(ctx, delivery); err != nil {
		return err
	}
	// 退避重试：1m / 5m（C14-4）。返回 nil 让 worker 不把任务行标 failed。
	return s.jobs.RequeueAfter(ctx, jobID, webhookRetryBackoff[delivery.Attempts-1])
}

// postWebhookDelivery 执行一次带签名的 HTTP POST（10s 超时）。
// 2xx 视为成功；网络错误与非 2xx 返回错误，状态码写入 delivery.LastStatusCode。
func (s *ProjectService) postWebhookDelivery(ctx context.Context, proj *model.Project, delivery *model.WebhookDelivery) error {
	envelope := struct {
		Event     string          `json:"event"`
		ProjectID uuid.UUID       `json:"project_id"`
		VersionID *uuid.UUID      `json:"version_id,omitempty"`
		LineID    *uuid.UUID      `json:"line_id,omitempty"`
		Data      json.RawMessage `json:"data"`
	}{
		Event:     delivery.Event,
		ProjectID: delivery.ProjectID,
		VersionID: delivery.VersionID,
		LineID:    delivery.LineID,
		Data:      json.RawMessage(delivery.Payload),
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal webhook body: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, webhookHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, *proj.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", webhookUserAgent)
	// 签名头（C14-3）：HMAC-SHA256(WebhookSecret, rawBody)；接收方按 raw body 校验。
	req.Header.Set(webhook.HeaderName, webhook.Sign(proj.WebhookSecret, body))

	resp, err := webhookHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("post webhook: %w", err)
	}
	defer resp.Body.Close()
	// 丢弃响应体但必须读完以复用连接；体积上限防御恶意接收方。
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
	delivery.LastStatusCode = resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("webhook endpoint returned status %d", resp.StatusCode)
	}
	return nil
}

// webhookHTTPClient 投递专用 client：10s 整体超时在请求 ctx 上控制，
// 这里仅限制连接与响应头阶段，避免被恶意端点长期挂住。
var webhookHTTPClient = &http.Client{
	Timeout: webhookHTTPTimeout,
}

// ExecuteWebhookDelivery 直接对指定 delivery 执行一次投递尝试（含重试决策），
// 供测试与手动补偿调用。返回 delivery 终态是否已 failed。
func (s *ProjectService) ExecuteWebhookDelivery(ctx context.Context, jobID, deliveryID uuid.UUID) error {
	return s.runWebhookDeliver(ctx, jobID, deliveryID)
}
