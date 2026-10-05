package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// Config 程序配置
type Config struct {
	DownloadDir   string `json:"download_dir"`
	CookieBrowser string `json:"cookie_browser"`
	Playlist      bool   `json:"playlist"`
	AudioOnly     bool   `json:"audio_only"`
	Subs          bool   `json:"subs"`
	Embed         bool   `json:"embed"`
	RetryTimes    int    `json:"retry_times"`
	Concurrency   int    `json:"concurrency"`
	Format        string `json:"format"`
	MergeFormat   string `json:"merge_format"`
	Lang          string `json:"lang"`
}

const (
	configDir       = ".config/queen"
	legacyConfigDir = ".config/videodl"
	configFile      = "config.json"
	defaultDir      = "Downloads"
	retryFileName   = "视频失败链接-failed_links.txt"
)

func defaultConfig() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		DownloadDir:   filepath.Join(home, defaultDir),
		CookieBrowser: "", // 空 = 自动检测
		Playlist:      true,
		AudioOnly:     false,
		Subs:          false,
		Embed:         true,
		RetryTimes:    2,
		Concurrency:   4,
		Format:        "bestvideo*+bestaudio/best",
		MergeFormat:   "mp4",
		Lang:          "zh",
	}
}

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, configDir, configFile)
}

func legacyConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, legacyConfigDir, configFile)
}

// clampConfig 将配置值限制在合法范围内
func clampConfig(c *Config) {
	if c.Concurrency < 1 {
		c.Concurrency = 1
	}
	if c.Concurrency > 16 {
		c.Concurrency = 16
	}
	if c.RetryTimes < 0 {
		c.RetryTimes = 0
	}
	if c.RetryTimes > 5 {
		c.RetryTimes = 5
	}
	if c.Format == "" {
		c.Format = "bestvideo*+bestaudio/best"
	}
	if c.MergeFormat == "" {
		c.MergeFormat = "mp4"
	}
	if c.Lang != "zh" && c.Lang != "en" {
		c.Lang = "zh"
	}
}

func loadConfig() *Config {
	cfg := defaultConfig()
	data, err := os.ReadFile(configPath())
	migrated := false
	if err != nil {
		// 从旧路径 ~/.config/videodl 迁移
		data, err = os.ReadFile(legacyConfigPath())
		if err != nil {
			return cfg
		}
		migrated = true
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return defaultConfig()
	}
	cfg.DownloadDir = expandHome(cfg.DownloadDir)
	clampConfig(cfg)
	if migrated {
		cfg.save()
	}
	return cfg
}

func (c *Config) save() {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, configDir)
	_ = os.MkdirAll(dir, 0o755)
	clampConfig(c)
	data, _ := json.MarshalIndent(c, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, configFile), data, 0o600)
}

func expandHome(p string) string {
	if p == "~" {
		h, _ := os.UserHomeDir()
		return h
	}
	if strings.HasPrefix(p, "~/") {
		h, _ := os.UserHomeDir()
		return filepath.Join(h, p[2:])
	}
	return p
}

// 失败链接记录文件（放在下载目录下）
func retryFile(downloadDir string) string {
	if downloadDir == "" {
		home, _ := os.UserHomeDir()
		downloadDir = filepath.Join(home, defaultDir)
	}
	return filepath.Join(downloadDir, retryFileName)
}

var retryMu sync.Mutex

func appendToRetry(downloadDir, url string) {
	retryMu.Lock()
	defer retryMu.Unlock()

	f := retryFile(downloadDir)
	_ = os.MkdirAll(filepath.Dir(f), 0o755)
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
	fmt.Fprintln(file, url)
}

func removeFromRetry(downloadDir, url string) {
	retryMu.Lock()
	defer retryMu.Unlock()

	f := retryFile(downloadDir)
	lines, err := readLines(f)
	if err != nil {
		return
	}
	var out []string
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

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		s := strings.TrimSpace(sc.Text())
		if s != "" {
			lines = append(lines, s)
		}
	}
	return lines, sc.Err()
}

// 检测 cookie 浏览器（仅返回 yt-dlp 支持的浏览器名）
// vars：以函数变量形式暴露，便于在测试中替换。
var detectBrowser = func() string {
	return detectBrowserOnce()
}

var (
	browserDetectOnce sync.Once
	browserDetected   string
)

func detectBrowserOnce() string {
	browserDetectOnce.Do(func() {
		browserDetected = detectBrowserUncached()
	})
	return browserDetected
}

func detectBrowserUncached() string {
	home, _ := os.UserHomeDir()
	candidates := []struct {
		name, dir string
	}{
		// 优先 Chrome 系，再 Firefox，再 Safari，最后 Edge
		{"chrome", "Library/Application Support/Google/Chrome"},
		{"chromium", "Library/Application Support/Chromium"},
		{"brave", "Library/Application Support/BraveSoftware/Brave-Browser"},
		{"firefox", "Library/Application Support/Firefox"},
		{"safari", "Library/Containers/com.apple.Safari/Data/Library/Cookies"},
		{"safari", "Library/Cookies"},
		{"edge", "Library/Application Support/Microsoft Edge"},
	}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(home, c.dir)); err == nil {
			return c.name
		}
	}
	return ""
}

// isXURL 判断是否为 X / Twitter 链接（含 t.co 短链）。
// X 对匿名请求会限制画质，许多视频需登录态（Requires authentication）才能拿到最高画质，
// 因此需要自动携带浏览器 cookies。
func isXURL(u string) bool {
	u = strings.ToLower(strings.TrimSpace(u))
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	return strings.HasPrefix(u, "x.com/") ||
		strings.HasPrefix(u, "www.x.com/") ||
		strings.HasPrefix(u, "m.x.com/") ||
		strings.HasPrefix(u, "twitter.com/") ||
		strings.HasPrefix(u, "www.twitter.com/") ||
		strings.HasPrefix(u, "mobile.twitter.com/") ||
		strings.HasPrefix(u, "m.twitter.com/") ||
		strings.HasPrefix(u, "t.co/")
}

func hasCmd(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// yt-dlp 下载参数
func (c *Config) ytdlpArgs(url string) []string {
	args := []string{}
	browser := c.CookieBrowser
	if browser == "" && isXURL(url) {
		// X/Twitter 默认下载最高画质需要一个已登录的浏览器 cookies（否则画质被封顶、
		// 认证受限视频无法下载）。用户未显式指定浏览器时自动探测。
		browser = detectBrowser()
	}
	if browser != "" {
		args = append(args, "--cookies-from-browser", browser)
	}
	if !c.Playlist {
		args = append(args, "--no-playlist")
	}

	retries := c.RetryTimes
	if retries < 0 {
		retries = 0
	}
	if retries > 5 {
		retries = 5
	}

	// 分片并发随任务并发下调，避免 16×16 打满磁盘/网络
	fragments := 16
	if c.Concurrency > 4 {
		fragments = 8
	}
	if c.Concurrency > 8 {
		fragments = 4
	}

	args = append(args,
		"-f", c.Format,
		"--merge-output-format", c.MergeFormat,
		"-o", "%(title)s [%(id)s].%(ext)s",
		"-P", c.DownloadDir,
		"--no-overwrites",
		"--concurrent-fragments", strconv.Itoa(fragments),
		"--http-chunk-size", "10M",
		"--buffer-size", "16K",
		"--socket-timeout", "30",
		"--retries", strconv.Itoa(retries),
		"--fragment-retries", strconv.Itoa(retries),
		"--newline",
		"--progress",
	)
	if c.AudioOnly {
		args = append(args, "--extract-audio", "--audio-format", "mp3", "--audio-quality", "0")
	}
	if c.Embed {
		args = append(args, "--embed-metadata", "--embed-thumbnail")
	}
	if c.Subs && !c.AudioOnly {
		args = append(args, "--write-subs", "--sub-langs", "zh.*,en.*", "--embed-subs")
	}
	args = append(args, url)
	return args
}

// notify 发送 macOS 桌面通知（非 macOS 平台静默跳过）
func notify(title, msg, sound string) {
	if runtime.GOOS != "darwin" {
		return
	}
	esc := func(s string) string {
		s = strings.ReplaceAll(s, "\n", " ")
		s = strings.ReplaceAll(s, "\r", " ")
		return strings.ReplaceAll(s, `"`, `""`)
	}
	script := fmt.Sprintf(`display notification "%s" with title "%s" sound name "%s"`,
		esc(msg), esc(title), esc(sound))
	_ = exec.Command("osascript", "-e", script).Start()
}
