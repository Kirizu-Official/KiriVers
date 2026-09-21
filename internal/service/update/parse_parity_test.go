package update_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
)

// TestParseParityWithService 逐例对照 update 包的版本引用解析与
// service.ParseVersionRef 的语义一致性。
//
// 背景：CatalogLoader 的 GORM 实现位于 internal/repository，而 internal/service
// 依赖 internal/repository；internal/service/update（非测试代码）引用 service
// 包会形成 repository → update → service → repository 导入环，因此 update 包内
// 实现了同等语义的解析器。本测试以外部测试包（不触发测试期导入重写）通过
// 导出行为对照两者：
//   - service 解析失败 ⇔ update.Check 返回 ErrEngineMismatch；
//   - service 解析成功 ⇔ update.Check 不返回 ErrEngineMismatch
//     （命中夹具版本返回 nil，未命中返回 ErrVersionNotFound）；
//   - "010"=10、"v1.2.3+build"=1.2.3 经夹具直查验证同记录解析。
func TestParseParityWithService(t *testing.T) {
	cur := model.Version{
		ID:                     uuid.New(),
		ChannelSlug:            "stable",
		Status:                 model.VersionStatusPublished,
		VersionInteger:         ptrI(10),
		VersionSemver:          ptrS("1.0.0"),
		VersionSemverCanonical: ptrS("1.0.0"),
		Changelog:              model.ChangelogMap{},
	}
	cat := &update.Catalog{
		Project: update.ProjectInfo{CompareEngine: model.CompareEngineSemver, CacheSMaxageSeconds: 60, DefaultLocale: "en"},
		Versions: []update.VersionState{
			{Version: cur},
		},
	}

	cases := []string{
		"10", "010", "1", "007",
		"1.0.0", "v1.0.0", "V1.0.0", "1.0.0+nightly", "v1.0.0+20260912", " 1.0.0 ",
		"1.2.3", "1.2.3-beta.1",
		"1.2", "1.2.3.4", "0", "-5", "", "   ", "not-a-version", "01.2.3",
	}
	for _, raw := range cases {
		_, svcErr := service.ParseVersionRef(raw)
		_, checkErr := update.Check(cat, update.CheckInput{OS: "windows", Arch: "x86_64", CurrentVersion: raw})
		if svcErr != nil {
			if !errors.Is(checkErr, update.ErrEngineMismatch) {
				t.Fatalf("parse disagreement for %q: service rejects (%v), update accepts (%v)", raw, svcErr, checkErr)
			}
			continue
		}
		if errors.Is(checkErr, update.ErrEngineMismatch) {
			t.Fatalf("parse disagreement for %q: service accepts, update rejects", raw)
		}
		if errors.Is(checkErr, update.ErrVersionNotFound) {
			// 合法但不在夹具中（如 1.2.3）——解析路径一致即可。
			continue
		}
		if checkErr != nil {
			t.Fatalf("unexpected error for %q: %v", raw, checkErr)
		}
	}
}

// TestIntegerAndSemverResolveSameRecord 102 与 1.2.3（此处 10/1.0.0 同理）
// 必须解析到同一 Version 记录（验收项 C08：解析同一记录）。
func TestIntegerAndSemverResolveSameRecord(t *testing.T) {
	cur := model.Version{
		ID:                     uuid.New(),
		ChannelSlug:            "stable",
		Status:                 model.VersionStatusPublished,
		VersionInteger:         ptrI(102),
		VersionSemver:          ptrS("1.2.3"),
		VersionSemverCanonical: ptrS("1.2.3"),
		Changelog:              model.ChangelogMap{},
	}
	cat := &update.Catalog{
		Project:  update.ProjectInfo{CompareEngine: model.CompareEngineSemver, CacheSMaxageSeconds: 60},
		Versions: []update.VersionState{{Version: cur}},
	}
	for _, raw := range []string{"102", "1.2.3", "v1.2.3", "1.2.3+build.7", "0102"} {
		res, err := update.Check(cat, update.CheckInput{OS: "windows", Arch: "x86_64", CurrentVersion: raw})
		if err != nil || res.Status != 204 {
			t.Fatalf("current_version=%q must resolve to the 1.2.3 record: err=%v status=%d", raw, err, res.Status)
		}
	}
}

func ptrI(v int64) *int64   { return &v }
func ptrS(v string) *string { return &v }
