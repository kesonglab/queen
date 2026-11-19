package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------- util ----------

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "—"},
		{-1, "—"},
		{512, "512B"},
		{1024, "1.0KiB"},
		{1536, "1.5KiB"},
		{1024 * 1024, "1.0MiB"},
		{1024 * 1024 * 1024, "1.0GiB"},
	}
	for _, c := range cases {
		if got := humanBytes(c.in); got != c.want {
			t.Errorf("humanBytes(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFmtSecAndDuration(t *testing.T) {
	cases := map[int]string{0: "0:00", 9: "0:09", 65: "1:05", 3600: "1:00:00", 3725: "1:02:05"}
	for in, want := range cases {
		if got := fmtSec(in); got != want {
			t.Errorf("fmtSec(%d) = %q, want %q", in, got, want)
		}
	}
	if got := humanDuration(0); got != "0s" {
		t.Errorf("humanDuration(0) = %q", got)
	}
	if got := humanDuration(90 * 1e9); got != "1m 30s" {
		t.Errorf("humanDuration(90s) = %q", got)
	}
}

func TestTruncateAndPadWidth(t *testing.T) {
	if got := truncateWidth("abcdef", 4); displayWidth(got) > 4 {
		t.Errorf("truncateWidth produced width %d", displayWidth(got))
	}
	if got := truncateWidth("这是一个很长的中文标题", 8); displayWidth(got) > 8 {
		t.Errorf("cjk truncate width = %d (%q)", displayWidth(got), got)
	}
	if got := padWidth("ab", 5); got != "ab   " {
		t.Errorf("padWidth = %q", got)
	}
	if got := padWidth("这是一个长标题", 4); got != "这是一个长标题" {
		t.Errorf("padWidth should not cut: %q", got)
	}
}

func TestHostOf(t *testing.T) {
	cases := map[string]string{
		"https://www.youtube.com/watch?v=x":   "www.youtube.com",
		"http://user:pass@example.com:8080/x": "example.com",
		"https://x.com":                       "x.com",
		"not a url":                           "not a url",
		"":                                    "",
	}
	for in, want := range cases {
		if got := hostOf(in); got != want {
			t.Errorf("hostOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseURLsAndDedupe(t *testing.T) {
	got := parseURLs("https://a.example/x\nhttps://a.example/x http://b.example/y ftp://no not-a-url\nhttps://c.example/z,")
	want := []string{"https://a.example/x", "http://b.example/y", "https://c.example/z"}
	if len(got) != len(want) {
		t.Fatalf("parseURLs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseURLs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if out := dedupeURLs([]string{"a", "b", "a", " "}); len(out) != 2 {
		t.Fatalf("dedupeURLs = %v", out)
	}
}

func TestStripANSI(t *testing.T) {
	in := "\x1b[31mred\x1b[0m and \x1b]0;title\x07plain"
	if got := stripANSI(in); got != "red and plain" {
		t.Fatalf("stripANSI = %q", got)
	}
	if got := stripANSI("no ansi"); got != "no ansi" {
		t.Fatalf("stripANSI plain = %q", got)
	}
}

func TestCycleChoice(t *testing.T) {
	opts := []string{"a", "b", "c"}
	if got := cycleChoice("a", opts, 1); got != "b" {
		t.Errorf("got %q", got)
	}
	if got := cycleChoice("a", opts, -1); got != "c" {
		t.Errorf("got %q", got)
	}
	if got := cycleChoice("unknown", opts, 1); got != "a" {
		t.Errorf("unknown forward = %q", got)
	}
	if got := cycleChoice("unknown", opts, -1); got != "c" {
		t.Errorf("unknown backward = %q", got)
	}
	if got := cycleInt(8, []int{4, 8, 16}, 1); got != 16 {
		t.Errorf("cycleInt = %d", got)
	}
}

func TestClampHelpers(t *testing.T) {
	if got := clampInt(100, 0, 10); got != 10 {
		t.Errorf("clampInt = %d", got)
	}
	if got := clampFloat(1.5, 0, 1); got != 1 {
		t.Errorf("clampFloat = %v", got)
	}
	if got := clampScroll(-5, 10, 3); got != 0 {
		t.Errorf("clampScroll(-5) = %d", got)
	}
	if got := clampScroll(100, 10, 3); got != 7 {
		t.Errorf("clampScroll(100) = %d", got)
	}
}

// ---------- config ----------

func TestConfigDefaults(t *testing.T) {
	c := defaultConfig()
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"version", c.ConfigVersion, configVersion},
		{"concurrency", c.Concurrency, 4},
		{"fragments", c.Fragments, 8},
		{"connection budget", c.ConnectionBudget, 32},
		{"same site", c.SameSiteLimit, 2},
		{"retries", c.RetryTimes, 2},
		{"retry sleep", c.RetrySleep, 3},
		{"throttled", c.ThrottledRate, "100K"},
		{"trim", c.TrimFilenames, 200},
		{"ignore config", c.IgnoreYtdlpConfig, true},
		{"playlist", c.Playlist, false},
		{"playlist folder", c.PlaylistFolder, true},
		{"audio format", c.AudioFormat, "m4a"},
		{"embed", c.Embed, false},
		{"archive", c.Archive, false},
		{"privacy", c.PrivacyMode, false},
		{"failure logs", c.KeepFailureLogs, true},
		{"update check", c.UpdateCheck, true},
		{"theme", c.Theme, "auto"},
		{"lang", c.Lang, "zh"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestConfigNormalizeClamps(t *testing.T) {
	c := defaultConfig()
	c.Concurrency = 99
	c.Fragments = 0
	c.ConnectionBudget = 9999
	c.SameSiteLimit = 0
	c.RetryTimes = -3
	c.RetrySleep = 999
	c.Theme = "neon"
	c.Lang = "fr"
	c.FormatPreset = "unknown"
	c.MergeFormat = "avi"
	c.AudioFormat = "wav"
	c.DownloadDir = "   "
	c.TrimFilenames = 1
	c.normalize()
	if c.Concurrency != 16 || c.Fragments != 1 || c.ConnectionBudget != 64 || c.SameSiteLimit != 1 {
		t.Errorf("clamps wrong: %+v", c)
	}
	if c.RetryTimes != 0 || c.RetrySleep != 60 {
		t.Errorf("retry clamps wrong: %d %d", c.RetryTimes, c.RetrySleep)
	}
	if c.Theme != "auto" || c.Lang != "zh" || c.FormatPreset != "quality" {
		t.Errorf("enum fallbacks wrong: %+v", c)
	}
	if c.MergeFormat != "mp4" || c.AudioFormat != "m4a" {
		t.Errorf("format fallbacks wrong: %+v", c)
	}
	if strings.TrimSpace(c.DownloadDir) == "" {
		t.Error("empty download dir should fall back to ~/Downloads")
	}
	if c.TrimFilenames != 60 {
		t.Errorf("trim filenames = %d, want 60", c.TrimFilenames)
	}
}

func TestConfigSaveLoadRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	c := defaultConfig()
	c.Concurrency = 7
	c.Theme = "light"
	c.Lang = "en"
	c.DownloadDir = filepath.Join(home, "Videos")
	if err := c.save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	info, err := os.Stat(configPath())
	if err != nil {
		t.Fatalf("config file missing: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("config permissions = %v, want 0600", info.Mode().Perm())
	}
	loaded, notices := loadConfig()
	if len(notices) != 0 {
		t.Fatalf("unexpected notices: %+v", notices)
	}
	if loaded.Concurrency != 7 || loaded.Theme != "light" || loaded.Lang != "en" {
		t.Fatalf("roundtrip lost values: %+v", loaded)
	}
}

func TestConfigCorruptBackup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(configDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath(), []byte("{this is not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, notices := loadConfig()
	if cfg.Concurrency != 4 {
		t.Errorf("corrupt config should fall back to defaults, got %+v", cfg)
	}
	if len(notices) != 1 || notices[0].key != "notice_config_corrupt_backup" {
		t.Errorf("expected corrupt-backup notice, got %+v", notices)
	}
	matches, _ := filepath.Glob(configPath() + ".corrupt-*")
	if len(matches) != 1 {
		t.Errorf("expected one backup file, got %v", matches)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil || !strings.Contains(string(data), "not json") {
		t.Errorf("backup should preserve original bytes, got %q err=%v", data, err)
	}
}

func TestRetryFileMigrationAndConcurrency(t *testing.T) {
	dir := t.TempDir()
	legacy := legacyRetryFile(dir)
	if err := os.WriteFile(legacy, []byte("https://example.com/old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	migrateRetryFile(dir)
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Errorf("legacy retry file should be migrated")
	}
	if lines := readRetry(dir); len(lines) != 1 || lines[0] != "https://example.com/old" {
		t.Fatalf("migrated retry lines = %v", lines)
	}

	// 并发追加去重
	done := make(chan struct{})
	for i := 0; i < 20; i++ {
		go func(n int) {
			appendToRetry(dir, "https://example.com/"+string(rune('a'+n%5)))
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 20; i++ {
		<-done
	}
	if lines := readRetry(dir); len(lines) != 6 { // old + 5 unique
		t.Errorf("retry lines = %v (want 6)", lines)
	}
	for i := 0; i < 5; i++ {
		removeFromRetry(dir, "https://example.com/"+string(rune('a'+i)))
	}
	if lines := readRetry(dir); len(lines) != 1 {
		t.Errorf("after removal = %v", lines)
	}
}

func TestEffectiveFormatPresets(t *testing.T) {
	c := defaultConfig()
	c.FormatPreset = "quality"
	if got := c.effectiveFormat(); got != "bv*+ba/b" {
		t.Errorf("quality format = %q", got)
	}
	c.FormatPreset = "compat"
	if got := c.effectiveFormat(); !strings.Contains(got, "avc1") || !strings.Contains(got, "mp4a") {
		t.Errorf("compat format = %q", got)
	}
	c.FormatPreset = "custom"
	c.Format = "worst"
	if got := c.effectiveFormat(); got != "worst" {
		t.Errorf("custom format = %q", got)
	}
	c.Format = ""
	if got := c.effectiveFormat(); got == "" {
		t.Error("custom empty format should fall back")
	}
}

func TestResetToDefaultsKeepsLangAndDir(t *testing.T) {
	c := defaultConfig()
	c.Lang = "en"
	c.DownloadDir = "/tmp/keepme"
	c.Concurrency = 12
	c.resetToDefaults()
	if c.Concurrency != 4 {
		t.Errorf("concurrency should reset, got %d", c.Concurrency)
	}
	if c.Lang != "en" || c.DownloadDir != "/tmp/keepme" {
		t.Errorf("lang/dir should be preserved: %+v", c)
	}
}
