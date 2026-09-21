package repository

import (
	"testing"

	"github.com/google/uuid"

	"github.com/Kirizu-Official/KiriVers/internal/model"
)

func TestMemoryInstallPolicyReplaceAndChannelDelete(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	store := NewMemoryInstallPolicyRuleStore()
	projectID := uuid.Must(uuid.NewRandom())
	channelID := uuid.Must(uuid.NewRandom())

	err := store.Replace(ctx, projectID, uuid.Nil, "windows", "x86_64", []model.InstallPolicyRule{
		{Path: "config/user.json", InstallPolicy: model.InstallPolicyKeepIfExists},
		{Path: "bin/app.exe", InstallPolicy: model.InstallPolicyOverwrite},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.List(ctx, projectID, uuid.Nil, "windows", "x86_64")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Path != "bin/app.exe" || got[1].Path != "config/user.json" {
		t.Fatalf("project list=%+v", got)
	}

	if err := store.Replace(ctx, projectID, channelID, "windows", "x86_64", []model.InstallPolicyRule{
		{Path: "debug.log", InstallPolicy: model.InstallPolicyKeepIfExists},
	}); err != nil {
		t.Fatal(err)
	}
	ch, err := store.List(ctx, projectID, channelID, "windows", "x86_64")
	if err != nil {
		t.Fatal(err)
	}
	if len(ch) != 1 || ch[0].Path != "debug.log" {
		t.Fatalf("channel list=%+v", ch)
	}

	if err := store.Replace(ctx, projectID, uuid.Nil, "windows", "x86_64", nil); err != nil {
		t.Fatal(err)
	}
	empty, err := store.List(ctx, projectID, uuid.Nil, "windows", "x86_64")
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("cleared project still has %d", len(empty))
	}
	ch, _ = store.List(ctx, projectID, channelID, "windows", "x86_64")
	if len(ch) != 1 {
		t.Fatalf("channel rules must survive project replace, got %d", len(ch))
	}

	if err := store.DeleteByChannel(ctx, projectID, channelID); err != nil {
		t.Fatal(err)
	}
	ch, _ = store.List(ctx, projectID, channelID, "windows", "x86_64")
	if len(ch) != 0 {
		t.Fatalf("channel delete leftover=%+v", ch)
	}
}
