package database

import (
	"reflect"
	"testing"
)

func TestExpandLegacyAnnouncementMap(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"zh-CN":{"title":"你好","subtitle":"副","markdown":"# md"},"en":{"title":"Hello","subtitle":"","markdown":""}}`)
	got := expandLegacyAnnouncementMap(raw)
	want := []expandedAnnouncementLocale{
		{Language: "en", Title: "Hello"},
		{Language: "zh-CN", Title: "你好", Subtitle: "副", Content: "# md"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got=%+v want=%+v", got, want)
	}
	if expandLegacyAnnouncementMap(nil) != nil {
		t.Fatal("nil raw")
	}
	if expandLegacyAnnouncementMap([]byte(`{}`)) != nil {
		t.Fatal("empty object")
	}
}
