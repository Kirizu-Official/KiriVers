package update

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// ---------- diff 夹具 ----------

// diffFixture 构造多文件干净升级目录：1.0.0 → 2.0.0（published、rollout 100、
// windows/x86_64 ready、全量包 1000 字节）。Manifest 差异：target 比 source 多
// 两条（新增），source 多一条（删除）。
func diffFixture() (*Catalog, *fakeLineDetails, uuid.UUID, uuid.UUID) {
	src := mkVersion("stable", model.VersionStatusPublished, intP(10), semP("1.0.0"))
	tgt := mkVersion("stable", model.VersionStatusPublished, intP(20), semP("2.0.0"))

	srcLineID, tgtLineID := uuid.New(), uuid.New()
	src.Lines = append(src.Lines, mkIDLine(srcLineID, "windows", "x86_64",
		model.VersionLineStatusReady, "roothash-1.0.0",
		mkPkg("demo-1.0.0-windows-x86_64.zip", 1000, nil)))
	tgt.Lines = append(tgt.Lines, mkIDLine(tgtLineID, "windows", "x86_64",
		model.VersionLineStatusReady, "roothash-2.0.0",
		mkPkg("demo-2.0.0-windows-x86_64.zip", 1000, nil)))

	details := &fakeLineDetails{byLine: map[uuid.UUID]*LineDetail{
		srcLineID: {Manifest: []model.ManifestEntry{
			mkManifestEntry("bin/app.exe", 100, model.InstallPolicyOverwrite),
			mkManifestEntry("old/removed.txt", 10, model.InstallPolicyOverwrite),
		}},
		tgtLineID: {Manifest: []model.ManifestEntry{
			mkManifestEntry("bin/app.exe", 120, model.InstallPolicyOverwrite),
			mkManifestEntry("new/added1.txt", 10, model.InstallPolicyOverwrite),
			mkManifestEntry("new/added2.txt", 10, model.InstallPolicyOverwrite),
		}},
	}}

	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, src, tgt)
	cat.Matrix = &MatrixInfo{OS: "windows", Arch: "x86_64", PackageType: model.PackageTypeMultiFile}
	return cat, details, srcLineID, tgtLineID
}

// diffInput 构造 1.0.0 → 2.0.0 的默认 diff 请求。
func diffInput() DiffInput {
	return DiffInput{SourceVersion: "1.0.0", TargetVersion: "2.0.0", OS: "windows", Arch: "x86_64"}
}

// diffCtx 是测试用 context。
var diffCtx = context.Background()

// ---------- 干净升级与全量一致性 ----------

// 验收项：check 与 diff 对同一全量包三元组完全一致（同一 matchHwVariant 结果行）。
func TestDiffFullPackageConsistentWithCheck(t *testing.T) {
	cat, details, _, _ := diffFixture()

	chk, err := Check(cat, CheckInput{OS: "windows", Arch: "x86_64", CurrentVersion: "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	df, err := Diff(diffCtx, cat, details, diffInput())
	if err != nil {
		t.Fatal(err)
	}
	b := df.Body
	if b.DiffMode != DiffModeFullPackage {
		t.Fatalf("diff_mode = %s", b.DiffMode)
	}
	if b.PackageURL != chk.Body.PackageURL || b.Size != chk.Body.Size || b.SHA256 != chk.Body.SHA256 {
		t.Fatalf("full package triple must match check: diff=%s/%d/%s check=%s/%d/%s",
			b.PackageURL, b.Size, b.SHA256, chk.Body.PackageURL, chk.Body.Size, chk.Body.SHA256)
	}
	if b.RootHash != "roothash-2.0.0" || b.CompareEngine != model.CompareEngineSemver {
		t.Fatalf("root_hash/compare_engine wrong: %+v", b)
	}
	if b.VersionInteger == nil || *b.VersionInteger != 20 || b.VersionSemver == nil || *b.VersionSemver != "2.0.0" {
		t.Fatalf("dual numbers wrong: %+v", b)
	}
	// 多文件默认响应：deleted_paths 仅 Manifest diff（一条删除），
	// 无逐文件 URL 数组（验收项），无 invalid_paths。
	if !equalStrings(b.DeletedPaths, []string{"old/removed.txt"}) {
		t.Fatalf("deleted_paths = %v", b.DeletedPaths)
	}
	if b.Files != nil {
		t.Fatalf("default multi-file diff must never include per-file urls: %+v", b.Files)
	}
	if b.InvalidPaths != nil {
		t.Fatalf("clean upgrade must not report invalid_paths: %+v", b.InvalidPaths)
	}
}

// source 状态宽松（C09-7）：Draft / 无平台线 / Revoked / yank 一律全量回退，不报错。
func TestDiffSourceAnomaliesFallBackToFull(t *testing.T) {
	cat, details, srcLineID, _ := diffFixture()

	// source Draft。
	cat.Versions[0].Version.Status = model.VersionStatusDraft
	res, err := Diff(diffCtx, cat, details, diffInput())
	if err != nil || res.Body.DiffMode != DiffModeFullPackage {
		t.Fatalf("draft source must fall back to full, got %v / %s", err, modeOf(res))
	}
	cat.Versions[0].Version.Status = model.VersionStatusPublished

	// source 本平台线 yanked → 视作离开路径：仍全量且跳过灰度闸。
	cat.Versions[0].Lines[0].Status = model.VersionLineStatusYanked
	markGrayIncomplete(&cat.Versions[1].Version)
	res, err = Diff(diffCtx, cat, details, diffInput())
	if err != nil || res.Body.DiffMode != DiffModeFullPackage {
		t.Fatalf("yanked source must fall back to full, got %v / %s", err, modeOf(res))
	}
	cat.Versions[0].Lines[0].Status = model.VersionLineStatusReady
	done := testGrayCompleteAt
	cat.Versions[1].Version.GrayCompletedAt = &done

	// source revoked → 全量回退（同时是灰度离开路径）。
	cat.Versions[0].Version.Status = model.VersionStatusRevoked
	res, err = Diff(diffCtx, cat, details, diffInput())
	if err != nil || res.Body.DiffMode != DiffModeFullPackage {
		t.Fatalf("revoked source must fall back to full, got %v / %s", err, modeOf(res))
	}
	cat.Versions[0].Version.Status = model.VersionStatusPublished

	// source 无平台线。
	cat.Versions[0].Lines = nil
	res, err = Diff(diffCtx, cat, details, diffInput())
	if err != nil || res.Body.DiffMode != DiffModeFullPackage {
		t.Fatalf("missing source line must fall back to full, got %v / %s", err, modeOf(res))
	}

	_ = srcLineID
}

// prefer_full=true → 直接全量（§7.1 回退条件）。
func TestDiffPreferFull(t *testing.T) {
	cat, details, _, tgtLineID := diffFixture()
	in := diffInput()
	in.PreferFull = true
	in.Capabilities = []string{"patch_package", "binary_delta"}
	in.AcceptedDeltaAlgos = []string{"bsdiff"}
	// 即便存在可命中的 patch 对象也直接全量。
	details.byLine[tgtLineID].Patches = []DeltaArtifactInfo{mkPatchDelta("1.0.0", "2.0.0", 600)}
	res, err := Diff(diffCtx, cat, details, in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Body.DiffMode != DiffModeFullPackage {
		t.Fatalf("prefer_full must yield full_package, got %s", res.Body.DiffMode)
	}
}

// ---------- 状态闸与协议 ----------

// 目标状态闸：Draft → VERSION_NOT_VISIBLE；Revoked → VERSION_REVOKED；
// 未知 source/target → VERSION_NOT_FOUND；target 无线 → VERSION_LINE_NOT_FOUND。
func TestDiffTargetGates(t *testing.T) {
	cat, details, _, _ := diffFixture()

	cat.Versions[1].Version.Status = model.VersionStatusDraft
	if _, err := Diff(diffCtx, cat, details, diffInput()); !errors.Is(err, ErrVersionNotVisible) {
		t.Fatalf("draft target must be VERSION_NOT_VISIBLE, got %v", err)
	}
	cat.Versions[1].Version.Status = model.VersionStatusRevoked
	if _, err := Diff(diffCtx, cat, details, diffInput()); !errors.Is(err, ErrVersionRevoked) {
		t.Fatalf("revoked target must be VERSION_REVOKED, got %v", err)
	}
	cat.Versions[1].Version.Status = model.VersionStatusPublished

	stamped := cat.Versions[1].Lines[0].PacksReadyAt
	cat.Versions[1].Lines[0].PacksReadyAt = nil
	if _, err := Diff(diffCtx, cat, details, diffInput()); !errors.Is(err, ErrVersionNotVisible) {
		t.Fatalf("unstamped target must be VERSION_NOT_VISIBLE, got %v", err)
	}
	cat.Versions[1].Lines[0].PacksReadyAt = stamped

	in := diffInput()
	in.TargetVersion = "9.9.9"
	if _, err := Diff(diffCtx, cat, details, in); !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("unknown target must be VERSION_NOT_FOUND, got %v", err)
	}
	in = diffInput()
	in.SourceVersion = "not-a-version"
	if _, err := Diff(diffCtx, cat, details, in); !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("unparsable source must be VERSION_NOT_FOUND, got %v", err)
	}
	in = diffInput()
	cat.Versions[1].Lines = nil // 目标（2.0.0）无线
	if _, err := Diff(diffCtx, cat, details, in); !errors.Is(err, ErrVersionLineNotFound) {
		t.Fatalf("missing target line must be VERSION_LINE_NOT_FOUND, got %v", err)
	}
}

// 渠道冲突。
func TestDiffChannel(t *testing.T) {
	cat, details, _, _ := diffFixture()

	in := diffInput()
	in.Channel = "beta"
	if _, err := Diff(diffCtx, cat, details, in); !errors.Is(err, ErrChannelConflict) {
		t.Fatalf("expected CHANNEL_CONFLICT, got %v", err)
	}
	in.Channel = "stable"
	if _, err := Diff(diffCtx, cat, details, in); err != nil {
		t.Fatalf("matching channel must pass: %v", err)
	}
}

// ---------- 灰度闸（C09-11）----------

// 验收项：非强制且灰度未命中 → 412 PRECONDITION_FAILED（匿名与 device 未命中
// 两个分支）；强制路径与离开路径绕过。
func TestDiffGrayGatePrecondition(t *testing.T) {
	cat, details, _, _ := diffFixture()
	markGrayIncomplete(&cat.Versions[1].Version)

	if _, err := Diff(diffCtx, cat, details, diffInput()); !errors.Is(err, ErrPreconditionFailed) {
		t.Fatalf("anonymous incomplete gray must be 412, got %v", err)
	}

	in := diffInput()
	in.DeviceID = "dev-miss"
	in.DeviceHash = "hash-miss"
	if _, err := Diff(diffCtx, cat, details, in); !errors.Is(err, ErrPreconditionFailed) {
		t.Fatalf("gray miss must be 412, got %v", err)
	}

	in.DeviceHash = "hash-hit"
	cat.Versions[1].Allowlist = Allowlist{Version: []string{"hash-hit"}}
	res, err := Diff(diffCtx, cat, details, in)
	if err != nil {
		t.Fatalf("gray hit must pass: %v", err)
	}
	if res.Body.DiffMode != DiffModeFullPackage {
		t.Fatalf("gray hit must serve: %s", res.Body.DiffMode)
	}

	done := testGrayCompleteAt
	cat.Versions[1].Version.GrayCompletedAt = &done
	if _, err := Diff(diffCtx, cat, details, diffInput()); err != nil {
		t.Fatalf("complete gray must pass anonymously: %v", err)
	}
}

// 强制路径绕过灰度闸（复用 select 强制判定）：floor 高于当前 → 强制 → 放行。
func TestDiffGrayGateBypassedByMandatoryFloor(t *testing.T) {
	cat, details, _, _ := diffFixture()
	markGrayIncomplete(&cat.Versions[1].Version)
	floor := "1.5.0"
	cat.Project.MinimumSupportedVersion = &floor

	// 匿名 + rollout 10% + 强制 floor → 放行（强制忽略灰度）。
	res, err := Diff(diffCtx, cat, details, diffInput())
	if err != nil {
		t.Fatalf("mandatory floor must bypass gray gate: %v", err)
	}
	if res.Body.DiffMode != DiffModeFullPackage {
		t.Fatalf("unexpected mode: %s", res.Body.DiffMode)
	}

	// floor 不强制时同样请求 → 412（对照）。
	cat.Project.MinimumSupportedVersion = nil
	if _, err := Diff(diffCtx, cat, details, diffInput()); !errors.Is(err, ErrPreconditionFailed) {
		t.Fatalf("without mandatory floor must be 412, got %v", err)
	}
}

// ---------- patch_package 消费（夹具）----------

// mkPatchDelta 构造可命中 1.0.0 → 2.0.0 的 patch_package 对象。
// 夹具全量包命名规则为 mkPkg：SHA-256 = "sha-" + file_name，因此
// 1.0.0 / 2.0.0 线全量归档哈希即下方推导（与 auto_delta Job 写入的
// DeltaSourceSHA256/DeltaTargetSHA256 同构，C13-4 SHA 身份匹配）。
func mkPatchDelta(src, tgt string, size int64) DeltaArtifactInfo {
	return DeltaArtifactInfo{
		Kind:             DeltaKindPatchPackage,
		FileName:         "patch-" + src + "-" + tgt + ".zip",
		Size:             size,
		SHA256:           "sha-patch",
		SourceSHA256:     "sha-demo-" + src + "-windows-x86_64.zip",
		TargetSHA256:     "sha-demo-" + tgt + "-windows-x86_64.zip",
		SourceVersionRef: src,
		TargetVersionRef: tgt,
	}
}

// patch_package：能力声明 + 预生成对象 + 体积 <70% 全量 → diff_mode=patch_package。
func TestDiffPatchPackageConsumed(t *testing.T) {
	cat, details, _, tgtLineID := diffFixture()
	details.byLine[tgtLineID].Patches = []DeltaArtifactInfo{mkPatchDelta("1.0.0", "2.0.0", 600)}

	in := diffInput()
	in.Capabilities = []string{"patch_package"}
	res, err := Diff(diffCtx, cat, details, in)
	if err != nil {
		t.Fatal(err)
	}
	b := res.Body
	if b.DiffMode != DiffModePatchPackage {
		t.Fatalf("expected patch_package, got %s", b.DiffMode)
	}
	if b.PackageURL != "/api/v1/projects//packages/sha-patch" || b.Size != 600 || b.SHA256 != "sha-patch" {
		t.Fatalf("patch triple wrong: %+v", b)
	}

	// 未声明 patch_package 能力 → 全量（§7.5.4）。
	in2 := diffInput()
	res2, _ := Diff(diffCtx, cat, details, in2)
	if res2.Body.DiffMode != DiffModeFullPackage {
		t.Fatalf("without capability must be full_package, got %s", res2.Body.DiffMode)
	}
}

// 体积闸门已移到入队/预热（Manifest 未压缩合计）；消费侧只要 Size>0 即采用 patch。
func TestDiffPatchPackageTooLargeFallsBack(t *testing.T) {
	cat, details, _, tgtLineID := diffFixture()
	details.byLine[tgtLineID].Patches = []DeltaArtifactInfo{mkPatchDelta("1.0.0", "2.0.0", 800)} // 80% of zip Size, ignored

	in := diffInput()
	in.Capabilities = []string{"patch_package"}
	res, err := Diff(diffCtx, cat, details, in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Body.DiffMode != DiffModePatchPackage {
		t.Fatalf("existing patch with size>0 must be consumed, got %s", res.Body.DiffMode)
	}
}

// C09-8：降级目标禁止 patch/binary_delta（§7.5.3 降级只用全量包）。
func TestDiffDowngradeForbidsDelta(t *testing.T) {
	cat, details, _, tgtLineID := diffFixture()
	// 在 1.0.0 线上放置 2.0.0 → 1.0.0 的 patch 对象（降级方向）。
	details.byLine[tgtLineID].Patches = []DeltaArtifactInfo{mkPatchDelta("2.0.0", "1.0.0", 600)}

	in := DiffInput{SourceVersion: "2.0.0", TargetVersion: "1.0.0", OS: "windows", Arch: "x86_64"}
	in.Capabilities = []string{"patch_package", "binary_delta"}
	in.AcceptedDeltaAlgos = []string{"bsdiff"}
	res, err := Diff(diffCtx, cat, details, in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Body.DiffMode != DiffModeFullPackage {
		t.Fatalf("downgrade must be full_package, got %s", res.Body.DiffMode)
	}
}

// ---------- binary_delta 消费（真实元数据夹具，单文件）----------

// mkBinaryDelta 构造 1.0.0 → 2.0.0 的单文件二进制差量对象。
// 哈希与夹具全量包的 SHA-256（mkPkg 规则 sha-{file_name}）对应，
// 与真实生成 Job 写入的 DeltaSourceSHA256/DeltaTargetSHA256 同构（C10-6）。
func mkBinaryDelta(algo string) DeltaArtifactInfo {
	return DeltaArtifactInfo{
		Kind:             DeltaKindBinaryDelta,
		FileName:         "demo-2.0.0-from-1.0.0-windows-x86_64-" + algo + "-sha-demo-1.0.0-windows-x86_64.zip-sha-demo-2.0.0-windows-x86_64.zip." + algo,
		Size:             300,
		SHA256:           "sha-delta",
		Algo:             algo,
		SourceSHA256:     "sha-demo-1.0.0-windows-x86_64.zip",
		TargetSHA256:     "sha-demo-2.0.0-windows-x86_64.zip",
		SourceVersionRef: "1.0.0",
		TargetVersionRef: "2.0.0",
	}
}

// binaryDeltaFixture 单文件矩阵 + 差量对象。
func binaryDeltaFixture() (*Catalog, *fakeLineDetails, uuid.UUID) {
	cat, details, _, tgtLineID := diffFixture()
	cat.Matrix = &MatrixInfo{OS: "windows", Arch: "x86_64", PackageType: model.PackageTypeSingleFile, DeltaAlgo: "bsdiff"}
	details.byLine[tgtLineID].Deltas = []DeltaArtifactInfo{mkBinaryDelta("bsdiff")}
	return cat, details, tgtLineID
}

// C09-8 / §7.2：单文件 + binary_delta 能力 + 算法交集 + 基线哈希吻合 →
// binary_delta；基线不纯净 / 未声明算法 / 多文件 → 全量。
func TestDiffBinaryDeltaConsumed(t *testing.T) {
	cat, details, _ := binaryDeltaFixture()

	in := diffInput()
	in.LocalSHA256 = "sha-demo-1.0.0-windows-x86_64.zip" // source 官方哈希
	in.AcceptedDeltaAlgos = []string{"bsdiff"}
	res, err := Diff(diffCtx, cat, details, in)
	if err != nil {
		t.Fatal(err)
	}
	b := res.Body
	if b.DiffMode != DiffModeBinaryDelta || b.DeltaAlgo != "bsdiff" {
		t.Fatalf("expected binary_delta/bsdiff, got %s/%s", b.DiffMode, b.DeltaAlgo)
	}
	if b.PackageURL != "/api/v1/projects//packages/sha-delta" || b.Size != 300 || b.SHA256 != "sha-delta" {
		t.Fatalf("delta triple wrong: %+v", b)
	}

	// 基线哈希不吻合（非纯净）→ 全量。
	in.LocalSHA256 = "deadbeef"
	res, _ = Diff(diffCtx, cat, details, in)
	if res.Body.DiffMode != DiffModeFullPackage {
		t.Fatalf("dirty baseline must fall back to full, got %s", res.Body.DiffMode)
	}

	// 未声明 accepted_delta_algos（无交集）→ 全量（C09-8）。
	in.LocalSHA256 = "sha-demo-1.0.0-windows-x86_64.zip"
	in.AcceptedDeltaAlgos = nil
	res, _ = Diff(diffCtx, cat, details, in)
	if res.Body.DiffMode != DiffModeFullPackage {
		t.Fatalf("without accepted algos must be full, got %s", res.Body.DiffMode)
	}

	// 算法不在接受列表 → 全量。
	in.AcceptedDeltaAlgos = []string{"xdelta3"}
	res, _ = Diff(diffCtx, cat, details, in)
	if res.Body.DiffMode != DiffModeFullPackage {
		t.Fatalf("non-intersecting algo must be full, got %s", res.Body.DiffMode)
	}
}

// 多文件线禁止 binary_delta（§7：多文件不做每文件二进制差量）。
func TestDiffBinaryDeltaMultiFileForbidden(t *testing.T) {
	cat, details, _, tgtLineID := diffFixture()
	// 保持 multi_file 矩阵。
	details.byLine[tgtLineID].Deltas = []DeltaArtifactInfo{mkBinaryDelta("bsdiff")}

	in := diffInput()
	in.LocalSHA256 = "sha-demo-1.0.0-windows-x86_64.zip"
	in.AcceptedDeltaAlgos = []string{"bsdiff"}
	res, err := Diff(diffCtx, cat, details, in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Body.DiffMode != DiffModeFullPackage {
		t.Fatalf("multi-file binary_delta must be forbidden, got %s", res.Body.DiffMode)
	}
}

// C10-5：accepted ∩ 可用交集，优先矩阵默认算法；无默认时按算法名字典序确定性回退。
func TestDiffBinaryDeltaAlgoPreference(t *testing.T) {
	oldData := func(cat *Catalog, details *fakeLineDetails, tgtLineID uuid.UUID, algos ...string) DiffInput {
		var ds []DeltaArtifactInfo
		for _, a := range algos {
			ds = append(ds, mkBinaryDelta(a))
		}
		details.byLine[tgtLineID].Deltas = ds
		in := diffInput()
		in.LocalSHA256 = "sha-demo-1.0.0-windows-x86_64.zip"
		in.AcceptedDeltaAlgos = algos
		return in
	}

	// 交集含矩阵默认 bsdiff → 必选 bsdiff。
	cat, details, tgtLineID := binaryDeltaFixture()
	in := oldData(cat, details, tgtLineID, "xdelta3", "bsdiff", "hdiffpatch")
	res, err := Diff(diffCtx, cat, details, in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Body.DiffMode != DiffModeBinaryDelta || res.Body.DeltaAlgo != "bsdiff" {
		t.Fatalf("matrix default algo must win, got %s/%s", res.Body.DiffMode, res.Body.DeltaAlgo)
	}

	// 无矩阵默认：字典序回退 → bsdiff < hdiffpatch < xdelta3。
	cat.Matrix = &MatrixInfo{OS: "windows", Arch: "x86_64", PackageType: model.PackageTypeSingleFile}
	in = oldData(cat, details, tgtLineID, "xdelta3", "bsdiff", "hdiffpatch")
	res, err = Diff(diffCtx, cat, details, in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Body.DeltaAlgo != "bsdiff" {
		t.Fatalf("lexicographic fallback must pick bsdiff, got %s", res.Body.DeltaAlgo)
	}

	// 客户端只接受部分算法 → 交集内字典序最前。
	in.AcceptedDeltaAlgos = []string{"xdelta3", "hdiffpatch"}
	res, err = Diff(diffCtx, cat, details, in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Body.DeltaAlgo != "hdiffpatch" {
		t.Fatalf("intersection fallback must pick hdiffpatch, got %s", res.Body.DeltaAlgo)
	}
}

// 验收项（C10-3/C10-6）：差量对象的源/目标官方哈希任一不吻合即不命中 ——
// 源或目标字节变化后旧差量 URL 不会被复用。
func TestDiffBinaryDeltaStaleHashNotServed(t *testing.T) {
	cat, details, tgtLineID := binaryDeltaFixture()

	d := mkBinaryDelta("bsdiff")
	d.SourceSHA256 = "sha-old-bytes" // 源字节已变化（同版本号不同字节）
	details.byLine[tgtLineID].Deltas = []DeltaArtifactInfo{d}

	in := diffInput()
	in.LocalSHA256 = "sha-demo-1.0.0-windows-x86_64.zip"
	in.AcceptedDeltaAlgos = []string{"bsdiff"}
	res, err := Diff(diffCtx, cat, details, in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Body.DiffMode != DiffModeFullPackage {
		t.Fatalf("stale source hash must not serve delta, got %s", res.Body.DiffMode)
	}

	// 目标哈希不吻合同样拒绝。
	d2 := mkBinaryDelta("bsdiff")
	d2.TargetSHA256 = "sha-other-target"
	details.byLine[tgtLineID].Deltas = []DeltaArtifactInfo{d2}
	res, err = Diff(diffCtx, cat, details, in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Body.DiffMode != DiffModeFullPackage {
		t.Fatalf("stale target hash must not serve delta, got %s", res.Body.DiffMode)
	}
}

// ---------- file_list 例外（C09-6 / §7.5.4）----------

// 验收项：未声明 file_list 的多文件 diff 响应无逐文件 URL 数组
// （已在 TestDiffFullPackageConsistentWithCheck 覆盖）；显式声明且新增 ≤16 →
// file_list 模式；超阈值 → 回退归档。
func TestDiffFileListException(t *testing.T) {
	cat, details, srcLineID, tgtLineID := diffFixture()

	// 为两条新增路径挂 kind=file 产物。
	details.byLine[tgtLineID].Files = []FileArtifactInfo{
		{Path: "new/added1.txt", FileName: "new-added1", Size: 10, SHA256: testSHA256},
		{Path: "new/added2.txt", FileName: "new-added2", Size: 10, SHA256: testSHA256},
	}

	in := diffInput()
	in.Capabilities = []string{"file_list"}
	res, err := Diff(diffCtx, cat, details, in)
	if err != nil {
		t.Fatal(err)
	}
	b := res.Body
	if b.DiffMode != DiffModeFileList {
		t.Fatalf("expected file_list, got %s", b.DiffMode)
	}
	if len(b.Files) != 2 || b.Files[0].URL != "/api/v1/projects//packages/"+testSHA256 {
		t.Fatalf("file_list entries wrong: %+v", b.Files)
	}
	if b.PackageURL != "" {
		t.Fatalf("file_list must not carry an archive url: %s", b.PackageURL)
	}
	if !equalStrings(b.DeletedPaths, []string{"old/removed.txt"}) {
		t.Fatalf("deleted_paths = %v", b.DeletedPaths)
	}

	// 未声明 file_list 能力 → 恒无逐文件数组（验收项）。
	res, _ = Diff(diffCtx, cat, details, diffInput())
	if res.Body.DiffMode != DiffModeFullPackage || res.Body.Files != nil {
		t.Fatalf("undeclared file_list must never produce files[]: %s / %+v", res.Body.DiffMode, res.Body.Files)
	}

	// 新增 >16 → 回退 full_package。
	entries := []model.ManifestEntry{mkManifestEntry("bin/app.exe", 120, model.InstallPolicyOverwrite)}
	var fileArts []FileArtifactInfo
	for i := 0; i < model.DefaultFileListMaxFiles+1; i++ {
		p := fmt.Sprintf("bulk/%02d.txt", i)
		entries = append(entries, mkManifestEntry(p, 1, model.InstallPolicyOverwrite))
		fileArts = append(fileArts, FileArtifactInfo{Path: p, FileName: "f-" + p})
	}
	// source 仅 bin/app.exe；target 额外新增 17 条（>16）。
	details.byLine[srcLineID].Manifest = []model.ManifestEntry{mkManifestEntry("bin/app.exe", 120, model.InstallPolicyOverwrite)}
	details.byLine[tgtLineID].Manifest = entries
	details.byLine[tgtLineID].Files = fileArts
	in = diffInput()
	in.Capabilities = []string{"file_list"}
	res, err = Diff(diffCtx, cat, details, in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Body.DiffMode != DiffModeFullPackage {
		t.Fatalf(">16 pending files must fall back to archive, got %s", res.Body.DiffMode)
	}
}

func TestDiffFileListProjectCapAndDirtyClamp(t *testing.T) {
	cat, details, srcLineID, tgtLineID := diffFixture()
	cat.Project.FileListMaxFiles = 4

	entries := []model.ManifestEntry{mkManifestEntry("bin/app.exe", 120, model.InstallPolicyOverwrite)}
	var fileArts []FileArtifactInfo
	for i := 0; i < 5; i++ {
		p := fmt.Sprintf("bulk/%02d.txt", i)
		entries = append(entries, mkManifestEntry(p, 1, model.InstallPolicyOverwrite))
		fileArts = append(fileArts, FileArtifactInfo{Path: p, FileName: "f-" + p, SHA256: testSHA256})
	}
	details.byLine[srcLineID].Manifest = []model.ManifestEntry{mkManifestEntry("bin/app.exe", 120, model.InstallPolicyOverwrite)}
	details.byLine[tgtLineID].Manifest = entries
	details.byLine[tgtLineID].Files = fileArts
	in := diffInput()
	in.Capabilities = []string{"file_list"}
	res, err := Diff(diffCtx, cat, details, in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Body.DiffMode != DiffModeFullPackage || res.Body.Files != nil {
		t.Fatalf("project cap 4 with 5 files must not be file_list: %s %+v", res.Body.DiffMode, res.Body.Files)
	}

	dirty := &Catalog{Project: ProjectInfo{FileListMaxFiles: 32}}
	applyFileListCeiling(dirty, 16)
	if dirty.Project.FileListMaxFiles != 16 {
		t.Fatalf("dirty 32 must clamp to 16, got %d", dirty.Project.FileListMaxFiles)
	}
}

// 缺少 kind=file 产物映射 → file_list 不命中，回退归档。
func TestDiffFileListRequiresFileArtifacts(t *testing.T) {
	cat, details, _, _ := diffFixture()
	in := diffInput()
	in.Capabilities = []string{"file_list"}
	res, err := Diff(diffCtx, cat, details, in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Body.DiffMode != DiffModeFullPackage {
		t.Fatalf("without file artifacts must fall back, got %s", res.Body.DiffMode)
	}
}

// ---------- 辅助 ----------

func modeOf(res *DiffResult) string {
	if res == nil || res.Body == nil {
		return "<nil>"
	}
	return res.Body.DiffMode
}
