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
	if cfg.LLMBudget != 12*time.Minute {
		t.Fatalf("LLMBudget default = %v, want 12m", cfg.LLMBudget)
	}
	if cfg.CrawlTimeout != 2*time.Hour {
		t.Fatalf("CrawlTimeout default = %v, want 2h", cfg.CrawlTimeout)
	}
	if !cfg.ReadOnly {
		t.Fatal("POST /crawls must be off by default: it makes the server fetch any URL it is given")
	}
}

func TestLoadReadOnlyFromEnv(t *testing.T) {
	cfg, err := Load(nil, env(map[string]string{"DATABASE_URL": "postgres://x", "READ_ONLY": "true"}), io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	if !cfg.ReadOnly {
		t.Fatal("READ_ONLY=true should set ReadOnly")
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
		{"lease under twice the timeout", []string{"-lease", "25s", "-timeout", "15s"}, map[string]string{"DATABASE_URL": "x"}, "twice"},
		{"llm budget too short", nil, map[string]string{"DATABASE_URL": "x", "LLM_BUDGET": "10s"}, "llm-budget"},
		{"crawl timeout too short", nil, map[string]string{"DATABASE_URL": "x", "CRAWL_TIMEOUT": "10s"}, "crawl-timeout"},
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

func TestMailDefaultsToLogWithLocalLinks(t *testing.T) {
	cfg, err := Load(nil, env(map[string]string{"DATABASE_URL": "postgres://x"}), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MailProvider != "log" || cfg.WebBaseURL != "http://localhost:3000" || cfg.MailFromName != "Postpaper" {
		t.Fatalf("defaults = %+v", cfg)
	}
}

func TestBrevoNeedsKeySenderAndWebAddress(t *testing.T) {
	base := map[string]string{"DATABASE_URL": "postgres://x", "MAIL_PROVIDER": "brevo"}

	_, err := Load(nil, env(base), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "BREVO_API_KEY") || !strings.Contains(err.Error(), "MAIL_FROM") || !strings.Contains(err.Error(), "WEB_BASE_URL") {
		t.Fatalf("err = %v, want all three named", err)
	}

	ok := map[string]string{"DATABASE_URL": "postgres://x", "MAIL_PROVIDER": "brevo", "BREVO_API_KEY": "k", "MAIL_FROM": "digest@example.com", "WEB_BASE_URL": "https://digest.example.com/"}
	cfg, err := Load(nil, env(ok), io.Discard)
	if err != nil || cfg.WebBaseURL != "https://digest.example.com" {
		t.Fatalf("cfg = %+v, %v (trailing slash must go)", cfg, err)
	}

	bad := map[string]string{"DATABASE_URL": "postgres://x", "MAIL_PROVIDER": "brevo", "BREVO_API_KEY": "k", "MAIL_FROM": "Name <digest@example.com", "WEB_BASE_URL": "ftp://x"}
	if _, err := Load(nil, env(bad), io.Discard); err == nil || !strings.Contains(err.Error(), "MAIL_FROM") || !strings.Contains(err.Error(), "WEB_BASE_URL") {
		t.Fatalf("err = %v, want bad address and bad URL rejected", err)
	}

	if _, err := Load(nil, env(map[string]string{"DATABASE_URL": "postgres://x", "MAIL_PROVIDER": "smtp"}), io.Discard); err == nil {
		t.Fatal("unknown provider accepted")
	}
}

func TestTelegramSettings(t *testing.T) {
	base := map[string]string{"DATABASE_URL": "postgres://x"}
	with := func(extra map[string]string) map[string]string {
		m := map[string]string{}
		for k, v := range base {
			m[k] = v
		}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}

	cfg, err := Load(nil, env(with(map[string]string{"TELEGRAM_BOT_TOKEN": "1:abc", "TELEGRAM_CHAT_UPSC": "@postpaper_upsc", "TELEGRAM_CHAT_ENGINEERING": "-1001234567890"})), io.Discard)
	if err != nil || cfg.TelegramChats["upsc"] != "@postpaper_upsc" || cfg.TelegramChats["engineering"] != "-1001234567890" {
		t.Fatalf("cfg = %+v, %v", cfg, err)
	}
	if cfg, err = Load(nil, env(base), io.Discard); err != nil || len(cfg.TelegramChats) != 0 {
		t.Fatalf("default = %+v, %v, want no channels", cfg.TelegramChats, err)
	}
	if _, err = Load(nil, env(with(map[string]string{"TELEGRAM_CHAT_UPSC": "@postpaper_upsc"})), io.Discard); err == nil || !strings.Contains(err.Error(), "TELEGRAM_BOT_TOKEN") {
		t.Fatalf("a channel without a token: %v", err)
	}
	if _, err = Load(nil, env(with(map[string]string{"TELEGRAM_BOT_TOKEN": "t", "TELEGRAM_CHAT_UPSC": "not a chat"})), io.Discard); err == nil || !strings.Contains(err.Error(), "@channelname") {
		t.Fatalf("a bad chat id: %v", err)
	}
}

func TestBlockedHostsFromEnv(t *testing.T) {
	cfg, err := Load(nil, env(map[string]string{"DATABASE_URL": "postgres://x", "BLOCKED_HOSTS": " a.test, www.b.test ,,"}), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(cfg.BlockedHosts, "|"); got != "a.test|www.b.test" {
		t.Fatalf("BlockedHosts = %q", got)
	}
}

func TestProPlans(t *testing.T) {
	cfg, err := Load(nil, env(map[string]string{"DATABASE_URL": "postgres://x"}), io.Discard)
	if err != nil || cfg.ProPlans[9900] != 31 || cfg.ProPlans[24900] != 92 || len(cfg.ProPlans) != 2 {
		t.Fatalf("default plans = %v, %v; want 99 rupees for 31 days and 249 for 92", cfg.ProPlans, err)
	}

	cfg, err = Load(nil, env(map[string]string{"DATABASE_URL": "postgres://x", "PRO_PLANS": " 4900:15 , 49900:366 "}), io.Discard)
	if err != nil || cfg.ProPlans[4900] != 15 || cfg.ProPlans[49900] != 366 {
		t.Fatalf("custom plans = %v, %v", cfg.ProPlans, err)
	}

	for _, bad := range []string{"9900", "9900:", "x:31", "9900:0", "-5:31", "9900:31,9900:60"} {
		if _, err := ParsePlans(bad); err == nil {
			t.Errorf("ParsePlans(%q) accepted a bad plan", bad)
		}
	}
}

func TestTeamPostsIsOffUnlessSet(t *testing.T) {
	cfg, err := Load(nil, env(map[string]string{"DATABASE_URL": "postgres://x"}), io.Discard)
	if err != nil || cfg.TeamPosts {
		t.Fatalf("default = %v, %v; want off", cfg.TeamPosts, err)
	}
	cfg, err = Load(nil, env(map[string]string{"DATABASE_URL": "postgres://x", "TEAM_POSTS": "true"}), io.Discard)
	if err != nil || !cfg.TeamPosts {
		t.Fatalf("TEAM_POSTS=true = %v, %v; want on", cfg.TeamPosts, err)
	}
}

func TestAdminEmailsAreTrimmedAndLowerCased(t *testing.T) {
	cfg, err := Load(nil, env(map[string]string{"DATABASE_URL": "postgres://x", "ADMIN_EMAILS": " Boss@Example.com, ,second@x.test "}), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(cfg.AdminEmails, "|"); got != "boss@example.com|second@x.test" {
		t.Fatalf("AdminEmails = %q", got)
	}

	cfg, _ = Load(nil, env(map[string]string{"DATABASE_URL": "postgres://x"}), io.Discard)
	if len(cfg.AdminEmails) != 0 || cfg.AdminPassword != "" {
		t.Fatalf("no ADMIN_EMAILS or ADMIN_PASSWORD must mean no admins, got %v %q", cfg.AdminEmails, cfg.AdminPassword)
	}

	cfg, _ = Load(nil, env(map[string]string{"DATABASE_URL": "postgres://x", "ADMIN_PASSWORD": "a long enough password"}), io.Discard)
	if cfg.AdminPassword != "a long enough password" {
		t.Fatalf("AdminPassword = %q", cfg.AdminPassword)
	}
}
