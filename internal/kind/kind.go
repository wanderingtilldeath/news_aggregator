// Package kind labels a story as opinion, analysis or an explainer from its address alone: no model,
// no storage. Publishers put these in the path (/opinion/, /explained/), and a plain news report has
// none of them, so the label is empty for it. The label is a hint for the reader, not a verdict.
package kind

import "strings"

const (
	Opinion   = "opinion"
	Analysis  = "analysis"
	Explainer = "explainer"
)

// pathMarks are path segments that mark a kind, checked in order: opinion wins over analysis.
var pathMarks = []struct {
	kind  string
	marks []string
}{
	{Opinion, []string{"opinion", "opinions", "editorial", "editorials", "comment", "commentisfree", "column", "columns", "viewpoint", "viewpoints", "op-ed", "oped"}},
	{Explainer, []string{"explained", "explainer", "explainers", "explaining", "in-depth", "long-read", "longread"}},
	{Analysis, []string{"analysis", "analyses", "expert-speak", "insight", "insights"}},
}

// For returns the kind of the story at this URL path, or "" for an ordinary report.
func For(path string) string {
	segments := strings.Split(strings.ToLower(path), "/")
	for _, pm := range pathMarks {
		for _, seg := range segments {
			for _, m := range pm.marks {
				if seg == m || strings.HasPrefix(seg, m+"-") {
					return pm.kind
				}
			}
		}
	}

	return ""
}
