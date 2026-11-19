package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// displayWidth 返回字符串在终端中的显示宽度（已忽略 ANSI 转义序列，CJK 记 2 列）。
// lipgloss.Width 内部基于 runewidth，正好满足这里的全部需求。
func displayWidth(s string) int { return lipglossWidth(s) }

// padWidth 用空格把 s 补齐到 n 个显示列；超长则原样返回。
func padWidth(s string, n int) string {
	w := displayWidth(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}

// padLeft 左补空格到 n 个显示列。
func padLeft(s string, n int) string {
	w := displayWidth(s)
	if w >= n {
		return s
	}
	return strings.Repeat(" ", n-w) + s
}

// truncateWidth 按显示列截断纯文本（不含 ANSI），超出时追加省略号。
func truncateWidth(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if displayWidth(s) <= n {
		return s
	}
	if n <= 3 {
		return strings.Repeat(".", n)
	}
	var b strings.Builder
	used := 0
	limit := n - 1
	for _, r := range s {
		rw := runeWidth(r)
		if used+rw > limit {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	return b.String() + "…"
}

func runeWidth(r rune) int {
	// 与 lipgloss/runewidth 同源：东亚宽字符占两列，组合字符不占列。
	return lipglossRuneWidth(r)
}

// humanBytes 把字节数格式化为人类可读字符串。b<=0 时返回占位符。
func humanBytes(b float64) string {
	if b <= 0 || math.IsNaN(b) || math.IsInf(b, 0) {
		return "—"
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}
	i := 0
	for b >= 1024 && i < len(units)-1 {
		b /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%dB", int(b))
	}
	return fmt.Sprintf("%.1f%s", b, units[i])
}

// humanRate 格式化速率（字节/秒）。
func humanRate(bps float64) string {
	if bps <= 0 || math.IsNaN(bps) || math.IsInf(bps, 0) {
		return "—"
	}
	return humanBytes(bps) + "/s"
}

// fmtSec 把秒数格式化为 H:MM:SS（小于 1 小时时为 M:SS 更紧凑，但为对齐统一保留小时位）。
func fmtSec(sec int) string {
	if sec < 0 {
		sec = 0
	}
	h := sec / 3600
	sec %= 3600
	m := sec / 60
	s := sec % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// humanDuration 把时长格式化为「1h 2m」「3m 5s」「12s」这类紧凑形式。
func humanDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d.Seconds())
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	switch {
	case h > 0:
		return fmt.Sprintf("%dh %02dm", h, m)
	case m > 0:
		return fmt.Sprintf("%dm %02ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

// formatBytesField 解析 yt-dlp 模板里可能是 "NA"/空串的数值字段。
func parseNum(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "NA" || s == "None" || s == "null" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || v < 0 {
		return 0, false
	}
	return v, true
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// stripANSI 移除常见的 ANSI CSI/OSC 转义序列（用于日志与错误详情）。
func stripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != 0x1b {
			b.WriteByte(c)
			continue
		}
		// ESC 序列：CSI (ESC [) / OSC (ESC ]) / 其他两字节
		j := i + 1
		if j >= len(s) {
			break
		}
		switch s[j] {
		case '[':
			j++
			for j < len(s) && !(s[j] >= 0x40 && s[j] <= 0x7e) {
				j++
			}
			i = j
		case ']':
			j++
			for j < len(s) && s[j] != 0x07 && !(s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\') {
				j++
			}
			if j < len(s) && s[j] == 0x1b {
				j++
			}
			i = j
		default:
			i = j
		}
	}
	return b.String()
}

// oneLine 折叠所有空白为单个空格并去掉首尾空白。
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// truncateRunes 按 rune 数截断并追加省略号（用于日志/错误详情）。
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 3 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

// hostOf 从 URL 中提取小写主机名（不含端口）。解析失败时返回空串。
func hostOf(rawURL string) string {
	s := strings.TrimSpace(rawURL)
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.LastIndex(s, ":"); i >= 0 && !strings.Contains(s[i:], "]") {
		s = s[:i]
	}
	return strings.ToLower(strings.Trim(s, "[]"))
}

// shortURL 去掉协议前缀并截断，用于紧凑显示。
func shortURL(u string) string {
	s := strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	return truncateWidth(s, 72)
}

// parseURLs 从任意文本中提取 http(s) 链接（去重、保留顺序）。
func parseURLs(s string) []string {
	var out []string
	seen := make(map[string]struct{})
	for _, field := range strings.Fields(s) {
		field = strings.TrimSpace(field)
		field = strings.Trim(field, "\"'`,;<>()[]")
		if !strings.HasPrefix(field, "http://") && !strings.HasPrefix(field, "https://") {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	return out
}

// dedupeURLs 去重且保留顺序。
func dedupeURLs(urls []string) []string {
	seen := make(map[string]struct{}, len(urls))
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}
	return out
}
