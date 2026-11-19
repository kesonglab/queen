package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// ---------- yt-dlp 参数 ----------

func argIndex(args []string, flag string) int {
	for i, a := range args {
		if a == flag {
			return i
		}
	}
	return -1
}

func hasFlag(args []string, flag string) bool { return argIndex(args, flag) >= 0 }

func flagValue(args []string, flag string) (string, bool) {
	i := argIndex(args, flag)
	if i < 0 || i+1 >= len(args) {
		return "", false
	}
	return args[i+1], true
}

func requireFlag(t *testing.T, args []string, flag string) {
	t.Helper()
	if !hasFlag(args, flag) {
		t.Errorf("missing flag %q in %v", flag, args)
	}
}

func requirePair(t *testing.T, args []string, flag, want string) {
	t.Helper()
	got, ok := flagValue(args, flag)
	if !ok {
		t.Fatalf("missing flag %q in %v", flag, args)
	}
	if got != want {
		t.Errorf("flag %q = %q, want %q", flag, got, want)
	}
}

func TestBuildArgsDefaults(t *testing.T) {
	cfg := defaultConfig()
	cfg.DownloadDir = "/tmp/dl"
	eff := cfg.Effective(defaultSiteDB(), "https://example.com/video")
	args := cfg.BuildArgs("https://example.com/video", eff, 8)

	requireFlag(t, args, "--ignore-config")
	requirePair(t, args, "--color", "never")
	requireFlag(t, args, "--newline")
	requireFlag(t, args, "--progress")
	requirePair(t, args, "-f", "bv*+ba/b")
	requirePair(t, args, "--merge-output-format", "mp4")
	requirePair(t, args, "-o", "%(title)s [%(id)s].%(ext)s")
	requirePair(t, args, "-P", "/tmp/dl")
	requireFlag(t, args, "--no-overwrites")
	requirePair(t, args, "--trim-filenames", "200")
	requireFlag(t, args, "--no-playlist")
	requirePair(t, args, "--retries", "2")
	requirePair(t, args, "--fragment-retries", "2")
	requirePair(t, args, "--socket-timeout", "30")
	requirePair(t, args, "--retry-sleep", "3")
	requirePair(t, args, "--throttled-rate", "100K")
	requirePair(t, args, "--concurrent-fragments", "8")
	if got := args[len(args)-1]; got != "https://example.com/video" {
		t.Errorf("url should be last, got %q", got)
	}

	// 这些是 v1.4.0 的反优化项，必须不再出现
	if hasFlag(args, "--http-chunk-size") {
		t.Error("--http-chunk-size must not be passed (experimental, breaks range requests)")
	}
	if hasFlag(args, "--buffer-size") {
		t.Error("--buffer-size must not be passed (--resize-buffer already default)")
	}

	// 进度模板必须存在两处（download / postprocess）
	var templates []string
	for i, a := range args {
		if a == "--progress-template" && i+1 < len(args) {
			templates = append(templates, args[i+1])
		}
	}
	if len(templates) != 2 {
		t.Fatalf("expected 2 progress templates, got %d (%v)", len(templates), templates)
	}
	if !strings.Contains(templates[0], "QDL") || !strings.Contains(templates[0], "progress.downloaded_bytes") {
		t.Errorf("download template = %q", templates[0])
	}
	if !strings.Contains(templates[1], "QPP") || !strings.Contains(templates[1], "postprocessor") {
		t.Errorf("postprocess template = %q", templates[1])
	}
}

func TestBuildArgsCompatPreset(t *testing.T) {
	cfg := defaultConfig()
	cfg.FormatPreset = "compat"
	eff := cfg.Effective(defaultSiteDB(), "https://youtu.be/x")
	args := cfg.BuildArgs("https://youtu.be/x", eff, 16)
	format, _ := flagValue(args, "-f")
	if !strings.Contains(format, "avc1") || !strings.Contains(format, "mp4a") {
		t.Errorf("compat format = %q", format)
	}
}

func TestBuildArgsAudioOnly(t *testing.T) {
	cfg := defaultConfig()
	cfg.AudioOnly = true
	cfg.Embed = true
	eff := cfg.Effective(defaultSiteDB(), "https://example.com/a")
	args := cfg.BuildArgs("https://example.com/a", eff, 4)
	requireFlag(t, args, "-x")
	requirePair(t, args, "--audio-format", "m4a")
	requirePair(t, args, "--audio-quality", "0")
	if hasFlag(args, "--merge-output-format") {
		t.Error("audio-only should not pass --merge-output-format")
	}
	requireFlag(t, args, "--embed-metadata")
}

func TestBuildArgsSubtitles(t *testing.T) {
	cfg := defaultConfig()
	cfg.Subs = true
	cfg.AutoSubs = true
	cfg.SubLangs = "zh.*,en.*"
	eff := cfg.Effective(defaultSiteDB(), "https://example.com/a")
	args := cfg.BuildArgs("https://example.com/a", eff, 4)
	requireFlag(t, args, "--write-subs")
	requireFlag(t, args, "--write-auto-subs")
	requireFlag(t, args, "--embed-subs")
	requirePair(t, args, "--sub-langs", "zh.*,en.*")
}

func TestBuildArgsPlaylistAndArchive(t *testing.T) {
	cfg := defaultConfig()
	cfg.Playlist = true
	cfg.PlaylistFolder = true
	cfg.Archive = true
	cfg.DownloadDir = "/tmp/dl"
	eff := cfg.Effective(defaultSiteDB(), "https://example.com/playlist")
	args := cfg.BuildArgs("https://example.com/playlist", eff, 4)
	requireFlag(t, args, "--yes-playlist")
	out, _ := flagValue(args, "-o")
	if !strings.Contains(out, "playlist_index") || !strings.Contains(out, "playlist_title") {
		t.Errorf("playlist output template = %q", out)
	}
	requirePair(t, args, "--download-archive", filepath.Join("/tmp/dl", archiveFileName))
}

func TestBuildArgsAuthAndSite(t *testing.T) {
	cfg := defaultConfig()
	cfg.Impersonate = "chrome-136"
	cfg.ExtractorArgs = "youtube:player_client=default"
	cfg.Proxy = "http://127.0.0.1:7890"
	cfg.UserAgent = "UA/1.0"
	cfg.Referer = "https://ref.example/"
	cfg.LimitRate = "2M"
	eff := cfg.Effective(defaultSiteDB(), "https://example.com/a")
	args := cfg.BuildArgs("https://example.com/a", eff, 4)
	requirePair(t, args, "--impersonate", "chrome-136")
	requirePair(t, args, "--extractor-args", "youtube:player_client=default")
	requirePair(t, args, "--proxy", "http://127.0.0.1:7890")
	requirePair(t, args, "--user-agent", "UA/1.0")
	requirePair(t, args, "--referer", "https://ref.example/")
	requirePair(t, args, "--limit-rate", "2M")
}

func TestBuildArgsCookiesPrecedence(t *testing.T) {
	cfg := defaultConfig()
	cfg.CookieBrowser = "chrome"
	cfg.CookiesFile = "/tmp/cookies.txt"
	eff := cfg.Effective(defaultSiteDB(), "https://example.com/a")
	args := cfg.BuildArgs("https://example.com/a", eff, 4)
	requirePair(t, args, "--cookies", "/tmp/cookies.txt")
	if hasFlag(args, "--cookies-from-browser") {
		t.Error("cookies file should take precedence over browser")
	}

	cfg.CookiesFile = ""
	eff = cfg.Effective(defaultSiteDB(), "https://example.com/a")
	args = cfg.BuildArgs("https://example.com/a", eff, 4)
	requirePair(t, args, "--cookies-from-browser", "chrome")
}

func TestOutputTemplate(t *testing.T) {
	if got := outputTemplate(defaultConfig()); got != "%(title)s [%(id)s].%(ext)s" {
		t.Errorf("default template = %q", got)
	}
	cfg := defaultConfig()
	cfg.Playlist = true
	cfg.PlaylistFolder = false
	if got := outputTemplate(cfg); got != "%(title)s [%(id)s].%(ext)s" {
		t.Errorf("playlist-without-folder template = %q", got)
	}
}

// ---------- 版本与升级 ----------

func TestParseYtdlpVersion(t *testing.T) {
	cases := map[string]bool{
		"2026.10.26":        true,
		"2026.10.26.232746": true,
		"v2026.10.26":       true,
		"2026.1.5":          true,
		"not-a-version":     false,
		"2026":              false,
		"2026.10":           false,
		"":                  false,
	}
	for in, ok := range cases {
		_, got := parseYtdlpVersion(in)
		if got != ok {
			t.Errorf("parseYtdlpVersion(%q) ok=%v want %v", in, got, ok)
		}
	}
	d, ok := parseYtdlpVersion("2026.10.26")
	if !ok || d.Year() != 2026 || d.Month() != 10 || d.Day() != 26 {
		t.Fatalf("parsed date = %v ok=%v", d, ok)
	}
}

func TestUpgradeCommand(t *testing.T) {
	brew := YtdlpInfo{Path: "/opt/homebrew/bin/yt-dlp", BrewManaged: true}
	if cmd := brew.UpgradeCommand(); cmd[0] != "brew" {
		t.Errorf("brew upgrade = %v", cmd)
	}
	if !strings.Contains(brew.UpgradeLabel(), "brew") {
		t.Errorf("brew label = %q", brew.UpgradeLabel())
	}
	plain := YtdlpInfo{Path: "/usr/local/bin/yt-dlp"}
	if cmd := plain.UpgradeCommand(); cmd[0] != "yt-dlp" || cmd[1] != "-U" {
		t.Errorf("plain upgrade = %v", cmd)
	}
}

func TestIsBrewManaged(t *testing.T) {
	cases := map[string]bool{
		"/opt/homebrew/bin/yt-dlp":                          true,
		"/usr/local/opt/yt-dlp/bin/yt-dlp":                  true,
		"/opt/homebrew/Cellar/yt-dlp/2026.10.26/bin/yt-dlp": true,
		"/usr/bin/yt-dlp":                                   false,
		"":                                                  false,
	}
	for in, want := range cases {
		if got := isBrewManaged(in); got != want {
			t.Errorf("isBrewManaged(%q) = %v", in, got)
		}
	}
}

// ---------- 站点档案 ----------

func TestSiteDBLookup(t *testing.T) {
	db := defaultSiteDB()
	if p := db.Lookup("www.youtube.com"); p.Name != "YouTube" {
		t.Errorf("www.youtube.com -> %+v", p)
	}
	if p := db.Lookup("m.youtube.com"); p.Name != "YouTube" {
		t.Errorf("m.youtube.com -> %+v", p)
	}
	if p := db.Lookup("youtu.be"); p.Name != "YouTube" {
		t.Errorf("youtu.be -> %+v", p)
	}
	if p := db.Lookup("notyoutube.com"); p.Name != "" {
		t.Errorf("notyoutube.com should not match, got %+v", p)
	}
	if p := db.Lookup(""); p.Name != "" {
		t.Errorf("empty host should not match")
	}
	if p := db.Lookup("rule34video.com"); !p.Privacy || p.Impersonate != "chrome" {
		t.Errorf("rule34video profile = %+v", p)
	}
	if p := db.Lookup("WWW.BILIBILI.COM"); p.Name != "Bilibili" {
		t.Errorf("case-insensitive lookup failed: %+v", p)
	}
}

func TestSiteDBUserOverride(t *testing.T) {
	db := &SiteDB{
		builtin: []SiteProfile{
			{Name: "Example", Match: []string{"example.com"}, Fragments: 8, Cookies: "auto"},
			{Name: "Sub", Match: []string{"sub.example.com"}, Fragments: 4},
		},
		user: []SiteProfile{
			{Match: []string{"example.com"}, Fragments: 2, Impersonate: "safari"},
		},
	}
	p := db.Lookup("www.example.com")
	if p.Name != "Example" || p.Fragments != 2 || p.Impersonate != "safari" {
		t.Fatalf("merge failed: %+v", p)
	}
	// 最长后缀决定内置档案；用户覆盖仍然生效
	p = db.Lookup("sub.example.com")
	if p.Name != "Sub" {
		t.Errorf("longest builtin suffix should win: %+v", p)
	}
	if p.Fragments != 2 {
		t.Errorf("user override fragments = %d, want 2", p.Fragments)
	}
	if p.Impersonate != "safari" {
		t.Errorf("user override should still apply: %+v", p)
	}
}

func TestParseSiteProfilesFormats(t *testing.T) {
	arr := []byte(`[{"name":"A","match":["a.com"]}]`)
	if ps, err := parseSiteProfiles(arr); err != nil || len(ps) != 1 || ps[0].Name != "A" {
		t.Fatalf("array parse: %v %v", ps, err)
	}
	obj := []byte(`{"profiles":[{"name":"B","match":["b.com"]}]}`)
	if ps, err := parseSiteProfiles(obj); err != nil || len(ps) != 1 || ps[0].Name != "B" {
		t.Fatalf("object parse: %v %v", ps, err)
	}
	if _, err := parseSiteProfiles([]byte(`{bad`)); err == nil {
		t.Fatal("invalid json should error")
	}
}

func TestEffectiveSite(t *testing.T) {
	cfg := defaultConfig()
	old := detectBrowser
	detectBrowser = func() string { return "firefox" }
	defer func() { detectBrowser = old }()

	// auto → 自动探测
	eff := cfg.Effective(defaultSiteDB(), "https://x.com/user/status/1")
	if eff.CookiesBrowser != "firefox" {
		t.Errorf("x.com auto cookies = %q", eff.CookiesBrowser)
	}
	// 非档案站点继承全局
	eff = cfg.Effective(defaultSiteDB(), "https://example.com/a")
	if eff.CookiesBrowser != "" {
		t.Errorf("unknown site cookies = %q", eff.CookiesBrowser)
	}
	// 全局显式浏览器也用于 auto 档案
	cfg.CookieBrowser = "brave"
	eff = cfg.Effective(defaultSiteDB(), "https://x.com/a")
	if eff.CookiesBrowser != "brave" {
		t.Errorf("explicit browser should win: %q", eff.CookiesBrowser)
	}
	// rule34video 的隐私 + 不嵌封面 + impersonate
	eff = cfg.Effective(defaultSiteDB(), "https://rule34video.com/video/1")
	if !eff.Privacy || eff.Embed || eff.Impersonate != "chrome" {
		t.Errorf("rule34video effective = %+v", eff)
	}
	// profile off 关闭 cookie
	db := &SiteDB{builtin: []SiteProfile{{Name: "Off", Match: []string{"off.test"}, Cookies: "off"}}}
	eff = cfg.Effective(db, "https://off.test/a")
	if eff.CookiesBrowser != "" || eff.CookiesFile != "" {
		t.Errorf("cookies off should clear both: %+v", eff)
	}
}

// ---------- i18n ----------

func TestI18nParity(t *testing.T) {
	zh := translations["zh"]
	en := translations["en"]
	if len(zh) == 0 || len(en) == 0 {
		t.Fatal("translation tables empty")
	}
	for k := range zh {
		if _, ok := en[k]; !ok {
			t.Errorf("key %q missing in en", k)
		}
	}
	for k := range en {
		if _, ok := zh[k]; !ok {
			t.Errorf("key %q missing in zh", k)
		}
	}
}

func TestI18nDynamicKeysExist(t *testing.T) {
	// 错误类型 / 阶段 / 状态 / 主题 / 预设的 key 必须成对存在
	var kinds []string
	for _, k := range []ErrorKind{
		KindUnknown, KindAuth, KindMembership, KindDRM, KindGeo, KindCloudflare,
		KindRateLimit, KindUnsupported, KindNetwork, KindNotFound, KindStorage, KindExtractor,
	} {
		kinds = append(kinds, k.I18nKey(), "errhint_"+string(k))
	}
	for st := StatusPending; st <= StatusAborted; st++ {
		kinds = append(kinds, st.I18nKey())
	}
	kinds = append(kinds,
		phaseExtracting, phaseDownloading, phaseMerging, phaseConverting,
		phaseEmbedding, phaseMetadata, phaseFixup, phasePostprocessing,
		"theme_auto", "theme_dark", "theme_light",
		"preset_quality", "preset_compat", "preset_custom",
	)
	for _, key := range kinds {
		if _, ok := translations["zh"][key]; !ok {
			t.Errorf("zh missing dynamic key %q", key)
		}
		if _, ok := translations["en"][key]; !ok {
			t.Errorf("en missing dynamic key %q", key)
		}
	}
}

func TestTrFallbackAndArgs(t *testing.T) {
	zhCfg := &Config{Lang: "zh"}
	enCfg := &Config{Lang: "en"}
	if got := tr(zhCfg, "menu_paste"); got != "粘贴链接下载" {
		t.Errorf("zh menu_paste = %q", got)
	}
	if got := tr(enCfg, "menu_paste"); got != "Paste links to download" {
		t.Errorf("en menu_paste = %q", got)
	}
	if got := tr(zhCfg, "no_such_key"); got != "no_such_key" {
		t.Errorf("fallback = %q", got)
	}
	if got := tr(zhCfg, "toast_clip_ok", 3); !strings.Contains(got, "3") {
		t.Errorf("format args not applied: %q", got)
	}
	if got := tr(nil, "menu_paste"); got != "粘贴链接下载" {
		t.Errorf("nil cfg should default zh: %q", got)
	}
}

// TestSourceVersionMatchesChangelog 保证源码内置版本与 CHANGELOG 最新版本一致
// （防止发布时忘记同步版本号）。
func TestSourceVersionMatchesChangelog(t *testing.T) {
	old := version
	defer func() { version = old }()

	if !strings.HasPrefix(version, "v") {
		t.Errorf("source version %q should carry the v prefix", version)
	}
	re := regexp.MustCompile(`(?m)^## (v[0-9]+\.[0-9]+\.[0-9]+)`)
	m := re.FindStringSubmatch(changelogMD)
	if m == nil {
		t.Fatal("no version heading found in CHANGELOG.md")
	}
	if m[1] != version {
		t.Errorf("source version %q != latest changelog version %q", version, m[1])
	}
	if got := versionLabel(); got != version {
		t.Errorf("versionLabel() = %q, want %q", got, version)
	}
}

func TestVersionLabel(t *testing.T) {
	old := version
	defer func() { version = old }()
	version = "dev"
	if got := versionLabel(); got != "dev" {
		t.Errorf("dev label = %q", got)
	}
	version = "1.6.0"
	if got := versionLabel(); got != "v1.6.0" {
		t.Errorf("semver label = %q", got)
	}
	version = "v1.6.0"
	if got := versionLabel(); got != "v1.6.0" {
		t.Errorf("tag label = %q", got)
	}
}

// TestMain 强制无色输出，保证视图宽度断言稳定（等价于 NO_COLOR 语义）。
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}
