package update

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

// stubDowngrade 是 DowngradeSource 的固定返回桩。
type stubDowngrade struct {
	active bool
	err    error
	calls  int
}

func (s *stubDowngrade) DowngradeActive(ctx context.Context, projectID uuid.UUID, os, arch, deviceHash string) (bool, error) {
	s.calls++
	return s.active, s.err
}

// memoryLoader 是返回固定快照的 CatalogLoader 桩。
type memoryLoader struct{ cat *Catalog }

func (m *memoryLoader) LoadCatalog(ctx context.Context, projectID uuid.UUID, os, arch string) (*Catalog, error) {
	return m.cat, nil
}

// 纯函数：DeviceDowngradeActive → 有差量也强制 full_package（C11-8 / §15.2）。
func TestDiffDeviceDowngradeForcesFullPackage(t *testing.T) {
	cat, details, _, tgtLineID := diffFixture()
	details.byLine[tgtLineID].Patches = []DeltaArtifactInfo{mkPatchDelta("1.0.0", "2.0.0", 600)}

	in := diffInput()
	in.Capabilities = []string{"patch_package", "binary_delta"}
	in.DeviceDowngradeActive = true
	res, err := Diff(diffCtx, cat, details, in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Body.DiffMode != DiffModeFullPackage {
		t.Fatalf("downgrade must force full_package, got %s", res.Body.DiffMode)
	}
	if !strings.HasSuffix(res.Body.PackageURL, "demo-2.0.0-windows-x86_64.zip") || res.Body.Size != 1000 {
		t.Fatalf("full package triple wrong: %s %d", res.Body.PackageURL, res.Body.Size)
	}

	// 恢复（未降级）→ 恢复差量。
	in.DeviceDowngradeActive = false
	res, err = Diff(diffCtx, cat, details, in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Body.DiffMode != DiffModePatchPackage {
		t.Fatalf("recovered device must get delta again, got %s", res.Body.DiffMode)
	}
}

// 编排层：Service.Check 查询降级并置位；降级生效 → delta_available=false，
// ETag / Cache-Control 不受影响（与灰度同样排除在快照外）；查询失败 → 按无
// 降级处理（遥测绝不阻塞更新协议）。
func TestServiceCheckDowngradeIntegration(t *testing.T) {
	cat, _ := stdCatalog()
	cat.Matrix = &MatrixInfo{OS: "windows", Arch: "x86_64", PackageType: model.PackageTypeSingleFile, DeltaAlgo: "hdiffpatch"}
	in := CheckInput{OS: "windows", Arch: "x86_64", CurrentVersion: "1.0.0", AcceptedDeltaAlgos: []string{"bsdiff"}, DeviceHash: "abc"}

	// 无降级基线。
	stub := &stubDowngrade{active: false}
	svc := NewService(&memoryLoader{cat: cat}, WithDowngradeSource(stub))
	res, err := svc.Check(diffCtx, uuid.New(), "windows", "x86_64", in)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Body.DeltaAvailable {
		t.Fatalf("baseline delta_available must be true: %+v", res.Body)
	}
	etagOK, ccOK := res.ETag, res.CacheControl
	if stub.calls != 1 {
		t.Fatalf("downgrade source must be consulted once, got %d", stub.calls)
	}

	// 降级生效：delta_available 翻转，ETag / Cache-Control 不变。
	stub.active = true
	res, err = svc.Check(diffCtx, uuid.New(), "windows", "x86_64", in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Body.DeltaAvailable {
		t.Fatalf("downgrade must disable delta_available: %+v", res.Body)
	}
	if res.ETag != etagOK || res.CacheControl != ccOK {
		t.Fatalf("downgrade must not affect ETag/Cache-Control: %q/%q vs %q/%q",
			res.ETag, res.CacheControl, etagOK, ccOK)
	}

	// 查询失败 → 按无降级处理。
	stub.active, stub.err = true, errors.New("boom")
	res, err = svc.Check(diffCtx, uuid.New(), "windows", "x86_64", in)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Body.DeltaAvailable {
		t.Fatal("downgrade source error must not block the update protocol")
	}

	// 无 DeviceHash → 不查询。
	stub.err, stub.calls = nil, 0
	inNoHash := in
	inNoHash.DeviceHash = ""
	if _, err := svc.Check(diffCtx, uuid.New(), "windows", "x86_64", inNoHash); err != nil {
		t.Fatal(err)
	}
	if stub.calls != 0 {
		t.Fatalf("empty device hash must skip the downgrade query, got %d calls", stub.calls)
	}
}

// 编排层：Service.Diff 同样置位 DeviceDowngradeActive。
func TestServiceDiffDowngradeIntegration(t *testing.T) {
	cat, details, _, _ := diffFixture()
	stub := &stubDowngrade{active: true}
	svc := NewService(&memoryLoader{cat: cat}, WithLineDetails(details), WithDowngradeSource(stub))
	in := diffInput()
	in.Capabilities = []string{"patch_package"}
	in.DeviceHash = "abc" // 触发编排层降级查询
	details.byLine[mustTgtLineID(t, cat)].Patches = []DeltaArtifactInfo{mkPatchDelta("1.0.0", "2.0.0", 600)}

	res, err := svc.Diff(diffCtx, uuid.New(), "windows", "x86_64", in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Body.DiffMode != DiffModeFullPackage {
		t.Fatalf("active downgrade must force full_package, got %s", res.Body.DiffMode)
	}
	if stub.calls != 1 {
		t.Fatalf("downgrade source must be consulted once, got %d", stub.calls)
	}
}

// mustTgtLineID 从目录里找 windows/x86_64 的目标线 ID（2.0.0）。
func mustTgtLineID(t *testing.T, cat *Catalog) uuid.UUID {
	t.Helper()
	for i := range cat.Versions {
		v := &cat.Versions[i]
		if v.Version.VersionSemverCanonical != nil && *v.Version.VersionSemverCanonical == "2.0.0" {
			for _, l := range v.Lines {
				if l.OS == "windows" && l.Arch == "x86_64" {
					return l.ID
				}
			}
		}
	}
	t.Fatal("target line not found")
	return uuid.Nil
}
