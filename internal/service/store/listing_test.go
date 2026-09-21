package store

import (
	"context"
	"strings"
	"testing"

	"github.com/Kirizu-Official/KiriVers/internal/model"
	"github.com/Kirizu-Official/KiriVers/internal/service/update"
)

func TestMatchManifestPathSetupExe(t *testing.T) {
	setupSHA := strings.Repeat("a", 64)
	otherSHA := strings.Repeat("c", 64)
	detail := &update.LineDetail{
		Manifest: []model.ManifestEntry{
			{Path: "Setup.exe", SHA256: setupSHA, Size: 12},
			{Path: "payload.dll", SHA256: otherSHA, Size: 8},
		},
		Files: []update.FileArtifactInfo{
			{FileName: "Setup.exe", SHA256: setupSHA, Size: 12},
			{FileName: "payload.dll", SHA256: otherSHA, Size: 8},
		},
	}
	got := matchManifestPathFile(detail, "Setup.exe")
	if got == nil || got.SHA256 != setupSHA {
		t.Fatalf("Setup.exe must resolve to that file's sha256, got %+v", got)
	}
	if matchManifestPathFile(detail, "Missing.exe") != nil {
		t.Fatal("unknown selector must not resolve")
	}
}

func TestMatchManifestPathViaManifestSHA(t *testing.T) {
	setupSHA := strings.Repeat("d", 64)
	detail := &update.LineDetail{
		Manifest: []model.ManifestEntry{{Path: "installer/Setup.exe", SHA256: setupSHA}},
		Files:    []update.FileArtifactInfo{{FileName: "win-setup.exe", SHA256: setupSHA, Size: 4}},
	}
	got := matchManifestPathFile(detail, "installer/Setup.exe")
	if got == nil || got.SHA256 != setupSHA {
		t.Fatalf("manifest path must map to file bytes by sha256, got %+v", got)
	}
}

func TestResolveStorePackageLineFullSHASeparated(t *testing.T) {
	native := &update.ArtifactInfo{SHA256: strings.Repeat("a", 64), FileName: "app-full.zip", Size: 10}
	feedPkg := update.ArtifactInfo{SHA256: strings.Repeat("b", 64), FileName: "app-store.zip", Size: 12}
	line := &update.LineState{
		RootHash:     strings.Repeat("c", 64),
		FullPkgs:     []update.ArtifactInfo{*native},
		StoreFullPkgs: []update.ArtifactInfo{feedPkg},
	}
	req := Request{Listing: &model.StoreListing{PackageSource: model.PackageSourceLineFull}}
	got, ok := resolveStorePackage(context.Background(), nil, req, line, native)
	if !ok || got == nil {
		t.Fatal("line_full must resolve store_full")
	}
	if got.SHA256 != feedPkg.SHA256 {
		t.Fatalf("line_full SHA=%s want store_full %s not native %s", got.SHA256, feedPkg.SHA256, native.SHA256)
	}
}

func TestResolveStorePackageMissingStoreFullSkipsMultiFile(t *testing.T) {
	native := &update.ArtifactInfo{SHA256: strings.Repeat("a", 64), FileName: "app-full.zip", Size: 10}
	line := &update.LineState{
		RootHash: strings.Repeat("c", 64),
		FullPkgs: []update.ArtifactInfo{*native},
	}
	req := Request{Listing: &model.StoreListing{PackageSource: model.PackageSourceLineFull}}
	got, ok := resolveStorePackage(context.Background(), nil, req, line, native)
	if ok || got != nil {
		t.Fatalf("multi-file without store_full must skip, got ok=%v pkg=%+v", ok, got)
	}
}

func TestResolveFeedPackageSingleFileStillUsesFull(t *testing.T) {
	native := &update.ArtifactInfo{SHA256: strings.Repeat("d", 64), FileName: "app.dmg", Size: 8}
	line := &update.LineState{FullPkgs: []update.ArtifactInfo{*native}}
	req := Request{Listing: &model.StoreListing{PackageSource: model.PackageSourceLineFull}}
	got, ok := resolveStorePackage(context.Background(), nil, req, line, native)
	if !ok || got == nil || got.SHA256 != native.SHA256 {
		t.Fatalf("single-file line_full must keep kind=full, got ok=%v pkg=%+v", ok, got)
	}
}
