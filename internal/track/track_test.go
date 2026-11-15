package track

import (
	"reflect"
	"strings"
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

func TestNormalizeTopics(t *testing.T) {
	long := strings.Repeat("x", MaxTopicLen+1)
	got := NormalizeTopics([]string{"  Monsoon  Rains ", "monsoon rains", "", long, "a", "b", "c", "d", "e"})
	want := []string{"monsoon rains", "a", "b", "c", "d"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("NormalizeTopics = %q, want %q", got, want)
	}
	if got := NormalizeTopics(nil); got == nil || len(got) != 0 {
		t.Fatalf("no topics must be an empty slice, got %#v", got)
	}
}
