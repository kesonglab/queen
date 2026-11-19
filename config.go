package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Config 是 ~/.config/queen/config.json 的映射。字段名保持向后兼容：
// v1.4.0 的旧字段含义不变，新增字段在缺失时由 defaultConfig 提供默认值。
type Config struct {
	ConfigVersion int    `json:"config_version"`
	DownloadDir   string `json:"download_dir"`
	Lang          string `json:"lang"`
	Theme         string `json:"theme"` // auto | dark | light

	// 内容
	Playlist       bool   `json:"playlist"`        // 下载整个播放列表
	PlaylistFolder bool   `json:"playlist_folder"` // 播放列表存放到子目录并按序号命名
	AudioOnly      bool   `json:"audio_only"`
	AudioFormat    string `json:"audio_format"` // m4a | mp3 | opus | flac | best
	Subs           bool   `json:"subs"`
	AutoSubs       bool   `json:"auto_subs"`
	SubLangs       string `json:"sub_langs"`
	Embed          bool   `json:"embed"` // 嵌入元数据/封面（默认关，见性能优化）

	// 画质/容器
	FormatPreset string `json:"format_preset"` // quality | compat | custom
	Format       string `json:"format"`        // 仅 custom 使用
	MergeFormat  string `json:"merge_format"`  // mp4 | mkv | webm

	// 网络/性能
	Concurrency      int    `json:"concurrency"`       // 并行任务数
	Fragments        int    `json:"fragments"`         // 每任务分片并发（-N）
	ConnectionBudget int    `json:"connection_budget"` // 全局连接预算
	SameSiteLimit    int    `json:"same_site_limit"`   // 同一站点并行任务上限
	RetryTimes       int    `json:"retry_times"`
	RetrySleep       int    `json:"retry_sleep"`    // 秒
	ThrottledRate    string `json:"throttled_rate"` // 例如 100K，空串关闭
	LimitRate        string `json:"limit_rate"`     // 限速，例如 2M，空串关闭

	// 站点/认证
	CookieBrowser string `json:"cookie_browser"` // 空 = 自动
	CookiesFile   string `json:"cookies_file"`   // cookies.txt 路径（可选）
	Impersonate   string `json:"impersonate"`    // 空 = 关闭；chrome / chrome-136 / safari ...
	ExtractorArgs string `json:"extractor_args"`
	Proxy         string `json:"proxy"`
	UserAgent     string `json:"user_agent"`
	Referer       string `json:"referer"`

	// 其它
	TrimFilenames     int  `json:"trim_filenames"`
	IgnoreYtdlpConfig bool `json:"ignore_ytdlp_config"`
	Archive           bool `json:"download_archive"`
	PrivacyMode       bool `json:"privacy_mode"` // 通知/日志不出现 URL 与标题
	KeepFailureLogs   bool `json:"keep_failure_logs"`
	UpdateCheck       bool `json:"update_check"`
}

const (
	configVersion    = 2
	configDirName    = ".config/queen"
	legacyConfigDir  = ".config/videodl"
	configFileName   = "config.json"
	sitesFileName    = "sites.json"
	stateDirName     = ".local/state/queen"
	logsDirName      = "logs"
	retryFileName    = "failed_links.txt"
	legacyRetryName  = "视频失败链接-failed_links.txt"
	archiveFileName  = ".queen-archive.txt"
	defaultDownloads = "Downloads"

	maxLogFiles = 200
)

// 设置页可循环的选项集合（顺序即 ←→ 的顺序）。
var (
	formatPresetOptions  = []string{"quality", "compat", "custom"}
	mergeFormatOptions   = []string{"mp4", "mkv", "webm"}
	audioFormatOptions   = []string{"m4a", "mp3", "opus", "flac", "best"}
	themeOptions         = []string{"auto", "dark", "light"}
	cookieBrowserOptions = []string{"", "chrome", "chromium", "brave", "firefox", "safari", "edge"}
	// 与 `yt-dlp --list-impersonate-targets`（curl_cffi）提供的客户端一致
	impersonateOptions = []string{"", "chrome", "chrome-136", "chrome-133", "safari", "safari-18.4", "firefox", "edge"}
	fragmentOptions    = []int{4, 8, 16}
)

// 画质预设对应的 yt-dlp 格式选择器。
func presetFormat(preset string) string {
	switch preset {
	case "compat":
		// 尽量选择 h264 + aac，macOS/QuickTime 兼容性最好
		return "bv*[vcodec^=avc1]+ba[acodec^=mp4a]/b[ext=mp4]/bv*+ba/b"
	case "custom":
		return ""
	default: // quality
		return "bv*+ba/b"
	}
}

func defaultConfig() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		ConfigVersion:     configVersion,
		DownloadDir:       filepath.Join(home, defaultDownloads),
		Lang:              "zh",
		Theme:             "auto",
		Playlist:          false,
		PlaylistFolder:    true,
		AudioOnly:         false,
		AudioFormat:       "m4a",
		Subs:              false,
		AutoSubs:          false,
		SubLangs:          "zh.*,en.*",
		Embed:             false,
		FormatPreset:      "quality",
		Format:            "bv*+ba/b",
		MergeFormat:       "mp4",
		Concurrency:       4,
		Fragments:         8,
		ConnectionBudget:  32,
		SameSiteLimit:     2,
		RetryTimes:        2,
		RetrySleep:        3,
		ThrottledRate:     "100K",
		LimitRate:         "",
		CookieBrowser:     "",
		CookiesFile:       "",
		Impersonate:       "",
		ExtractorArgs:     "",
		Proxy:             "",
		UserAgent:         "",
		Referer:           "",
		TrimFilenames:     200,
		IgnoreYtdlpConfig: true,
		Archive:           false,
		PrivacyMode:       false,
		KeepFailureLogs:   true,
		UpdateCheck:       true,
	}
}

// ---------- 路径 ----------

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil || h == "" {
		return "/tmp"
	}
	return h
}

func configPath() string {
	return filepath.Join(homeDir(), configDirName, configFileName)
}

func configDir() string { return filepath.Join(homeDir(), configDirName) }

func legacyConfigPath() string {
	return filepath.Join(homeDir(), legacyConfigDir, configFileName)
}

func sitesPath() string { return filepath.Join(configDir(), sitesFileName) }

func logsDir() string { return filepath.Join(homeDir(), stateDirName, logsDirName) }

func expandHome(p string) string {
	p = strings.TrimSpace(p)
	if p == "~" {
		return homeDir()
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(homeDir(), p[2:])
	}
	return p
}

// retryFile 返回下载目录下的失败链接文件；空目录回退到默认下载目录。
func retryFile(downloadDir string) string {
	downloadDir = expandHome(downloadDir)
	if downloadDir == "" {
		downloadDir = filepath.Join(homeDir(), defaultDownloads)
	}
	return filepath.Join(downloadDir, retryFileName)
}

func legacyRetryFile(downloadDir string) string {
	downloadDir = expandHome(downloadDir)
	if downloadDir == "" {
		downloadDir = filepath.Join(homeDir(), defaultDownloads)
	}
	return filepath.Join(downloadDir, legacyRetryName)
}

func archivePath(downloadDir string) string {
	downloadDir = expandHome(downloadDir)
	if downloadDir == "" {
		downloadDir = filepath.Join(homeDir(), defaultDownloads)
	}
	return filepath.Join(downloadDir, archiveFileName)
}

// ---------- 读取 / 保存 ----------

// loadNotice 是启动时需要展示给用户的提示（已 i18n 化，main 里再翻译）。
type loadNotice struct {
	key  string
	args []any
}

// loadConfig 读取配置。返回的 notices 会在 TUI 启动后以 toast 形式提示用户。
func loadConfig() (*Config, []loadNotice) {
	cfg := defaultConfig()
	var notices []loadNotice

	data, err := os.ReadFile(configPath())
	if err != nil {
		// 从旧路径 ~/.config/videodl 迁移
		if legacy, legacyErr := os.ReadFile(legacyConfigPath()); legacyErr == nil {
			data = legacy
			if json.Unmarshal(data, cfg) == nil {
				cfg.normalize()
				if err := cfg.save(); err == nil {
					notices = append(notices, loadNotice{key: "notice_config_migrated"})
				}
			} else {
				cfg = defaultConfig()
				notices = append(notices, loadNotice{key: "notice_config_corrupt_backup"})
			}
			return cfg, notices
		}
		return cfg, notices
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		// 损坏配置：备份后回默认值，并明确告知用户，避免静默覆盖。
		backup := configPath() + ".corrupt-" + time.Now().Format("20060102-150405")
		_ = os.WriteFile(backup, data, 0o600)
		cfg = defaultConfig()
		notices = append(notices, loadNotice{key: "notice_config_corrupt_backup", args: []any{backup}})
		return cfg, notices
	}

	cfg.DownloadDir = expandHome(cfg.DownloadDir)
	cfg.normalize()
	return cfg, notices
}

// save 原子写入配置（临时文件 + rename），返回错误供 UI 提示。
func (c *Config) save() error {
	c.normalize()
	dir := configDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("%s: %w", dir, err)
	}
	c.ConfigVersion = configVersion
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, configFileName+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, configPath())
}

// normalize 把配置值限制在合法范围，并补齐空值。
func (c *Config) normalize() {
	c.ConfigVersion = configVersion
	if strings.TrimSpace(c.DownloadDir) == "" {
		c.DownloadDir = expandHome("~/Downloads")
	}
	c.DownloadDir = expandHome(c.DownloadDir)
	if c.Lang != "zh" && c.Lang != "en" {
		c.Lang = "zh"
	}
	if !containsString(themeOptions, c.Theme) {
		c.Theme = "auto"
	}
	c.Concurrency = clampInt(c.Concurrency, 1, 16)
	c.Fragments = clampInt(c.Fragments, 1, 16)
	c.ConnectionBudget = clampInt(c.ConnectionBudget, 4, 64)
	c.SameSiteLimit = clampInt(c.SameSiteLimit, 1, 8)
	c.RetryTimes = clampInt(c.RetryTimes, 0, 10)
	c.RetrySleep = clampInt(c.RetrySleep, 0, 60)
	c.TrimFilenames = clampInt(c.TrimFilenames, 0, 240)
	if c.TrimFilenames > 0 && c.TrimFilenames < 60 {
		c.TrimFilenames = 60
	}
	if !containsString(formatPresetOptions, c.FormatPreset) {
		c.FormatPreset = "quality"
	}
	if strings.TrimSpace(c.Format) == "" {
		c.Format = "bv*+ba/b"
	}
	if !containsString(mergeFormatOptions, c.MergeFormat) {
		c.MergeFormat = "mp4"
	}
	if !containsString(audioFormatOptions, c.AudioFormat) {
		c.AudioFormat = "m4a"
	}
	if strings.TrimSpace(c.SubLangs) == "" {
		c.SubLangs = "zh.*,en.*"
	}
	if c.CookieBrowser != "" && !containsString(cookieBrowserOptions, c.CookieBrowser) {
		c.CookieBrowser = ""
	}
	c.CookiesFile = expandHome(c.CookiesFile)
	c.ThrottledRate = strings.TrimSpace(c.ThrottledRate)
	c.LimitRate = strings.TrimSpace(c.LimitRate)
	c.Proxy = strings.TrimSpace(c.Proxy)
	c.UserAgent = strings.TrimSpace(c.UserAgent)
	c.Referer = strings.TrimSpace(c.Referer)
	c.ExtractorArgs = strings.TrimSpace(c.ExtractorArgs)
	c.Impersonate = strings.TrimSpace(c.Impersonate)
}

// effectiveFormat 返回当前应该传给 yt-dlp 的 -f 值。
func (c *Config) effectiveFormat() string {
	if c.FormatPreset == "custom" {
		if f := strings.TrimSpace(c.Format); f != "" {
			return f
		}
		return "bv*+ba/b"
	}
	if f := presetFormat(c.FormatPreset); f != "" {
		return f
	}
	return "bv*+ba/b"
}

// diffFromDefault 用于设置页「恢复默认」。
func (c *Config) resetToDefaults() {
	def := defaultConfig()
	lang := c.Lang // 语言不重置，避免用户瞬间看不懂界面
	dir := c.DownloadDir
	*c = *def
	c.Lang = lang
	if strings.TrimSpace(dir) != "" && dir != def.DownloadDir {
		c.DownloadDir = dir
	}
}

// ---------- 失败链接文件 ----------

var retryMu sync.Mutex

func appendToRetry(downloadDir, url string) {
	retryMu.Lock()
	defer retryMu.Unlock()

	f := retryFile(downloadDir)
	if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
		return
	}
	lines, _ := readLines(f)
	for _, l := range lines {
		if l == url {
			return
		}
	}
	file, err := os.OpenFile(f, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = fmt.Fprintln(file, url)
}

func removeFromRetry(downloadDir, url string) {
	retryMu.Lock()
	defer retryMu.Unlock()

	f := retryFile(downloadDir)
	lines, err := readLines(f)
	if err != nil {
		return
	}
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if l != url {
			out = append(out, l)
		}
	}
	if len(out) == 0 {
		_ = os.Remove(f)
		return
	}
	_ = os.WriteFile(f, []byte(strings.Join(out, "\n")+"\n"), 0o600)
}

func readRetry(downloadDir string) []string {
	retryMu.Lock()
	defer retryMu.Unlock()
	lines, _ := readLines(retryFile(downloadDir))
	return lines
}

// migrateRetryFile 把 v1.4.0 的中英混杂文件名迁移为 failed_links.txt。
func migrateRetryFile(downloadDir string) {
	retryMu.Lock()
	defer retryMu.Unlock()
	oldName := legacyRetryFile(downloadDir)
	if _, err := os.Stat(oldName); err != nil {
		return
	}
	newName := retryFile(downloadDir)
	if _, err := os.Stat(newName); err == nil {
		return
	}
	_ = os.Rename(oldName, newName)
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		s := strings.TrimSpace(sc.Text())
		if s != "" {
			lines = append(lines, s)
		}
	}
	return lines, sc.Err()
}

// ---------- 小工具 ----------

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// cycleChoice 在当前值与选项列表之间循环。
// 当前值不在列表中时：向后取第一项，向前取最后一项。
func cycleChoice(cur string, opts []string, delta int) string {
	if len(opts) == 0 {
		return cur
	}
	idx, found := 0, false
	for i, o := range opts {
		if o == cur {
			idx, found = i, true
			break
		}
	}
	n := len(opts)
	if !found {
		if delta < 0 {
			return opts[n-1]
		}
		return opts[0]
	}
	idx = ((idx+delta)%n + n) % n
	return opts[idx]
}

func cycleInt(cur int, opts []int, delta int) int {
	if len(opts) == 0 {
		return cur
	}
	idx := 0
	for i, o := range opts {
		if o == cur {
			idx = i
			break
		}
	}
	n := len(opts)
	idx = ((idx+delta)%n + n) % n
	return opts[idx]
}
