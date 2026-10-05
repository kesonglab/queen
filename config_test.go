package main

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
)

func TestIsXURL(t *testing.T) {
	cases := map[string]bool{
		"https://x.com/user/status/123":       true,
		"https://www.x.com/foo/status/1":      true,
		"https://twitter.com/user/status/123": true,
		"https://mobile.twitter.com/a/b":      true,
		"http://t.co/abc123":                  true,
		"https://youtu.be/abc":                false,
		"https://www.youtube.com/watch?v=abc": false,
		"https://example.com/x.com/fake":      false,
		"https://notx.com/status/1":           false,
	}
	for u, want := range cases {
		if got := isXURL(u); got != want {
			t.Errorf("isXURL(%q) = %v, want %v", u, got, want)
		}
	}
}

func TestYtdlpArgsTwitterAutoCookie(t *testing.T) {
	cfg := defaultConfig()
	cfg.CookieBrowser = ""

	old := detectBrowser
	detectBrowser = func() string { return "firefox" }
	defer func() { detectBrowser = old }()

	args := cfg.ytdlpArgs("https://x.com/user/status/123")
	if !containsArg(args, "--cookies-from-browser") || !containsArg(args, "firefox") {
		t.Fatalf("expected auto-detected cookies for X URL, got: %v", args)
	}

	args = cfg.ytdlpArgs("https://youtu.be/abc")
	if containsArg(args, "--cookies-from-browser") {
		t.Fatalf("expected no cookies for non-X URL, got: %v", args)
	}
}

func TestYtdlpArgsExplicitCookie(t *testing.T) {
	cfg := defaultConfig()
	cfg.CookieBrowser = "edge"

	old := detectBrowser
	detectBrowser = func() string { return "firefox" }
	defer func() { detectBrowser = old }()

	args := cfg.ytdlpArgs("https://youtu.be/abc")
	if !containsArg(args, "edge") {
		t.Fatalf("expected explicit browser cookies, got: %v", args)
	}
}

func TestYtdlpArgsRetryTimes(t *testing.T) {
	cfg := defaultConfig()
	cfg.RetryTimes = 3
	args := cfg.ytdlpArgs("https://youtu.be/abc")
	if !containsPair(args, "--retries", "3") {
		t.Fatalf("expected --retries 3, got: %v", args)
	}
	if !containsPair(args, "--fragment-retries", "3") {
		t.Fatalf("expected --fragment-retries 3, got: %v", args)
	}

	cfg.RetryTimes = 0
	args = cfg.ytdlpArgs("https://youtu.be/abc")
	if !containsPair(args, "--retries", "0") {
		t.Fatalf("expected --retries 0, got: %v", args)
	}
}

func TestClampConfig(t *testing.T) {
	cfg := defaultConfig()
	cfg.Concurrency = 100
	cfg.RetryTimes = -1
	cfg.Lang = "fr"
	clampConfig(cfg)
	if cfg.Concurrency != 16 {
		t.Errorf("Concurrency = %d, want 16", cfg.Concurrency)
	}
	if cfg.RetryTimes != 0 {
		t.Errorf("RetryTimes = %d, want 0", cfg.RetryTimes)
	}
	if cfg.Lang != "zh" {
		t.Errorf("Lang = %q, want zh", cfg.Lang)
	}
}

func TestRetryFileConcurrency(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			u := "https://example.com/" + strconv.Itoa(n%5)
			appendToRetry(dir, u)
		}(i)
	}
	wg.Wait()

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			removeFromRetry(dir, "https://example.com/"+strconv.Itoa(n))
		}(i)
	}
	wg.Wait()

	lines := readRetry(dir)
	if len(lines) != 0 {
		t.Fatalf("expected empty retry file after removals, got %v", lines)
	}
	if _, err := os.Stat(filepath.Join(dir, retryFileName)); !os.IsNotExist(err) {
		t.Fatalf("expected retry file removed, err=%v", err)
	}
}

func TestParseURLs(t *testing.T) {
	if got := parseURLs(""); len(got) != 0 {
		t.Fatalf("empty input: got %v", got)
	}
	if got := parseURLs("  \n\t "); len(got) != 0 {
		t.Fatalf("whitespace: got %v", got)
	}

	got := parseURLs("https://a.example/x\nhttps://a.example/x\nhttp://b.example/y ftp://no not-a-url")
	if len(got) != 2 {
		t.Fatalf("got %v, want 2 unique http(s) URLs", got)
	}
	if got[0] != "https://a.example/x" || got[1] != "http://b.example/y" {
		t.Fatalf("got %v", got)
	}
}

func TestCycleChoice(t *testing.T) {
	opts := []string{"a", "b", "c"}
	if g := cycleChoice("a", opts, 1); g != "b" {
		t.Fatalf("got %q", g)
	}
	if g := cycleChoice("a", opts, -1); g != "c" {
		t.Fatalf("got %q", g)
	}
	if g := cycleChoice("custom", opts, 1); g != "a" {
		t.Fatalf("custom next: got %q, want a", g)
	}
}

func TestToggleSettingByID(t *testing.T) {
	m := &model{cfg: defaultConfig()}
	items := m.settingsItems()
	idx := map[string]int{}
	for i, it := range items {
		idx[it.id] = i
	}
	for _, id := range []string{"format", "merge", "dir", "lang", "save"} {
		if _, ok := idx[id]; !ok {
			t.Fatalf("missing setting id %q", id)
		}
	}

	dir := m.cfg.DownloadDir
	m.toggleSetting(idx["dir"], 1)
	if m.cfg.DownloadDir != dir {
		t.Fatal("dir row should ignore left/right")
	}

	oldFmt := m.cfg.Format
	m.toggleSetting(idx["format"], 1)
	if m.cfg.Format == oldFmt {
		t.Fatal("format should cycle")
	}
	if !containsPair(m.cfg.ytdlpArgs("https://youtu.be/abc"), "-f", m.cfg.Format) {
		t.Fatalf("format not passed to yt-dlp args")
	}

	oldMerge := m.cfg.MergeFormat
	m.toggleSetting(idx["merge"], 1)
	if m.cfg.MergeFormat == oldMerge {
		t.Fatal("merge should cycle")
	}
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func containsPair(args []string, flag, val string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && args[i+1] == val {
			return true
		}
	}
	return false
}
