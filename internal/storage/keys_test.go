package storage

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestArtifactObjectKey(t *testing.T) {
	got := ArtifactObjectKey(" MyApp ", "AB"+strings.Repeat("c", 62))
	want := "MyApp/" + "ab" + strings.Repeat("c", 62)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	id := uuid.MustParse("3f45f4a0-775b-4a51-837e-e4afcf0c031f")
	if ArtifactTempRel("slug", id) != "slug/temp/"+id.String() {
		t.Fatal(ArtifactTempRel("slug", id))
	}
	if GeoipObjectKey(id, "City.mmdb") != "geoip/"+id.String()+"/City.mmdb" {
		t.Fatal(GeoipObjectKey(id, "City.mmdb"))
	}
}

func TestPublicObjectURL(t *testing.T) {
	cdn := PublicObjectURL(S3Settings{PublicBaseURL: "https://cdn.example/o/"}, "app/"+strings.Repeat("a", 64))
	if cdn != "https://cdn.example/o/app/"+strings.Repeat("a", 64) {
		t.Fatalf("cdn=%q", cdn)
	}
	pathStyle := PublicObjectURL(S3Settings{
		Endpoint:     "http://127.0.0.1:9000",
		Bucket:       "public",
		UsePathStyle: true,
	}, "slug/hash")
	if pathStyle != "http://127.0.0.1:9000/public/slug/hash" {
		t.Fatalf("path-style=%q", pathStyle)
	}
	vh := PublicObjectURL(S3Settings{
		Endpoint:     "https://s3.amazonaws.com",
		Bucket:       "bucket",
		UsePathStyle: false,
	}, "slug/hash")
	if vh != "https://bucket.s3.amazonaws.com/slug/hash" {
		t.Fatalf("vhost=%q", vh)
	}
}
