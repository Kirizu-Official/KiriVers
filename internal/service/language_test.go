package service

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/repository"
)

func TestMemoryLanguageHarvestInsert(t *testing.T) {
	ctx := t.Context()
	store := repository.NewMemoryProjectStore()
	p := &model.Project{Slug: "legacy-app", DefaultLocale: "zh-CN"}
	if err := store.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateVersion(ctx, &model.Version{
		ProjectID: p.ID,
		Changelog: model.ChangelogMap{
			"zh-CN": {Markdown: "中文"},
			"en":    {Markdown: "English"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	svc := NewProjectService(store)
	if _, err := svc.SeedProjectLanguage(ctx, p.ID, "zh-CN"); err != nil {
		t.Fatal(err)
	}
	en := "en"
	if _, err := svc.CreateLanguage(ctx, p.ID, LanguageWrite{Code: &en}); err != nil {
		t.Fatal(err)
	}
	list, err := svc.ListLanguages(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("languages=%+v", list)
	}
	codes := map[string]bool{}
	for _, row := range list {
		codes[row.Code] = row.IsDefault
	}
	if !codes["zh-CN"] || codes["en"] {
		t.Fatalf("default flags=%v", codes)
	}
}

func TestCreateProjectSeedsDefaultLanguage(t *testing.T) {
	ctx := t.Context()
	svc := NewProjectService(repository.NewMemoryProjectStore())
	slug := "lang-zh"
	locale := "zh-CN"
	p, _, err := svc.Create(ctx, CreateProjectInput{Slug: &slug, DefaultLocale: &locale})
	if err != nil {
		t.Fatal(err)
	}
	if p.DefaultLocale != "zh-CN" {
		t.Fatalf("default_locale=%q", p.DefaultLocale)
	}
	list, err := svc.ListLanguages(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Code != "zh-CN" || !list[0].IsDefault || list[0].SortOrder != 0 {
		t.Fatalf("seeded=%+v", list)
	}

	enSlug := "lang-en"
	en := "en"
	enProj, _, err := svc.Create(ctx, CreateProjectInput{Slug: &enSlug, DefaultLocale: &en})
	if err != nil {
		t.Fatal(err)
	}
	enList, err := svc.ListLanguages(ctx, enProj.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(enList) != 1 || enList[0].Code != "en" || !enList[0].IsDefault {
		t.Fatalf("en seed=%+v", enList)
	}

	omit := "lang-omit"
	if _, _, err := svc.Create(ctx, CreateProjectInput{Slug: &omit}); !errors.Is(err, ErrInvalidProjectSettings) {
		t.Fatalf("omit default_locale: %v", err)
	}
	empty := ""
	emptySlug := "lang-empty"
	if _, _, err := svc.Create(ctx, CreateProjectInput{Slug: &emptySlug, DefaultLocale: &empty}); !errors.Is(err, ErrInvalidProjectSettings) {
		t.Fatalf("empty default_locale: %v", err)
	}
}

func TestLanguageCRUDDefaultAndDuplicate(t *testing.T) {
	ctx := t.Context()
	store := repository.NewMemoryProjectStore()
	svc := NewProjectService(store)
	slug := "lang-crud"
	locale := "zh-CN"
	p, _, err := svc.Create(ctx, CreateProjectInput{Slug: &slug, DefaultLocale: &locale})
	if err != nil {
		t.Fatal(err)
	}

	ja := "ja"
	name := "日本語"
	added, err := svc.CreateLanguage(ctx, p.ID, LanguageWrite{Code: &ja, DisplayName: &name})
	if err != nil {
		t.Fatal(err)
	}
	if added.Code != "ja" || added.DisplayName != "日本語" || added.IsDefault {
		t.Fatalf("ja=%+v", added)
	}

	dup := "JA"
	if _, err := svc.CreateLanguage(ctx, p.ID, LanguageWrite{Code: &dup}); !errors.Is(err, ErrLanguageTaken) {
		t.Fatalf("case-insensitive dup: %v", err)
	}

	if err := svc.DeleteLanguage(ctx, p.ID, "zh-CN"); !errors.Is(err, ErrLanguageIsDefault) {
		t.Fatalf("delete default: %v", err)
	}

	on := true
	patched, err := svc.PatchLanguage(ctx, p.ID, "ja", LanguageWrite{IsDefault: &on})
	if err != nil {
		t.Fatal(err)
	}
	if !patched.IsDefault {
		t.Fatal("ja should be default")
	}
	got, err := svc.Resolve(ctx, slug)
	if err != nil {
		t.Fatal(err)
	}
	if got.DefaultLocale != "ja" {
		t.Fatalf("project default_locale=%q", got.DefaultLocale)
	}
	zh, err := svc.ListLanguages(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	var zhRow *model.ProjectLanguage
	for i := range zh {
		if zh[i].Code == "zh-CN" {
			zhRow = &zh[i]
		}
	}
	if zhRow == nil || zhRow.IsDefault {
		t.Fatalf("zh-CN should no longer be default: %+v", zh)
	}

	if err := svc.DeleteLanguage(ctx, p.ID, "zh-CN"); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteLanguage(ctx, p.ID, "ja"); !errors.Is(err, ErrLanguageIsDefault) && !errors.Is(err, ErrLanguageLast) {
		t.Fatalf("delete last/default: %v", err)
	}

	fr := "fr"
	if _, _, err := svc.Patch(ctx, slug, PatchProjectInput{DefaultLocale: &fr}); err != nil {
		t.Fatal(err)
	}
	list, err := svc.ListLanguages(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	var foundFR bool
	for _, row := range list {
		if row.Code == "fr" && row.IsDefault {
			foundFR = true
		}
		if row.Code == "ja" && row.IsDefault {
			t.Fatal("ja still default after PATCH default_locale=fr")
		}
	}
	if !foundFR {
		t.Fatalf("PATCH default_locale did not upsert fr: %+v", list)
	}
}

func TestLanguageCapAndSharedCodeAcrossProjects(t *testing.T) {
	ctx := t.Context()
	svc := NewProjectService(repository.NewMemoryProjectStore())
	locale := "zh-CN"
	slugA := "lang-share-a"
	a, _, err := svc.Create(ctx, CreateProjectInput{Slug: &slugA, DefaultLocale: &locale})
	if err != nil {
		t.Fatal(err)
	}
	slugB := "lang-share-b"
	b, _, err := svc.Create(ctx, CreateProjectInput{Slug: &slugB, DefaultLocale: &locale})
	if err != nil {
		t.Fatal(err)
	}
	listA, err := svc.ListLanguages(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	listB, err := svc.ListLanguages(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listA) != 1 || listA[0].Code != "zh-CN" || len(listB) != 1 || listB[0].Code != "zh-CN" {
		t.Fatalf("two projects must share zh-CN: a=%+v b=%+v", listA, listB)
	}

	for i := 0; i < model.ProjectMaxLanguages-1; i++ {
		code := fmt.Sprintf("x%02d", i)
		if _, err := svc.CreateLanguage(ctx, a.ID, LanguageWrite{Code: &code}); err != nil {
			t.Fatalf("add %s: %v", code, err)
		}
	}
	overflow := "zz"
	if _, err := svc.CreateLanguage(ctx, a.ID, LanguageWrite{Code: &overflow}); !errors.Is(err, ErrInvalidLanguage) {
		t.Fatalf("cap: %v", err)
	}
}
