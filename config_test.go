package main

import "testing"

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

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}
