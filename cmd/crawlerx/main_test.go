package main

import "testing"

func TestUserAgentProduct(t *testing.T) {
	tests := map[string]string{
		"CrawlerX/1.0 (+https://example.test/bot)": "CrawlerX",
		"MyBot":   "MyBot",
		"Bot 2.0": "Bot",
		"":        "",
	}
	for in, want := range tests {
		if got := userAgentProduct(in); got != want {
			t.Errorf("userAgentProduct(%q) = %q, want %q", in, got, want)
		}
	}
}
