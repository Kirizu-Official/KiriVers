package model

import "testing"

func TestEffectiveFileListMax(t *testing.T) {
	t.Parallel()
	cases := []struct {
		platform, project, want int
	}{
		{0, 0, DefaultFileListMaxFiles},
		{-1, 0, DefaultFileListMaxFiles},
		{16, 0, 16},
		{16, 4, 4},
		{16, 32, 16},
		{8, 0, 8},
		{8, 8, 8},
	}
	for _, tc := range cases {
		if got := EffectiveFileListMax(tc.platform, tc.project); got != tc.want {
			t.Fatalf("EffectiveFileListMax(%d,%d)=%d want %d", tc.platform, tc.project, got, tc.want)
		}
	}
}
