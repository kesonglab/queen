package main

import (
	"fmt"
	"strings"
)

// taskBlockHeight 返回任务在列表中占用的行数。
func taskBlockHeight(t Task) int {
	switch t.Status {
	case StatusDownloading, StatusPostProcessing:
		return 2
	default:
		return 1
	}
}

func (m *model) viewDownload() ([]string, []keyHint) {
	th := m.theme
	width := m.frame.contentWidth()
	bodyH := m.frame.contentHeight()

	tasks := m.filteredTasks()
	m.taskCursor = clampInt(m.taskCursor, 0, maxInt(len(tasks)-1, 0))

	// 顶部：筛选状态与计数
	filterLabel := tr(m.cfg, "task_filter_all")
	switch m.filter {
	case filterActive:
		filterLabel = tr(m.cfg, "task_filter_active")
	case filterFailed:
		filterLabel = tr(m.cfg, "task_filter_failed")
	}
	head := th.Dim.Render(tr(m.cfg, "download_filter")+": ") +
		th.AccentText.Render(filterLabel) + "  " +
		th.Dim.Render(tr(m.cfg, "task_selected_hint", len(tasks)))

	body := []string{clipStyled(head, width), ""}

	// 任务列表可用高度 = 总高度 - 顶部 2 行 - 底部批量面板 3 行
	listH := maxInt(bodyH-5, 3)

	// 调整滚动位置，保证光标可见
	scroll := m.taskScroll
	if scroll < 0 {
		scroll = 0
	}
	if m.taskCursor < scroll {
		scroll = m.taskCursor
	}
	for {
		used := 0
		for i := scroll; i <= m.taskCursor && i < len(tasks); i++ {
			used += taskBlockHeight(tasks[i])
		}
		if used <= listH || scroll >= m.taskCursor {
			break
		}
		scroll++
	}
	if scroll >= len(tasks) {
		scroll = maxInt(len(tasks)-1, 0)
	}
	m.taskScroll = scroll

	// 填充可见任务块
	used := 0
	var blocks []string
	hidden := 0
	for i := scroll; i < len(tasks); i++ {
		h := taskBlockHeight(tasks[i])
		if used+h > listH {
			hidden = len(tasks) - i
			break
		}
		blocks = append(blocks, m.taskBlock(tasks[i], width, i == m.taskCursor)...)
		used += h
	}
	if len(tasks) == 0 {
		blocks = append(blocks, th.Dim.Render("  "+tr(m.cfg, "download_empty")))
	}
	body = append(body, blocks...)
	if hidden > 0 {
		body = append(body, th.Dim.Render("  "+tr(m.cfg, "more_items", hidden)))
	}

	// 底部批量面板
	body = append(body, "")
	body = append(body, m.batchPanel(width)...)

	hints := []keyHint{
		{"↑↓", "help_key_move"},
		{"x", "help_key_cancel_task"},
		{"f", "help_key_filter"},
		{"q", "help_key_abort"},
	}
	return body, hints
}

// taskBlock 渲染单个任务。
func (m *model) taskBlock(t Task, width int, selected bool) []string {
	th := m.theme
	icon, iconStyle := statusGlyph(t, m.spin, th)
	mark := marker(th, selected)

	idxTag := th.Dim.Render(fmt.Sprintf("%d.", t.Index+1))
	phase := ""
	if !t.Status.IsTerminal() {
		phaseText := tr(m.cfg, t.PhaseText())
		// 后处理阶段附上 yt-dlp 的后处理器名称（Merger / ExtractAudio ...）
		if t.Phase != phaseDownloading && t.Phase != phaseExtracting && t.PhaseDetail != "" &&
			!strings.HasPrefix(t.PhaseDetail, "retry_") {
			phaseText += " " + t.PhaseDetail
		}
		phase = th.InfoText.Render(phaseText)
	}
	if t.Attempt > 0 {
		phase += " " + th.WarnText.Render(fmt.Sprintf("↻%d", t.Attempt))
	}
	playlist := ""
	if t.PlaylistCount > 0 {
		playlist = th.Dim.Render(tr(m.cfg, "playlist_pos", t.PlaylistIndex, t.PlaylistCount))
	}

	// 标题行：图标 + 序号 + 名称 + 阶段/播放列表位置
	twoLine := taskBlockHeight(t) == 2
	prefixW := 2 + 1 + 1 + displayWidth(fmt.Sprintf("%d.", t.Index+1)) + 1
	suffixParts := []string{}
	if playlist != "" {
		suffixParts = append(suffixParts, playlist)
	}
	if phase != "" && twoLine {
		suffixParts = append(suffixParts, phase)
	}
	suffix := strings.Join(suffixParts, "  ")
	nameW := width - prefixW - displayWidth(suffix) - 2
	if nameW < 8 {
		nameW = maxInt(width-prefixW-2, 8)
		suffix = ""
	}
	name := truncateWidth(t.Name(), nameW)
	if selected {
		name = th.Selected.Render(name)
	} else {
		name = th.TextStyle.Render(name)
	}
	line1 := mark + iconStyle.Render(icon) + " " + idxTag + " " + name
	if suffix != "" {
		line1 += "  " + suffix
	}

	out := []string{clipStyled(line1, width)}
	if taskBlockHeight(t) == 1 {
		// 终态：附加状态/原因
		statusText := tr(m.cfg, t.Status.I18nKey())
		if t.Status == StatusFailed {
			statusText = tr(m.cfg, t.ErrKind.I18nKey())
			statusText = th.DangerText.Render(tr(m.cfg, "status_failed") + " · " + statusText)
		} else if t.Status == StatusDone {
			statusText = th.SuccessText.Render(statusText)
		} else {
			statusText = th.WarnText.Render(statusText)
		}
		out[0] = clipStyled(line1+"  "+statusText, width)
		return out
	}

	// 进行中：第二行显示进度条与数值
	barW := clampInt(width-56, 8, 30)
	bar := renderProgressBar(t.Percent, barW, th.BarFill, th.BarEmpty)
	stats := fmt.Sprintf("%s %s  %s  %s",
		th.AccentText.Render(pctText(t.Percent)),
		th.Dim.Render(t.SizeText()),
		th.Dim.Render(t.SpeedText()),
		th.Dim.Render("ETA "+t.ETAText()),
	)
	if t.TotalBytes == 0 && t.FragCount > 0 {
		stats += "  " + th.Dim.Render(fmt.Sprintf("frag %d/%d", t.FragIndex, t.FragCount))
	}
	line2 := "      " + bar + " " + stats
	out = append(out, clipStyled(line2, width))
	return out
}

// batchPanel 渲染批量进度面板。
func (m *model) batchPanel(width int) []string {
	th := m.theme
	total := len(m.tasks)
	ok, failed, aborted, active := 0, 0, 0, 0
	progressSum := 0.0
	for _, t := range m.tasks {
		switch t.Status {
		case StatusDone:
			ok++
		case StatusFailed:
			failed++
		case StatusAborted:
			aborted++
		default:
			active++
			progressSum += clampFloat(t.Percent, 0, 100)
		}
	}
	completed := ok + failed + aborted
	overall := 0.0
	if total > 0 {
		overall = (float64(completed) + progressSum/100) / float64(total) * 100
	}
	overall = clampFloat(overall, 0, 100)

	// 进度条宽度按剩余文本长度自适应，避免窄终端下整行被裁剪
	countsText := tr(m.cfg, "batch_line", completed, total, failed)
	reserve := displayWidth(countsText) + 12
	barW := clampInt(width-reserve, 10, 48)
	var segs []barSegment
	if total > 0 {
		segs = append(segs,
			barSegment{frac: float64(ok) / float64(total), style: th.SuccessText},
			barSegment{frac: float64(failed) / float64(total), style: th.DangerText},
			barSegment{frac: float64(aborted) / float64(total), style: th.WarnText},
			barSegment{frac: progressSum / 100 / float64(total), style: th.BarFill},
		)
	}
	bar := renderBarSegments(barW, segs, th.BarEmpty)
	line1 := "  " + bar + " " + th.AccentText.Render(pctText(overall)) + "  " +
		th.Dim.Render(tr(m.cfg, "batch_line", completed, total, failed))
	// 窄终端下省略成功/失败小计，避免被裁剪
	if extra := "  " + th.SuccessText.Render(fmt.Sprintf("✓ %d", ok)) + " " +
		th.DangerText.Render(fmt.Sprintf("✗ %d", failed)); displayWidth(line1)+displayWidth(extra) <= width {
		line1 += extra
	}
	line1 = clipStyled(line1, width)

	elapsed := timeSince(m.batchStart)
	eta := 0.0
	if overall > 1 && elapsed > 0 {
		eta = elapsed.Seconds() * (100 - overall) / overall
	}
	line2 := "  " + th.Dim.Render(tr(m.cfg, "batch_elapsed")+" "+humanDuration(elapsed)) +
		"  " + th.Dim.Render(tr(m.cfg, "batch_eta")+" "+humanDuration(durationFromSec(eta))) +
		"  " + th.Dim.Render(tr(m.cfg, "batch_running", active)) +
		"  " + th.Dim.Render(tr(m.cfg, "batch_abort_hint"))
	line2 = clipStyled(line2, width)
	return []string{line1, line2}
}
