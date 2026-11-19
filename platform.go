package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

// extraPathDirs 是 GUI / Finder 启动时可能缺失的常用工具目录。
// macOS 从 Finder 启动时 PATH 往往只有 /usr/bin:/bin:/usr/sbin:/sbin，
// 这会把 Homebrew 安装的 yt-dlp / ffmpeg 误判为缺失。
func extraPathDirs() []string {
	return []string{"/opt/homebrew/bin", "/usr/local/bin", "/opt/local/bin", "/usr/bin", "/bin"}
}

// ensurePath 把常用目录补进 PATH（幂等）。
func ensurePath() {
	cur := os.Getenv("PATH")
	parts := strings.Split(cur, ":")
	have := make(map[string]bool, len(parts))
	for _, p := range parts {
		have[p] = true
	}
	for _, d := range extraPathDirs() {
		if !have[d] {
			cur = cur + ":" + d
			have[d] = true
		}
	}
	_ = os.Setenv("PATH", strings.TrimPrefix(cur, ":"))
}

// lookupCmd 在增强后的 PATH 中查找可执行文件。
func lookupCmd(name string) (string, error) {
	return exec.LookPath(name)
}

func hasCmd(name string) bool {
	_, err := lookupCmd(name)
	return err == nil
}

// runCommand 执行命令并返回合并输出（用于短命令，例如 yt-dlp --version）。
func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// runCommandTimeout 带超时执行命令。
func runCommandTimeout(timeout time.Duration, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	done := make(chan struct{})
	var out []byte
	var err error
	go func() {
		out, err = cmd.CombinedOutput()
		close(done)
	}()
	select {
	case <-done:
		return string(out), err
	case <-time.After(timeout):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return string(out), errTimeout
	}
}

var errTimeout = &timeoutError{}

type timeoutError struct{}

func (*timeoutError) Error() string { return "command timed out" }

// openPath 在系统文件管理器中打开路径。
func openPath(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", path).Start()
	case "windows":
		return exec.Command("explorer", path).Start()
	default:
		return exec.Command("xdg-open", path).Start()
	}
}

// clipboardRead 读取系统剪贴板。
func clipboardRead() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("pbpaste").Output()
		return string(out), err
	case "windows":
		out, err := exec.Command("powershell", "-NoProfile", "-Command", "Get-Clipboard").Output()
		return string(out), err
	default:
		for _, c := range [][]string{{"wl-paste", "--no-newline"}, {"xclip", "-selection", "clipboard", "-o"}, {"xsel", "--clipboard", "--output"}} {
			if out, err := exec.Command(c[0], c[1:]...).Output(); err == nil {
				return string(out), nil
			}
		}
		return "", errNoClipboard
	}
}

var errNoClipboard = &clipboardError{}

type clipboardError struct{}

func (*clipboardError) Error() string { return "clipboard unavailable" }

// clipboardWrite 写入系统剪贴板。
func clipboardWrite(s string) error {
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("pbcopy")
		cmd.Stdin = strings.NewReader(s)
		return cmd.Run()
	case "windows":
		cmd := exec.Command("powershell", "-NoProfile", "-Command", "Set-Clipboard")
		cmd.Stdin = strings.NewReader(s)
		return cmd.Run()
	default:
		for _, c := range [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}} {
			cmd := exec.Command(c[0], c[1:]...)
			cmd.Stdin = strings.NewReader(s)
			if err := cmd.Run(); err == nil {
				return nil
			}
		}
		return errNoClipboard
	}
}

// killProcessGroup 杀掉整个进程组（yt-dlp + ffmpeg 等子进程）。
func killProcessGroup(pid int) {
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

// ---------- 浏览器 cookie 探测 ----------

var detectBrowser = func() string { return detectBrowserOnce() }

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

// detectBrowserUncached 按优先级探测本机已安装的浏览器（返回 yt-dlp 认识的名称）。
func detectBrowserUncached() string {
	home := homeDir()
	candidates := []struct{ name, dir string }{
		{"chrome", "Library/Application Support/Google/Chrome"},
		{"chromium", "Library/Application Support/Chromium"},
		{"brave", "Library/Application Support/BraveSoftware/Brave-Browser"},
		{"firefox", "Library/Application Support/Firefox"},
		{"edge", "Library/Application Support/Microsoft Edge"},
		{"safari", "Library/Containers/com.apple.Safari/Data/Library/Cookies"},
		{"safari", "Library/Cookies"},
	}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(home, c.dir)); err == nil {
			return c.name
		}
	}
	return ""
}

// ensureDir 创建目录并确认可用（供设置页与下载器共用）。
func ensureDir(path string) error {
	if strings.TrimSpace(path) == "" {
		return &pathError{"empty path"}
	}
	return os.MkdirAll(path, 0o755)
}

type pathError struct{ msg string }

func (e *pathError) Error() string { return e.msg }

// isBrewManaged 判断某个可执行文件是否由 Homebrew 管理（用于决定升级方式）。
func isBrewManaged(path string) bool {
	if path == "" {
		return false
	}
	return strings.Contains(path, "/Cellar/") || strings.Contains(path, "/opt/homebrew/") || strings.Contains(path, "/usr/local/opt/")
}
