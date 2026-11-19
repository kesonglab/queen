package main

import (
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// notify 发送系统桌面通知。参数通过 osascript 的 argv 传递，
// 彻底避免把远程可控文本拼进 AppleScript 源码（转义/注入问题）。
func notify(title, msg, sound string) {
	switch runtime.GOOS {
	case "darwin":
		notifyDarwin(title, msg, sound)
	case "linux":
		if hasCmd("notify-send") {
			_ = exec.Command("notify-send", title, msg).Start()
		}
	}
}

func notifyDarwin(title, msg, sound string) {
	if sound == "" {
		sound = "Glass"
	}
	// on run argv 让所有文本都以参数形式传入，无需任何转义。
	script := "on run argv\n" +
		"display notification (item 1 of argv) with title (item 2 of argv) sound name (item 3 of argv)\n" +
		"end run"
	cmd := exec.Command("osascript", "-e", script, "--", plainTextNotification(msg), plainTextNotification(title), sound)
	if err := cmd.Start(); err != nil {
		return
	}
	// 不等待会留下僵尸进程；后台回收，不阻塞 UI。
	go func() { _ = cmd.Wait() }()
}

// notifyTaskFailed 按隐私模式构造失败通知。
func notifyTaskFailed(cfg *Config, db *SiteDB, url, detail string, kind ErrorKind) {
	title := tr(cfg, "notify_fail_title")
	var msg string
	priv := cfg.PrivacyMode
	if db != nil && db.Lookup(hostOf(url)).Privacy {
		priv = true
	}
	if priv {
		msg = tr(cfg, "notify_fail_private", tr(cfg, kind.I18nKey()))
	} else {
		detail = oneLine(detail)
		if detail == "" {
			detail = tr(cfg, kind.I18nKey())
		}
		msg = shortURL(url) + ": " + truncateRunes(detail, 120)
	}
	notify(title, msg, "Basso")
}

// notifyBatchDone 批量完成通知。
func notifyBatchDone(cfg *Config, ok, failed int) {
	title := tr(cfg, "notify_done_title")
	var msg string
	sound := "Glass"
	switch {
	case failed == 0:
		msg = tr(cfg, "notify_done_ok", ok)
	case ok == 0:
		msg = tr(cfg, "notify_done_all_fail", failed)
		sound = "Basso"
	default:
		msg = tr(cfg, "notify_done_fail", ok, failed)
		sound = "Basso"
	}
	if cfg.PrivacyMode {
		msg = tr(cfg, "notify_done_private", ok, failed)
	}
	notify(title, msg, sound)
}

// notificationThrottle 防止批量失败时通知刷屏：同一 URL 2 秒内只发一次。
type notificationThrottle struct {
	last time.Time
	key  string
}

func (t *notificationThrottle) allow(key string) bool {
	now := time.Now()
	if key == t.key && now.Sub(t.last) < 2*time.Second {
		return false
	}
	t.last = now
	t.key = key
	return true
}

// plainTextNotification 把多行文本压成一行，防止通知溢出。
func plainTextNotification(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}
