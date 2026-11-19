package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// spinnerFrames 是下载中任务的状态动画帧（8 帧循环）。
var spinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

func spinnerFrame(n int) string {
	if len(spinnerFrames) == 0 {
		return " "
	}
	return spinnerFrames[((n%len(spinnerFrames))+len(spinnerFrames))%len(spinnerFrames)]
}

// statusGlyph 返回任务状态图标与配色。
func statusGlyph(t Task, spin int, th *Theme) (string, lipgloss.Style) {
	switch t.Status {
	case StatusDone:
		return "✓", th.SuccessText
	case StatusFailed:
		return "✗", th.DangerText
	case StatusAborted:
		return "⊘", th.WarnText
	case StatusPending:
		return "·", th.Dim
	case StatusExtracting:
		return spinnerFrame(spin), th.InfoText
	case StatusPostProcessing:
		return "⚙", th.InfoText
	default:
		return spinnerFrame(spin), th.AccentText
	}
}

// ---------- 进度条 ----------

// barSegment 是一段有颜色的进度片段。
type barSegment struct {
	frac  float64 // 0..1
	style lipgloss.Style
}

// renderBarSegments 绘制分段进度条；剩余部分用 empty 样式填充。
func renderBarSegments(width int, segs []barSegment, empty lipgloss.Style) string {
	if width < 1 {
		width = 1
	}
	var b strings.Builder
	used := 0
	for _, seg := range segs {
		if seg.frac <= 0 {
			continue
		}
		n := int(seg.frac*float64(width) + 0.5)
		if n <= 0 {
			continue
		}
		if used+n > width {
			n = width - used
		}
		if n <= 0 {
			break
		}
		b.WriteString(seg.style.Render(strings.Repeat("█", n)))
		used += n
	}
	if used < width {
		b.WriteString(empty.Render(strings.Repeat("░", width-used)))
	}
	return b.String()
}

// renderProgressBar 绘制单段进度条。
func renderProgressBar(pct float64, width int, fill, empty lipgloss.Style) string {
	return renderBarSegments(width, []barSegment{{frac: clampFloat(pct/100, 0, 1), style: fill}}, empty)
}

// pctText 返回右对齐的百分比文本。
func pctText(pct float64) string {
	return fmt.Sprintf("%3d%%", int(clampFloat(pct, 0, 100)+0.5))
}

// ---------- 键位提示 ----------

// keyHint 是一个键位提示（描述用 i18n key，渲染时翻译）。
type keyHint struct {
	key     string
	descKey string
}

// renderHints 渲染一排键位提示；空间不足时自动截断。
func renderHints(th *Theme, cfg *Config, hints []keyHint, maxWidth int) string {
	parts := make([]string, 0, len(hints))
	for _, h := range hints {
		parts = append(parts, th.StatusKey.Render("["+h.key+"]")+" "+th.StatusBar.Render(tr(cfg, h.descKey)))
	}
	s := strings.Join(parts, th.StatusBar.Render(" · "))
	if maxWidth > 0 && displayWidth(s) > maxWidth {
		s = clipStyled(s, maxWidth)
	}
	return s
}

// renderStatusLine 组合左提示与右提示，左右对齐到 width。
func renderStatusLine(th *Theme, left, right string, width int) string {
	if displayWidth(right) >= width {
		return clipStyled(right, width)
	}
	avail := width - displayWidth(right)
	if displayWidth(left) > avail {
		left = clipStyled(left, maxInt(avail-1, 0))
	}
	gap := width - displayWidth(left) - displayWidth(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// ---------- 表格 ----------

type tableAlign uint8

const (
	alignLeft tableAlign = iota
	alignRight
	alignCenter
)

// tableCell 是一个表格单元格：raw 文本参与宽度计算，style 在截断后应用。
type tableCell struct {
	raw   string
	style lipgloss.Style
	align tableAlign
}

func textCell(s string, style lipgloss.Style) tableCell {
	return tableCell{raw: s, style: style, align: alignLeft}
}

func rightCell(s string, style lipgloss.Style) tableCell {
	return tableCell{raw: s, style: style, align: alignRight}
}

// columnWidths 计算各列的自然宽度（显示列）。
func columnWidths(rows [][]tableCell) []int {
	if len(rows) == 0 {
		return nil
	}
	widths := make([]int, 0, len(rows[0]))
	for i := range rows[0] {
		w := 0
		for _, row := range rows {
			if i >= len(row) {
				continue
			}
			if cw := displayWidth(row[i].raw); cw > w {
				w = cw
			}
		}
		widths = append(widths, w)
	}
	return widths
}

// renderTableRow 渲染一行；widths 为每列宽度，gap 为列间空格。
func renderTableRow(cells []tableCell, widths []int, gap int) string {
	parts := make([]string, 0, len(cells))
	for i, c := range cells {
		w := 0
		if i < len(widths) {
			w = widths[i]
		}
		text := c.raw
		if w > 0 && displayWidth(text) > w {
			text = truncateWidth(text, w)
		}
		if w > 0 {
			switch c.align {
			case alignRight:
				text = padLeft(text, w)
			case alignCenter:
				l := (w - displayWidth(text)) / 2
				if l < 0 {
					l = 0
				}
				text = strings.Repeat(" ", l) + text
				text = padWidth(text, w)
			default:
				text = padWidth(text, w)
			}
		}
		parts = append(parts, c.style.Render(text))
	}
	return strings.Join(parts, strings.Repeat(" ", gap))
}

// renderTableRows 渲染整张表（每行按自身列数渲染）。
func renderTableRows(rows [][]tableCell, gap int) []string {
	widths := columnWidths(rows)
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, renderTableRow(row, widths, gap))
	}
	return out
}

// sectionHeader 渲染「▌ 标题 ─────」形式的分节标题。
func sectionHeader(th *Theme, title string, width int) string {
	prefix := th.AccentText.Render("▌ ") + th.DimBold.Render(title)
	used := displayWidth("▌ ") + displayWidth(title)
	remain := width - used - 1
	if remain < 2 {
		return prefix
	}
	return prefix + " " + th.Separator.Render(strings.Repeat("─", remain))
}

// ruleRow 渲染整行分隔线。
func ruleRow(th *Theme, width int) string {
	return th.Separator.Render(strings.Repeat("─", maxInt(width, 1)))
}

// marker 返回列表选中行前缀。
func marker(th *Theme, selected bool) string {
	if selected {
		return th.Cursor.Render("▶ ")
	}
	return "  "
}
