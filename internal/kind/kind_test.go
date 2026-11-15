package kind

import "testing"

func TestFor(t *testing.T) {
	for path, want := range map[string]string{
		"/opinion/columns/why-it-matters/": Opinion,
		"/commentisfree/2026/nov/15/x":     Opinion,
		"/editorial/budget":                Opinion,
		"/explained/explained-politics/x":  Explainer,
		"/article/explained/story-slug":    Explainer,
		"/expert-speak/why-india":          Analysis,
		"/news-analysis/x":                 "",
		"/analysis/2026/x":                 Analysis,
		"/news/world/story":                "",
		"/article/india/some-opinionated":  "",
		"/Opinion/Mixed-Case":              Opinion,
		"/opinion-pieces/x":                Opinion,
		"":                                 "",
	} {
		if got := For(path); got != want {
			t.Errorf("For(%q) = %q, want %q", path, got, want)
		}
	}
}
