package database

import (
	"testing"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

func TestHarvestLocaleCodes(t *testing.T) {
	t.Parallel()
	got := HarvestLocaleCodes("zh-CN", []string{"zh-CN", "ja", "en", "JA", ""}, []string{"en", "fr"})
	want := []string{"en", "fr", "ja"}
	if len(got) != len(want) {
		t.Fatalf("got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got=%v want=%v", got, want)
		}
	}
	empty := HarvestLocaleCodes("en")
	if len(empty) != 0 {
		t.Fatalf("empty harvest=%v", empty)
	}
}

func TestHarvestLocaleCodesSkipsInvalid(t *testing.T) {
	t.Parallel()
	got := HarvestLocaleCodes("en", []string{"x", "zh.CN", "ja", "not a code", "ko"})
	want := []string{"ja", "ko"}
	if len(got) != len(want) {
		t.Fatalf("got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got=%v want=%v", got, want)
		}
	}
}

func TestBackfillDefaultCode(t *testing.T) {
	t.Parallel()
	if got := backfillDefaultCode("zh-CN"); got != "zh-CN" {
		t.Fatalf("valid=%q", got)
	}
	if got := backfillDefaultCode(""); got != model.LegacyDefaultLocale {
		t.Fatalf("empty=%q", got)
	}
	if got := backfillDefaultCode("x"); got != model.LegacyDefaultLocale {
		t.Fatalf("invalid=%q", got)
	}
}

func TestTrimLanguageHarvest(t *testing.T) {
	t.Parallel()
	extras := make([]string, model.ProjectMaxLanguages)
	for i := range extras {
		extras[i] = "xx"
	}
	got := trimLanguageHarvest(extras, 1)
	if len(got) != model.ProjectMaxLanguages-1 {
		t.Fatalf("len=%d", len(got))
	}
	if trimLanguageHarvest(extras, model.ProjectMaxLanguages) != nil {
		t.Fatal("full table should harvest nothing")
	}
}
