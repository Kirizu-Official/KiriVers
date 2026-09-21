package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/platform"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
)

var (
	// ErrClientNotFound 名册行不存在 → HTTP 404。
	ErrClientNotFound = errors.New("client not found")
	// ErrClientPolicyNone 项目 device_id_policy=none，拒绝登录建档 → HTTP 400。
	ErrClientPolicyNone = errors.New("device_id policy 'none' does not support client registry")
	// ErrClientDeviceRequired 登录缺少 device_id → HTTP 400。
	ErrClientDeviceRequired = errors.New("device_id is required")
)

const maxClientCustomDepth = 8

// ClientLoginInput 是客户端登录 API 的输入。
type ClientLoginInput struct {
	DeviceID string
	Version  string
	OS       string
	Arch     string
	Channel  string
	IP       string
	Custom   model.JSONObject
}

// ClientListQuery 是管理端名册列表筛选。
type ClientListQuery struct {
	Q           string
	OS          string
	Arch        string
	Version     string
	ActiveSince *time.Time
	Limit       int
	Offset      int
}

// LoginClient 按 device_id 策略哈希后 upsert JSON + 运行字段。
// none 策略拒绝；匿名（空 device_id）拒绝。成功后推进本项目进行中灰度。
func (s *ProjectService) LoginClient(ctx context.Context, project *model.Project, in ClientLoginInput) (*model.Client, error) {
	if project == nil {
		return nil, ErrProjectNotFound
	}
	if project.DeviceIDPolicy == model.DeviceIDPolicyNone {
		return nil, ErrClientPolicyNone
	}
	raw := strings.TrimSpace(in.DeviceID)
	if raw == "" {
		return nil, ErrClientDeviceRequired
	}
	hash, err := storedClientDeviceHash(project, raw)
	if err != nil {
		return nil, err
	}
	if err := validateClientCustom(in.Custom); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	row := &model.Client{
		ProjectID:   project.ID,
		DeviceHash:  hash,
		LastVersion: strings.TrimSpace(in.Version),
		LastOS:      platform.CanonicalOSWrite(in.OS),
		LastArch:    platform.CanonicalArch(in.Arch),
		LastChannel: strings.TrimSpace(in.Channel),
		LastIP:      strings.TrimSpace(in.IP),
	}
	s.applyClientGeo(ctx, row)
	out, _, err := s.store.UpsertClientLogin(ctx, row, in.Custom, now)
	if err != nil {
		return nil, err
	}
	_ = s.EnsureProjectGrayAdmission(ctx, project)
	return out, nil
}

// TouchClientCheck 带 device_id 的 update/check：只 upsert 运行字段，不改 JSON。
// none / 空 device_id 不建档。随后推进进行中灰度。
func (s *ProjectService) TouchClientCheck(ctx context.Context, project *model.Project, deviceID, version, os, arch, channel, ip string) {
	if project == nil || project.DeviceIDPolicy == model.DeviceIDPolicyNone {
		_ = s.EnsureProjectGrayAdmission(ctx, project)
		return
	}
	raw := strings.TrimSpace(deviceID)
	if raw == "" {
		_ = s.EnsureProjectGrayAdmission(ctx, project)
		return
	}
	hash, err := storedClientDeviceHash(project, raw)
	if err != nil || hash == "" {
		_ = s.EnsureProjectGrayAdmission(ctx, project)
		return
	}
	now := time.Now().UTC()
	row := &model.Client{
		ProjectID:   project.ID,
		DeviceHash:  hash,
		LastVersion: strings.TrimSpace(version),
		LastOS:      platform.CanonicalOSWrite(os),
		LastArch:    platform.CanonicalArch(arch),
		LastChannel: strings.TrimSpace(channel),
		LastIP:      strings.TrimSpace(ip),
	}
	s.applyClientGeo(ctx, row)
	_, _, _ = s.store.UpsertClientCheck(ctx, row, now)
	_ = s.EnsureProjectGrayAdmission(ctx, project)
}

// GetClient 按项目内 UUID 取名册行。
func (s *ProjectService) GetClient(ctx context.Context, projectID, id uuid.UUID) (*model.Client, error) {
	c, err := s.store.GetClientByID(ctx, projectID, id)
	if err != nil {
		return nil, ErrClientNotFound
	}
	return c, nil
}

// DeleteClient 删除名册行，并清掉该 device_hash 在本项目全部灰度白名单。
func (s *ProjectService) DeleteClient(ctx context.Context, projectID, id uuid.UUID) error {
	cl, err := s.store.GetClientByID(ctx, projectID, id)
	if err != nil {
		return ErrClientNotFound
	}
	if _, err := s.store.DeleteAllowlistByDeviceID(ctx, projectID, cl.DeviceHash); err != nil {
		return err
	}
	if err := s.store.DeleteClient(ctx, projectID, id); err != nil {
		return ErrClientNotFound
	}
	s.invalidateProject(ctx, projectID)
	return nil
}

func (s *ProjectService) applyClientGeo(ctx context.Context, row *model.Client) {
	if row == nil {
		return
	}
	existing, err := s.store.GetClientByDeviceHash(ctx, row.ProjectID, row.DeviceHash)
	if err == nil && existing != nil && strings.TrimSpace(existing.LastIP) == strings.TrimSpace(row.LastIP) {
		row.CountryCode = existing.CountryCode
		row.RegionCode = existing.RegionCode
		row.GeoI18n = existing.GeoI18n.Clone()
		return
	}
	if s.geoip == nil {
		return
	}
	res := s.geoip(ctx, row.LastIP)
	row.CountryCode = res.CountryCode
	row.RegionCode = res.RegionCode
	row.GeoI18n = res.GeoI18n.Clone()
}

// ListClients 管理端名册列表。
func (s *ProjectService) ListClients(ctx context.Context, projectID uuid.UUID, q ClientListQuery) ([]model.Client, int64, error) {
	return s.store.ListClients(ctx, projectID, repository.ClientListFilter{
		Q: q.Q, OS: q.OS, Arch: q.Arch, Version: q.Version,
		ActiveSince: q.ActiveSince, Limit: q.Limit, Offset: q.Offset,
	})
}

// ClientBucketStats 客户端列表页聚合。
func (s *ProjectService) ClientBucketStats(ctx context.Context, projectID uuid.UUID) (repository.ClientBuckets, error) {
	return s.store.ClientBuckets(ctx, projectID, time.Now().UTC())
}

// ClientDailySeries 概览折线：每日新设备与活跃。
func (s *ProjectService) ClientDailySeries(ctx context.Context, projectID uuid.UUID, from, to time.Time) ([]model.ClientDailyStats, error) {
	if to.Before(from) {
		from, to = to, from
	}
	return s.store.ListClientDailyStats(ctx, projectID, from, to)
}

// TelemetryDayPoint 是概览 7 日安装成败柱的一天计数。
type TelemetryDayPoint struct {
	Installed int64
	Failed    int64
}

// TelemetryDailySeries 按 UTC 日聚合 installed/failed。telemetry 仓储未注入时返回空 map。
func (s *ProjectService) TelemetryDailySeries(ctx context.Context, projectID uuid.UUID, from, to time.Time) (map[string]TelemetryDayPoint, error) {
	out := map[string]TelemetryDayPoint{}
	if s.telemetry == nil {
		return out, nil
	}
	if to.Before(from) {
		from, to = to, from
	}
	rows, err := s.telemetry.CountStatusByDay(ctx, projectID, from, to, []string{
		model.TelemetryStatusInstalled, model.TelemetryStatusFailed,
	})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		day := row.Day.UTC().Format("2006-01-02")
		cur := out[day]
		switch row.Status {
		case model.TelemetryStatusInstalled:
			cur.Installed += row.Count
		case model.TelemetryStatusFailed:
			cur.Failed += row.Count
		}
		out[day] = cur
	}
	return out, nil
}

// ClientUpdatedTo 名册 last_version 是否已达到目标版本比较键（D5）。
func ClientUpdatedTo(engine, lastVersion string, target *model.Version) bool {
	if target == nil || strings.TrimSpace(lastVersion) == "" {
		return false
	}
	ref, err := ParseVersionRef(lastVersion)
	if err != nil {
		return false
	}
	cur := &model.Version{}
	if ref.IsInteger {
		cur.VersionInteger = &ref.Integer
	} else {
		sv := ref.Semver
		cur.VersionSemver = &sv
		cur.VersionSemverCanonical = &sv
	}
	cmp, ok := update.CompareVersions(engine, cur, target)
	return ok && cmp >= 0
}

func storedClientDeviceHash(project *model.Project, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if utf8.RuneCountInString(raw) > model.MaxDeviceIDRunes {
		return "", fmt.Errorf("%w: device_id length must be 1..%d", ErrInvalidAllowlistEntry, model.MaxDeviceIDRunes)
	}
	switch project.DeviceIDPolicy {
	case model.DeviceIDPolicyNone:
		return "", nil
	case model.DeviceIDPolicyRaw:
		return raw, nil
	default:
		return HashDeviceID(project, raw), nil
	}
}

func validateClientCustom(custom model.JSONObject) error {
	if custom == nil {
		return nil
	}
	b, err := json.Marshal(custom)
	if err != nil {
		return ErrInvalidRequest("custom must be a JSON object")
	}
	if len(b) > model.MaxClientCustomJSONBytes {
		return ErrInvalidRequest("custom exceeds 16 KiB")
	}
	if err := maxJSONDepth(custom, 0, maxClientCustomDepth); err != nil {
		return err
	}
	return nil
}

func maxJSONDepth(v any, depth, max int) error {
	if depth > max {
		return ErrInvalidRequest("custom JSON nesting exceeds limit")
	}
	switch t := v.(type) {
	case map[string]any:
		for _, child := range t {
			if err := maxJSONDepth(child, depth+1, max); err != nil {
				return err
			}
		}
	case model.JSONObject:
		for _, child := range t {
			if err := maxJSONDepth(child, depth+1, max); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range t {
			if err := maxJSONDepth(child, depth+1, max); err != nil {
				return err
			}
		}
	}
	return nil
}
