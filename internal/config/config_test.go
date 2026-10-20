package config

import (
	"io"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(nil, env(map[string]string{"DATABASE_URL": "postgres://x"}), io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Workers != 8 || cfg.HostDelay != time.Second || cfg.MaxBytes != 2<<20 || cfg.ListenAddr != ":8080" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if !strings.Contains(cfg.UserAgent, "CrawlerX") || !strings.Contains(cfg.UserAgent, "+http") {
		t.Fatalf("default UserAgent %q should identify the crawler with a contact URL", cfg.UserAgent)
	}
	if cfg.Seed != "" {
		t.Fatalf("there must be no hardcoded seed, got %q", cfg.Seed)
	}
}

func TestLoadEnvThenFlagPrecedence(t *testing.T) {
	cfg, err := Load(
		[]string{"-workers", "3", "-allowed-hosts", "a.test, b.test:81"},
		env(map[string]string{"DATABASE_URL": "postgres://x", "WORKERS": "20", "HOST_DELAY": "250ms", "SEED": "http://a.test/"}),
		io.Discard,
	)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Workers != 3 {
		t.Errorf("Workers = %d, want the flag (3) to beat the env (20)", cfg.Workers)
	}
	if cfg.HostDelay != 250*time.Millisecond {
		t.Errorf("HostDelay = %v, want env value 250ms", cfg.HostDelay)
	}
	if cfg.Seed != "http://a.test/" {
		t.Errorf("Seed = %q", cfg.Seed)
	}
	if len(cfg.AllowedHosts) != 2 || cfg.AllowedHosts[1] != "b.test:81" {
		t.Errorf("AllowedHosts = %v", cfg.AllowedHosts)
	}
}

func TestLoadRejectsBadInput(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
		want string
	}{
		{"missing database", nil, nil, "database URL is required"},
		{"bad env int", nil, map[string]string{"DATABASE_URL": "x", "WORKERS": "many"}, "WORKERS"},
		{"bad env duration", nil, map[string]string{"DATABASE_URL": "x", "HOST_DELAY": "soon"}, "HOST_DELAY"},
		{"negative workers", []string{"-workers", "-1"}, map[string]string{"DATABASE_URL": "x"}, "workers"},
		{"lease shorter than timeout", []string{"-lease", "5s", "-timeout", "10s"}, map[string]string{"DATABASE_URL": "x"}, "lease"},
		{"exit without seed", []string{"-exit-when-done"}, map[string]string{"DATABASE_URL": "x"}, "needs -seed"},
		{"bad depth", []string{"-max-depth", "99"}, map[string]string{"DATABASE_URL": "x"}, "max-depth"},
		{"bad log format", []string{"-log-format", "xml"}, map[string]string{"DATABASE_URL": "x"}, "log-format"},
		{"unknown flag", []string{"-nope"}, map[string]string{"DATABASE_URL": "x"}, "nope"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(tt.args, env(tt.env), io.Discard)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestDigestSettings(t *testing.T) {
	base := map[string]string{"DATABASE_URL": "postgres://x"}
	with := func(kv ...string) map[string]string {
		m := map[string]string{}
		for k, v := range base {
			m[k] = v
		}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return m
	}

	cfg, err := Load(nil, env(base), io.Discard)
	if err != nil || cfg.SourcesFile != "" || cfg.DigestAt != "06:00" || cfg.Timezone != "Asia/Kolkata" || cfg.SummaryProvider != "auto" {
		t.Fatalf("defaults: %+v, %v", cfg, err)
	}

	cfg, err = Load([]string{"-sources", "s.json", "-digest-at", "07:30"}, env(with("GEMINI_API_KEY", "k", "TIMEZONE", "UTC")), io.Discard)
	if err != nil || cfg.SourcesFile != "s.json" || cfg.DigestAt != "07:30" || cfg.GeminiAPIKey != "k" || cfg.Timezone != "UTC" {
		t.Fatalf("explicit: %+v, %v", cfg, err)
	}

	for name, args := range map[string][]string{
		"bad time":      {"-sources", "s.json", "-digest-at", "25:00"},
		"bad zone":      {"-sources", "s.json", "-timezone", "Mars/Base"},
		"bad provider":  {"-summary-provider", "gpt"},
		"gemini no key": {"-summary-provider", "gemini"},
	} {
		if _, err := Load(args, env(base), io.Discard); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}

	// The key is never a flag, so it cannot leak into a process listing.
	if _, err := Load([]string{"-gemini-api-key", "k"}, env(base), io.Discard); err == nil {
		t.Error("-gemini-api-key must not exist as a flag")
	}
}

func TestSummaryTuning(t *testing.T) {
	base := map[string]string{"DATABASE_URL": "postgres://x"}

	cfg, err := Load(nil, env(base), io.Discard)
	if err != nil || cfg.SummaryBatch != 6 || cfg.SummaryWorkers != 3 || cfg.SummaryWindowDays != 7 || cfg.ImageCacheMB != 128 || cfg.MaxSites != 0 {
		t.Fatalf("defaults: %+v, %v", cfg, err)
	}

	cfg, err = Load([]string{"-summary-batch", "10"}, env(map[string]string{"DATABASE_URL": "postgres://x", "SUMMARY_WORKERS": "2", "SUMMARY_WINDOW_DAYS": "14"}), io.Discard)
	if err != nil || cfg.SummaryBatch != 10 || cfg.SummaryWorkers != 2 || cfg.SummaryWindowDays != 14 {
		t.Fatalf("explicit: %+v, %v", cfg, err)
	}

	for name, args := range map[string][]string{
		"batch 0":   {"-summary-batch", "0"},
		"batch 21":  {"-summary-batch", "21"},
		"workers 0": {"-summary-workers", "0"},
		"workers 9": {"-summary-workers", "9"},
		"window 0":  {"-summary-window-days", "0"},
		"window 31": {"-summary-window-days", "31"},
	} {
		if _, err := Load(args, env(base), io.Discard); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
