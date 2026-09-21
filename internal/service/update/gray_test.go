package update

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

func grayVersion(integer int64, semver string, complete bool) VersionState {
	return mkVersion("stable", model.VersionStatusPublished, intP(integer), semP(semver),
		func(v *model.Version) {
			if complete {
				return
			}
			markGrayIncomplete(v)
		})
}

func addLine(vs *VersionState, os, arch, pkg string) *LineState {
	mkLine(vs, os, arch, model.VersionLineStatusReady, mkPkg(pkg, 100, nil))
	line := vs.Line(os, arch)
	line.ID = uuid.New()
	return line
}

func grayReq(cur *VersionState, rawDevice, deviceHash string) Request {
	req := defaultReq(cur)
	req.DeviceID = rawDevice
	req.DeviceHash = deviceHash
	return req
}

func withVersionAllowlist(vs *VersionState, ids ...string) *VersionState {
	vs.Allowlist = Allowlist{Version: ids}
	return vs
}

func TestSelectTarget_VersionAllowlistHitWhenIncomplete(t *testing.T) {
	cur := grayVersion(10, "1.0.0", true)
	tgt := grayVersion(20, "2.0.0", false)
	addLine(&tgt, "windows", "x86_64", "pkg-x64.zip")
	withVersionAllowlist(&tgt, "hash-allow", "hash-other")
	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, tgt)

	res, err := SelectTarget(cat, grayReq(&cur, "device-allow", "hash-allow"))
	if err != nil || res.Target == nil {
		t.Fatalf("allowlisted device must hit: res=%+v err=%v", res, err)
	}
	if res.Target.Version.ID != tgt.Version.ID {
		t.Fatalf("unexpected target %+v", res.Target.Version)
	}

	res, err = SelectTarget(cat, grayReq(&cur, "device-deny", "hash-deny"))
	if err != nil || res.Target != nil {
		t.Fatalf("non-allowlisted device must miss: res=%+v err=%v", res, err)
	}

	res, err = SelectTarget(cat, grayReq(&cur, "", ""))
	if err != nil || res.Target != nil {
		t.Fatalf("anonymous must miss incomplete gray: res=%+v err=%v", res, err)
	}

	res, err = SelectTarget(cat, grayReq(&cur, "device-allow", ""))
	if err != nil || res.Target != nil {
		t.Fatalf("empty allowlist key must not hit: res=%+v err=%v", res, err)
	}
}

func TestCheck_AllowlistDeviceGets200OthersGet204(t *testing.T) {
	cur := grayVersion(10, "1.0.0", true)
	addLine(&cur, "windows", "x86_64", "demo-1.0.0.zip")
	tgt := grayVersion(20, "2.0.0", false)
	addLine(&tgt, "windows", "x86_64", "demo-2.0.0.zip")
	withVersionAllowlist(&tgt, "hash-allow")
	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, tgt)

	res, err := Check(cat, CheckInput{OS: "windows", Arch: "x86_64", CurrentVersion: "1.0.0",
		DeviceID: "device-allow", DeviceHash: "hash-allow"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != 200 || res.Body == nil || !res.Body.HasUpdate {
		t.Fatalf("allowlisted device must get 200, got %+v", res)
	}

	res, err = Check(cat, CheckInput{OS: "windows", Arch: "x86_64", CurrentVersion: "1.0.0",
		DeviceID: "device-deny", DeviceHash: "hash-deny"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != 204 || res.Body != nil {
		t.Fatalf("non-allowlisted device must get 204, got %+v", res)
	}

	if res.CacheControl != cacheControlPrivateDevice {
		t.Fatalf("incomplete gray check must be private, got %s", res.CacheControl)
	}
}

func TestSelectTarget_MandatoryIgnoresGrayAndAllowlist(t *testing.T) {
	cur := grayVersion(10, "1.0.0", true)
	tgt := grayVersion(20, "2.0.0", false)
	addLine(&tgt, "windows", "x86_64", "pkg-x64.zip")
	withVersionAllowlist(&tgt, "hash-someone-else")
	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, tgt)
	cat.Project.MinimumSupportedVersion = semP("2.0.0")

	res, err := SelectTarget(cat, grayReq(&cur, "dev-not-allowlisted", "hash-not-allowlisted"))
	if err != nil || res.Target == nil {
		t.Fatalf("mandatory path must ignore gray and allowlist: res=%+v err=%v", res, err)
	}
	if res.Reason != ReasonMinimumSupportedVersion || !res.IsMandatory {
		t.Fatalf("expected mandatory minimum_supported_version, got reason=%s mandatory=%v", res.Reason, res.IsMandatory)
	}
}

func TestSelectTarget_AnonymousMissUntilComplete(t *testing.T) {
	cur := grayVersion(10, "1.0.0", true)
	tgt := grayVersion(20, "2.0.0", false)
	addLine(&tgt, "windows", "x86_64", "pkg-x64.zip")
	withVersionAllowlist(&tgt, "hash-a", "hash-b")
	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, tgt)

	req := defaultReq(&cur)
	if res, err := SelectTarget(cat, req); err != nil || res.Target != nil {
		t.Fatalf("anonymous while incomplete must miss: res=%+v err=%v", res, err)
	}

	done := time.Unix(2, 0).UTC()
	cat.Versions[1].Version.GrayCompletedAt = &done
	if res, err := SelectTarget(cat, defaultReq(&cur)); err != nil || res.Target == nil {
		t.Fatalf("anonymous after complete must hit: res=%+v err=%v", res, err)
	}
}

func TestCatalogETag_AllowlistAndCompleteTrigger(t *testing.T) {
	cur := grayVersion(10, "1.0.0", true)
	tgt := grayVersion(20, "2.0.0", false)
	addLine(&tgt, "windows", "x86_64", "pkg.zip")
	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, tgt)
	base := CatalogETag(cat)

	withVersionAllowlist(&cat.Versions[1], "hash-1")
	if got := CatalogETag(cat); got == base {
		t.Fatalf("version allowlist edit must change etag")
	}

	after := CatalogETag(cat)
	done := time.Unix(9, 0).UTC()
	cat.Versions[1].Version.GrayCompletedAt = &done
	if got := CatalogETag(cat); got == after {
		t.Fatalf("gray_completed_at must change etag")
	}
}

func TestDiffGrayGateAllowlistOnly(t *testing.T) {
	cat, details, _, _ := diffFixture()
	markGrayIncomplete(&cat.Versions[1].Version)
	in := diffInput()
	in.DeviceID = "dev-miss"
	in.DeviceHash = "hash-miss"
	if _, err := Diff(diffCtx, cat, details, in); err != ErrPreconditionFailed {
		t.Fatalf("expected ErrPreconditionFailed, got %v", err)
	}

	withVersionAllowlist(&cat.Versions[1], "hash-miss")
	if res, err := Diff(diffCtx, cat, details, in); err != nil || res.Body == nil {
		t.Fatalf("allowlisted device must pass the gray gate: err=%v", err)
	}
}

func TestGrayHitFor_RawPolicyUsesRawDevice(t *testing.T) {
	cur := grayVersion(10, "1.0.0", true)
	tgt := grayVersion(20, "2.0.0", false)
	addLine(&tgt, "windows", "x86_64", "pkg-x64.zip")
	withVersionAllowlist(&tgt, "raw-device-id")
	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, tgt)
	cat.Project.DeviceIDPolicy = model.DeviceIDPolicyRaw

	res, err := SelectTarget(cat, grayReq(&cur, "raw-device-id", "irrelevant-hash"))
	if err != nil || res.Target == nil {
		t.Fatalf("raw policy allowlist must compare raw device id: res=%+v err=%v", res, err)
	}
}

func TestGrayHitFor_CriticalAlwaysHits(t *testing.T) {
	cur := grayVersion(10, "1.0.0", true)
	tgt := grayVersion(20, "2.0.0", false)
	tgt.Version.IsCritical = true
	addLine(&tgt, "windows", "x86_64", "pkg-x64.zip")
	cat := mkCatalog(model.CompareEngineSemver, []ChannelInfo{mkChannel("stable", 30)}, cur, tgt)
	res, err := SelectTarget(cat, defaultReq(&cur))
	if err != nil || res.Target == nil {
		t.Fatalf("critical must hit for anonymous: res=%+v err=%v", res, err)
	}
}
