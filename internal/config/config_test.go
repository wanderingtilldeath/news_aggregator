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
