package track

import (
	"reflect"
	"testing"
)

func TestNormalize(t *testing.T) {
	for _, tc := range []struct {
		tracks   []string
		mode     string
		wantT    []string
		wantMode string
	}{
		{nil, "", []string{UPSC, Engineering}, Mode1},
		{[]string{"engineering", "upsc", "upsc", "bogus"}, "7min", []string{UPSC, Engineering}, Mode7},
		{[]string{"engineering"}, "weird", []string{Engineering}, Mode1},
		{[]string{"bogus"}, Mode7, []string{UPSC, Engineering}, Mode7},
	} {
		gotT, gotMode := Normalize(tc.tracks, tc.mode)
		if !reflect.DeepEqual(gotT, tc.wantT) || gotMode != tc.wantMode {
			t.Errorf("Normalize(%v, %q) = %v, %q, want %v, %q", tc.tracks, tc.mode, gotT, gotMode, tc.wantT, tc.wantMode)
		}
	}
}

func TestEveryTrackHasCategories(t *testing.T) {
	for _, tr := range All() {
		if len(Categories(tr)) == 0 || Title(tr) == "" {
			t.Errorf("track %q has no categories or title", tr)
		}
	}
}
