package service

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// TestPromote_SuffixConflictForbiddenThenReuseFlow 验收 2：
// beta `1.2.3-beta.1` 不能直接改 channel=stable（后缀冲突 400）；
// 可新建更高比较键的 `1.2.3`（stable）并复用同一存储对象（零拷贝）。
func TestPromote_SuffixConflictForbiddenThenReuseFlow(t *testing.T) {
	svc, _, _, root := setupReuseService(t)
	ctx := context.Background()

	slug := "promote-flow"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	if _, _, err := svc.PutVersion(ctx, p.ID, "1.2.3-beta.1", VersionWriteInput{Channel: "beta"}); err != nil {
		t.Fatalf("put beta version: %v", err)
	}
	content := []byte("beta build payload")
	src, err := svc.UploadArtifact(ctx, p.Slug, "1.2.3-beta.1", "windows", "x86_64", UploadArtifactInput{
		Filename: "app.exe",
		Size:     int64(len(content)),
	}, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if _, err := svc.PublishVersion(ctx, p.ID, "1.2.3-beta.1"); err != nil {
		t.Fatalf("publish beta: %v", err)
	}

	// 后缀冲突：1.2.3-beta.1 → stable 违反 §4.3（stable 禁止预发布后缀）。
	_, err = svc.PromoteVersion(ctx, p.ID, "1.2.3-beta.1", PromoteVersionInput{TargetChannel: "stable"})
	var suffixErr *ChannelSuffixMismatchError
	if !errors.As(err, &suffixErr) || !errors.Is(err, ErrChannelSuffixMismatch) {
		t.Fatalf("expected CHANNEL_SUFFIX_MISMATCH, got %v", err)
	}

	objectsBefore := countStorageObjects(root)

	// 修复路径：新建更高比较键 Version（1.2.3 无后缀，比较键天然高于 1.2.3-beta.1）
	// 并复用 blob（§4.3 / §5.8）。
	if _, _, err := svc.PutVersion(ctx, p.ID, "1.2.3", VersionWriteInput{Channel: "stable"}); err != nil {
		t.Fatalf("put stable 1.2.3: %v", err)
	}
	srcID := src.ID
	reused, err := svc.ReuseArtifacts(ctx, p.Slug, "1.2.3", "windows", "x86_64", ReuseArtifactsInput{
		OS:     "windows",
		Arch:   "x86_64",
		Source: ReuseSource{ArtifactID: &srcID},
	})
	if err != nil {
		t.Fatalf("reuse artifacts onto new stable version: %v", err)
	}
	if reused.Artifacts[0].StorageKey != src.StorageKey {
		t.Fatalf("reuse must reference the same storage object")
	}
	if got := countStorageObjects(root); got != objectsBefore {
		t.Fatalf("storage object count must not change: before=%d after=%d", objectsBefore, got)
	}
	if _, err := svc.PublishVersion(ctx, p.ID, "1.2.3"); err != nil {
		t.Fatalf("publish stable 1.2.3: %v", err)
	}
}

// TestPromote_NoPrereleaseDirectChange 验收 3：无预发布后缀的 beta 版本
// （1.2.3 on beta）在规则允许时可直接改 channel=stable。
func TestPromote_NoPrereleaseDirectChange(t *testing.T) {
	svc, store, _, _ := setupReuseService(t)
	ctx := context.Background()

	slug := "promote-direct"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	if _, _, err := svc.PutVersion(ctx, p.ID, "1.2.3", VersionWriteInput{Channel: "beta"}); err != nil {
		t.Fatalf("put beta 1.2.3: %v", err)
	}
	content := []byte("payload")
	if _, err := svc.UploadArtifact(ctx, p.Slug, "1.2.3", "windows", "x86_64", UploadArtifactInput{
		Filename: "app.exe",
		Size:     int64(len(content)),
	}, bytes.NewReader(content)); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if _, err := svc.PublishVersion(ctx, p.ID, "1.2.3"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	linesBefore, _ := store.ListVersionLines(ctx, mustVersionID(t, svc, p.ID, "1.2.3"))
	stableCh, err := store.GetChannel(ctx, p.ID, model.ChannelStable)
	if err != nil {
		t.Fatalf("get stable channel: %v", err)
	}

	v, err := svc.PromoteVersion(ctx, p.ID, "1.2.3", PromoteVersionInput{TargetChannel: model.ChannelStable})
	if err != nil {
		t.Fatalf("promote beta(1.2.3) to stable: %v", err)
	}
	if v.ChannelSlug != model.ChannelStable {
		t.Fatalf("channel slug must be stable, got %s", v.ChannelSlug)
	}
	if v.ChannelID == nil || *v.ChannelID != stableCh.ID {
		t.Fatalf("channel id must point to stable channel")
	}

	// 晋升不移动产物行：线集合不变。
	linesAfter, _ := store.ListVersionLines(ctx, mustVersionID(t, svc, p.ID, "1.2.3"))
	if len(linesBefore) != len(linesAfter) {
		t.Fatalf("promotion must not move artifact lines")
	}
}

// TestPromote_GuardsDraftUnpublishedAndDisabledChannel 晋升只允许 Published 版本、
// 目标渠道必须存在且启用。
func TestPromote_GuardsDraftUnpublishedAndDisabledChannel(t *testing.T) {
	svc, store, _, _ := setupReuseService(t)
	ctx := context.Background()

	slug := "promote-guard"
	p, _, err := svc.Create(ctx, CreateProjectInput{DefaultLocale: ptr("en"), Slug: &slug})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	// Draft 直接用 PUT 改，不走 promote。
	if _, _, err := svc.PutVersion(ctx, p.ID, "1.0.0", VersionWriteInput{Channel: "beta"}); err != nil {
		t.Fatalf("put draft: %v", err)
	}
	if _, err := svc.PromoteVersion(ctx, p.ID, "1.0.0", PromoteVersionInput{TargetChannel: "stable"}); !errors.Is(err, ErrInvalidVersionTransition) {
		t.Fatalf("expected ErrInvalidVersionTransition for draft, got %v", err)
	}

	content := []byte("payload")
	if _, err := svc.UploadArtifact(ctx, p.Slug, "1.0.0", "windows", "x86_64", UploadArtifactInput{
		Filename: "app.exe",
		Size:     int64(len(content)),
	}, bytes.NewReader(content)); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if _, err := svc.PublishVersion(ctx, p.ID, "1.0.0"); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// 目标渠道不存在。
	if _, err := svc.PromoteVersion(ctx, p.ID, "1.0.0", PromoteVersionInput{TargetChannel: "nightly"}); !errors.Is(err, ErrChannelNotFound) {
		t.Fatalf("expected ErrChannelNotFound, got %v", err)
	}

	// 目标渠道已停用。
	if err := store.CreateChannel(ctx, &model.Channel{ProjectID: p.ID, Slug: "insider", StabilityRank: 15, Enabled: true, System: false}); err != nil {
		t.Fatalf("create channel: %v", err)
	}
	ch, _ := store.GetChannel(ctx, p.ID, "insider")
	ch.Enabled = false
	if err := store.SaveChannel(ctx, ch); err != nil {
		t.Fatalf("disable channel: %v", err)
	}
	if _, err := svc.PromoteVersion(ctx, p.ID, "1.0.0", PromoteVersionInput{TargetChannel: "insider"}); !errors.Is(err, ErrChannelNotFound) {
		t.Fatalf("expected ErrChannelNotFound for disabled channel, got %v", err)
	}

	// 幂等：晋升到当前渠道直接成功。
	if _, err := svc.PromoteVersion(ctx, p.ID, "1.0.0", PromoteVersionInput{TargetChannel: "beta"}); err != nil {
		t.Fatalf("idempotent promote failed: %v", err)
	}
}

// mustVersionID 供测试内按版本引用取 Version ID。
func mustVersionID(t *testing.T, svc *ProjectService, projectID uuid.UUID, ref string) uuid.UUID {
	t.Helper()
	v, err := svc.ResolveVersion(context.Background(), projectID, ref)
	if err != nil {
		t.Fatalf("resolve version %s: %v", ref, err)
	}
	return v.ID
}
