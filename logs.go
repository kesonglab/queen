package main

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// failureInput 是一次失败任务的完整诊断材料。
type failureInput struct {
	URL     string
	Title   string
	Site    string
	Args    []string
	Output  []string
	Kind    ErrorKind
	Detail  string
	Version string
	Privacy bool
}

// writeFailureLog 把失败诊断写入日志文件，返回路径。
//

// 日志里的 URL/标题在隐私模式下会被隐藏，但仍保留参数与输出，便于排查。
func writeFailureLog(cfg *Config, in failureInput) (string, error) {
	dir := logsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, time.Now().Format("20060102-150405")+"-"+shortHash(in.URL)+".log")

	var b strings.Builder
	b.WriteString("Queen failure log\n")
	fmt.Fprintf(&b, "time:        %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "queen:       %s\n", version)
	fmt.Fprintf(&b, "yt-dlp:      %s\n", in.Version)
	if in.Privacy {
		b.WriteString("site:        <hidden: privacy mode>\n")
	} else {
		fmt.Fprintf(&b, "site:        %s\n", in.Site)
	}
	detail, args := in.Detail, strings.Join(in.Args, " ")
	if in.Privacy {
		b.WriteString("url:         <hidden: privacy mode>\n")
		b.WriteString("title:       <hidden: privacy mode>\n")
		if in.URL != "" {
			detail = strings.ReplaceAll(detail, in.URL, "<hidden-url>")
			args = strings.ReplaceAll(args, in.URL, "<hidden-url>")
		}
	} else {
		fmt.Fprintf(&b, "url:         %s\n", in.URL)
		fmt.Fprintf(&b, "title:       %s\n", in.Title)
	}
	fmt.Fprintf(&b, "kind:        %s\n", in.Kind)
	fmt.Fprintf(&b, "detail:      %s\n", detail)
	fmt.Fprintf(&b, "args:        %s\n", args)
	b.WriteString("\n--- yt-dlp output ---\n")
	for _, line := range in.Output {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return "", err
	}
	pruneLogs(maxLogFiles)
	return path, nil
}

// pruneLogs 只保留最近 keep 个日志文件，防止无限增长。
func pruneLogs(keep int) {
	dir := logsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var logs []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		logs = append(logs, e.Name())
	}
	if len(logs) <= keep {
		return
	}
	sort.Sort(sort.Reverse(sort.StringSlice(logs)))
	for _, name := range logs[keep:] {
		_ = os.Remove(filepath.Join(dir, name))
	}
}

// latestFailureLog 返回最新的日志路径（供「打开日志」使用）。
func latestFailureLog() string {
	entries, err := os.ReadDir(logsDir())
	if err != nil {
		return ""
	}
	best := ""
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		if e.Name() > best {
			best = e.Name()
		}
	}
	if best == "" {
		return ""
	}
	return filepath.Join(logsDir(), best)
}

// diagnosticsReport 生成可一键复制/粘贴到 issue 的诊断文本。
func diagnosticsReport(cfg *Config, tasks []Task, ytdlp YtdlpInfo) string {
	var b strings.Builder
	b.WriteString("=== Queen diagnostics ===\n")
	fmt.Fprintf(&b, "queen:  %s\n", version)
	fmt.Fprintf(&b, "yt-dlp: %s (%s)\n", ytdlp.Version, ytdlp.Path)
	fmt.Fprintf(&b, "os:     %s\n", os.Getenv("GOOS"))
	fmt.Fprintf(&b, "config: concurrency=%d fragments=%d budget=%d same_site=%d retries=%d\n",
		cfg.Concurrency, cfg.Fragments, cfg.ConnectionBudget, cfg.SameSiteLimit, cfg.RetryTimes)
	fmt.Fprintf(&b, "        preset=%s merge=%s audio=%v cookies=%q impersonate=%q\n",
		cfg.FormatPreset, cfg.MergeFormat, cfg.AudioOnly, cfg.CookieBrowser, cfg.Impersonate)
	b.WriteString("\n--- failed tasks ---\n")
	failed := 0
	for _, t := range tasks {
		if t.Status != StatusFailed {
			continue
		}
		failed++
		url := t.URL
		detail := t.ErrDetail
		if cfg.PrivacyMode {
			url = "<hidden: privacy mode>"
			if t.URL != "" {
				detail = strings.ReplaceAll(detail, t.URL, "<hidden-url>")
			}
		}
		fmt.Fprintf(&b, "[%d] %s\n", t.Index+1, url)
		fmt.Fprintf(&b, "    kind: %s\n", t.ErrKind)
		fmt.Fprintf(&b, "    detail: %s\n", detail)
		if t.LogPath != "" {
			fmt.Fprintf(&b, "    log: %s\n", t.LogPath)
		}
	}
	if failed == 0 {
		b.WriteString("(none)\n")
	}
	return b.String()
}

// shortHash 生成 8 位十六进制短哈希（用于日志文件名，避免暴露完整 URL）。
func shortHash(s string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return fmt.Sprintf("%08x", h.Sum32())
}
