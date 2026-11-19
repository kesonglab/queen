package main

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// 终端最小尺寸与框架高度。
const (
	minTermWidth  = 60
	minTermHeight = 16
	headerLines   = 2 // 标题行 + 分隔线
	statusLines   = 1 // 底部状态栏
	chromeGaps    = 2 // 头尾之间各留一个空行
)

// frame 保存终端尺寸并做响应式换算。
type frame struct {
	width, height int
}

func newFrame(w, h int) frame { return frame{width: w, height: h} }

// contentWidth 返回内容区可用宽度（居中的最大 100 列）。
func (f frame) contentWidth() int {
	w := minInt(f.width-4, maxContentWidth)
	if w < 20 {
		w = maxInt(f.width-2, 10)
	}
	return w
}

// contentHeight 返回内容区可用行数。
func (f frame) contentHeight() int {
	h := f.height - headerLines - statusLines - chromeGaps
	if h < 1 {
		h = 1
	}
	return h
}

// tooSmall 判断终端是否小于可用尺寸。
func (f frame) tooSmall() bool {
	return f.width < minTermWidth || f.height < minTermHeight
}

// leftMargin 返回内容块居中所需的左边距。
func (f frame) leftMargin() int {
	m := (f.width - f.contentWidth()) / 2
	if m < 0 {
		return 0
	}
	return m
}

// clipStyled 按显示宽度截断可能带 ANSI 的字符串。
func clipStyled(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if displayWidth(s) <= width {
		return s
	}
	return ansi.Truncate(s, width, "…")
}

// fitLines 截断/补齐行集合到恰好 n 行。
func fitLines(lines []string, n int, width int) []string {
	if n < 0 {
		n = 0
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		if i < len(lines) {
			out = append(out, clipStyled(lines[i], width))
		} else {
			out = append(out, "")
		}
	}
	return out
}

// centerLines 给每行加左边距，使内容块在终端中水平居中。
func centerLines(lines []string, margin int, width int) []string {
	if margin <= 0 {
		return lines
	}
	pad := strings.Repeat(" ", margin)
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = pad + clipStyled(l, width)
	}
	return out
}

// wrapPlain 按显示宽度折行纯文本（不处理 ANSI），用于长错误信息与提示。
func wrapPlain(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		if displayWidth(para) <= width {
			out = append(out, para)
			continue
		}
		var line strings.Builder
		used := 0
		for _, r := range para {
			rw := runeWidth(r)
			if used+rw > width && used > 0 {
				out = append(out, line.String())
				line.Reset()
				used = 0
			}
			line.WriteRune(r)
			used += rw
		}
		if line.Len() > 0 {
			out = append(out, line.String())
		}
	}
	if len(out) == 0 {
		out = []string{""}
	}
	return out
}

// --- 滚动辅助 ---

// clampScroll 把滚动偏移限制在 [0, max] 区间。
func clampScroll(offset, total, visible int) int {
	maxOff := total - visible
	if maxOff < 0 {
		maxOff = 0
	}
	if offset > maxOff {
		offset = maxOff
	}
	if offset < 0 {
		offset = 0
	}
	return offset
}

// scrollWindow 返回窗口内的元素与新的偏移。
func scrollWindow[T any](items []T, offset, visible int) ([]T, int) {
	offset = clampScroll(offset, len(items), visible)
	end := minInt(offset+visible, len(items))
	if offset > end {
		offset = end
	}
	return items[offset:end], offset
}

// keepVisible 在光标移动后调整偏移，保证光标在窗口内。
func keepVisible(offset, cursor, visible, total int) int {
	if visible <= 0 {
		return 0
	}
	if cursor < offset {
		return cursor
	}
	if cursor >= offset+visible {
		return cursor - visible + 1
	}
	return clampScroll(offset, total, visible)
}
