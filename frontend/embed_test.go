package frontend

import (
	"io/fs"
	"testing"
)

func TestDistHasPlaceholderOrIndex(t *testing.T) {
	fsys := Dist()
	_, keepErr := fs.Stat(fsys, ".gitkeep")
	_, indexErr := fs.Stat(fsys, "index.html")
	if keepErr != nil && indexErr != nil {
		t.Fatal("embedded frontend/dist must contain .gitkeep or index.html")
	}
}
