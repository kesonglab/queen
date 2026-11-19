package main

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func (m *model) viewSummary() ([]string, []keyHint) {
	th := m.theme
	width := m.frame.contentWidth()

	ok, failed, aborted := 0, 0, 0
	for _, t := range m.tasks {
		switch t.Status {
		case StatusDone:
			ok++
		case StatusFailed:
			failed++
		case StatusAborted:
			aborted++
		}
	}
	total := len(m.tasks)
	_ = aborted

	var body []string

	// 概览行
	headline := th.SuccessText.Render(fmt.Sprintf("✓ %d", ok)) + "  " +
		th.DangerText.Render(fmt.Sprintf("✗ %d", failed))
	if aborted > 0 {
		headline += "  " + th.WarnText.Render(fmt.Sprintf("⊘ %d", aborted))
	}
	headline += "    " + th.Dim.Render(tr(m.cfg, "summary_total", total))
	body = append(body, "  "+clipStyled(headline, width-2))

	if m.aborted {
		body = append(body, "  "+th.WarnText.Render(tr(m.cfg, "download_aborted_note")))
	}

	if failed == 0 {
		body = append(body, "")
		body = append(body, "  "+th.SuccessText.Render(tr(m.cfg, "summary_no_fail")))
		body = append(body, "  "+th.Dim.Render(tr(m.cfg, "summary_no_fail_hint")))
		hints := []keyHint{
			{"Enter", "help_key_back"},
			{"o", "help_key_open"},
			{"c", "help_key_copy"},
		}
		return body, hints
	}

	// 失败链接文件位置
	body = append(body, "  "+th.Dim.Render(tr(m.cfg, "summary_retry_file")+": "+retryFile(m.cfg.DownloadDir)))

	// 结果表格
	body = append(body, "")
	body = append(body, m.summaryTable(width)...)

	// 操作按钮行
	body = append(body, "")
	body = append(body, "  "+renderHints(th, m.cfg, []keyHint{
		{"r", "summary_retry"},
		{"o", "summary_open"},
		{"c", "summary_copy"},
		{"l", "summary_logs"},
	}, width-2))
	hints := []keyHint{
		{"↑↓", "help_key_move"},
		{"Enter", "help_key_back"},
	}
	return body, hints
}

// summaryTable 渲染结果表格（可滚动）。
func (m *model) summaryTable(width int) []string {
	th := m.theme

	// 列宽：状态 2 / 站点 12 / 任务 自适应 / 大小 14 / 耗时 7 / 原因 18
	inner := width - 2 // 行前缀 "  "
	siteW := clampInt(inner/6, 8, 14)
	sizeW, tookW, reasonW := 16, 7, 18
	taskW := inner - (2 + siteW + sizeW + tookW + reasonW + 5*2)
	if taskW < 10 {
		sizeW = 10
		reasonW = 12
		taskW = maxInt(inner-(2+siteW+sizeW+tookW+reasonW+5*2), 8)
	}
	widths := []int{2, siteW, taskW, sizeW, tookW, reasonW}

	rows := make([][]tableCell, 0, len(m.tasks)+1)
	rows = append(rows, []tableCell{
		textCell(" ", th.TableHead),
		textCell(tr(m.cfg, "col_site"), th.TableHead),
		textCell(tr(m.cfg, "col_task"), th.TableHead),
		rightCell(tr(m.cfg, "col_size"), th.TableHead),
		rightCell(tr(m.cfg, "col_duration"), th.TableHead),
		textCell(tr(m.cfg, "col_reason"), th.TableHead),
	})

	for _, t := range m.tasks {
		icon, iconStyle := statusGlyph(t, m.spin, th)
		site := t.Site
		if site == "" {
			site = "—"
		}
		reason := ""
		switch t.Status {
		case StatusDone:
			reason = tr(m.cfg, "status_done")
		case StatusFailed:
			reason = tr(m.cfg, t.ErrKind.I18nKey())
		case StatusAborted:
			reason = tr(m.cfg, "status_aborted")
		default:
			reason = tr(m.cfg, t.PhaseText())
		}
		took := "—"
		if !t.Started.IsZero() && !t.Finished.IsZero() {
			took = humanDuration(t.Finished.Sub(t.Started))
		}
		size := "—"
		if t.TotalBytes > 0 {
			size = humanBytes(t.TotalBytes)
		} else if t.DownloadedBytes > 0 {
			size = humanBytes(t.DownloadedBytes)
		}
		rows = append(rows, []tableCell{
			{raw: icon, style: iconStyle, align: alignLeft},
			textCell(site, th.Dim),
			textCell(t.Name(), th.TextStyle),
			rightCell(size, th.Dim),
			rightCell(took, th.Dim),
			textCell(reason, reasonStyle(th, t)),
		})
	}

	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, renderTableRow(row, widths, 2))
	}
	// 表头 + 分隔线
	if len(lines) > 0 {
		out := []string{"  " + lines[0], "  " + th.rule(width-4)}
		bodyRows := lines[1:]
		// 滚动窗口：剩余高度
		visible := maxInt(m.frame.contentHeight()-8, 3)
		m.summaryScroll = clampScroll(m.summaryScroll, len(bodyRows), visible)
		window, offset := scrollWindow(bodyRows, m.summaryScroll, visible)
		m.summaryScroll = offset
		for _, l := range window {
			out = append(out, "  "+clipStyled(l, width-2))
		}
		if len(bodyRows) > visible {
			out = append(out, "  "+th.Dim.Render(tr(m.cfg, "more_items", len(bodyRows)-visible)))
		}
		return out
	}
	return nil
}

func reasonStyle(th *Theme, t Task) lipgloss.Style {
	switch t.Status {
	case StatusFailed:
		return th.DangerText
	case StatusDone:
		return th.SuccessText
	case StatusAborted:
		return th.WarnText
	default:
		return th.InfoText
	}
}

// timeSince / durationFromSec 是给视图层用的时间小工具。
func timeSince(t time.Time) time.Duration {
	if t.IsZero() {
		return 0
	}
	return time.Since(t)
}

func durationFromSec(sec float64) time.Duration {
	if sec <= 0 {
		return 0
	}
	return time.Duration(sec * float64(time.Second))
}
