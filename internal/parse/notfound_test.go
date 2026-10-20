package parse

import "testing"

func TestLooksLikeNotFound(t *testing.T) {
	cases := []struct {
		name string
		page Page
		want bool
	}{
		{"drishti soft 404", Page{Title: "Not Found", BodyImage: "https://www.drishtiias.com/drishti/img/error.png"}, true},
		{"title only", Page{Title: "Page Not Found"}, true},
		{"404 with site name", Page{Title: "404 | Example"}, true},
		{"oops", Page{Title: "Oops! Something went wrong"}, true},
		{"heading says so", Page{Title: "Example", H1: "404 - Not Found"}, true},
		{"error graphic only", Page{Title: "Example", BodyImage: "http://x.test/img/ERROR.png?v=2"}, true},
		{"real article mentioning it", Page{Title: "Why features are not found by users"}, false},
		{"title starting with a longer word", Page{Title: "404ing pages: a field guide"}, false},
		{"infographic page with tiny text", Page{Title: "President of India", BodyImage: "http://x.test/images/uploads/murmu.jpg", Snippet: "Infographics"}, false},
		{"empty page", Page{}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.page.LooksLikeNotFound(); got != tc.want {
				t.Fatalf("LooksLikeNotFound(%+v) = %v, want %v", tc.page, got, tc.want)
			}
		})
	}
}
