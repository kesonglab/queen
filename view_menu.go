package main

import (
	"fmt"
	"strings"
)

// menuItem 描述主菜单一项。num 既是数字快捷键也是显示用的键位徽标。
type menuItem struct {
	num      string
	id       string
	labelKey string
	descKey  string
}

var menuItems = []menuItem{
	{"1", "paste", "menu_paste", "menu_paste_d"},
	{"2", "clipboard", "menu_clip", "menu_clip_d"},
	{"3", "retry", "menu_retry", "menu_retry_d"},
	{"4", "settings", "menu_settings", "menu_settings_d"},
	{"5", "sites", "menu_sites", "menu_sites_d"},
	{"6", "opendir", "menu_opendir", "menu_opendir_d"},
	{"7", "update", "menu_update", "menu_update_d"},
	{"8", "changelog", "menu_changelog", "menu_changelog_d"},
	{"q", "quit", "menu_quit", "menu_quit_d"},
}

func (m *model) viewMenu() ([]string, []keyHint) {
	th := m.theme
	width := m.frame.contentWidth()

	// 标签列宽按最长标签计算，让描述列对齐
	labelW := 0
	for _, it := range menuItems {
		if w := displayWidth(tr(m.cfg, it.labelKey)); w > labelW {
			labelW = w
		}
	}
	if labelW > 26 {
		labelW = 26
	}

	var body []string
	for i, it := range menuItems {
		if i == 6 { // 「更新 yt-dlp」前留一点呼吸
			body = append(body, "")
		}
		sel := i == m.menuCursor
		labelStyle := th.TextStyle
		if sel {
			labelStyle = th.Selected
		}
		row := marker(th, sel) +
			th.Key.Render("["+it.num+"]") + " " +
			labelStyle.Render(padWidth(truncateWidth(tr(m.cfg, it.labelKey), labelW), labelW)) +
			"  " + th.Dim.Render(tr(m.cfg, it.descKey))
		body = append(body, clipStyled(row, width))
	}

	body = append(body, "")
	body = append(body, m.menuInfoLines()...)

	hints := []keyHint{
		{"↑↓", "help_key_move"},
		{"Enter", "help_key_confirm"},
		{"q", "help_key_quit"},
	}
	return body, hints
}

// menuInfoLines 渲染菜单下方的运行状态摘要。
func (m *model) menuInfoLines() []string {
	th := m.theme
	width := m.frame.contentWidth()
	labelW := 9

	var rows []string
	// 下载目录
	dir := shortenMiddle(expandHome(m.cfg.DownloadDir), maxInt(width-labelW-2, 20))
	rows = append(rows, th.Dim.Render(padWidth(tr(m.cfg, "menu_info_dir"), labelW))+" "+
		th.TextStyle.Render(dir))

	// 失败链接
	if failed := len(readRetry(m.cfg.DownloadDir)); failed > 0 {
		rows = append(rows, th.Dim.Render(padWidth(tr(m.cfg, "menu_info_failed"), labelW))+" "+
			th.WarnText.Render(fmt.Sprintf("%d %s", failed, tr(m.cfg, "items_unit"))))
	} else {
		rows = append(rows, th.Dim.Render(padWidth(tr(m.cfg, "menu_info_failed"), labelW))+" "+
			th.Dim.Render("0"))
	}

	// 站点档案
	sites := fmt.Sprintf("%s · %s",
		tr(m.cfg, "sites_builtin_count", len(m.db.List())-m.db.UserProfileCount()),
		tr(m.cfg, "sites_user_count", m.db.UserProfileCount()))
	rows = append(rows, th.Dim.Render(padWidth(tr(m.cfg, "menu_info_sites"), labelW))+" "+
		th.TextStyle.Render(sites))

	// yt-dlp 版本
	if m.ytdlp.OK {
		v := m.ytdlp.Version
		if v == "" {
			v = tr(m.cfg, "menu_info_not_checked")
		}
		style := th.TextStyle
		if m.ytdlp.AgeDays > ytdlpStaleDays {
			style = th.WarnText
			v += "  " + tr(m.cfg, "menu_info_stale", m.ytdlp.AgeDays)
		}
		rows = append(rows, th.Dim.Render(padWidth(tr(m.cfg, "menu_info_ytdlp"), labelW))+" "+
			style.Render(v))
	}

	lines := []string{th.rule(width)}
	lines = append(lines, rows...)
	return lines
}

// shortenMiddle 把长路径压缩为 "头…/尾" 形式。
func shortenMiddle(path string, width int) string {
	if displayWidth(path) <= width {
		return path
	}
	if width < 12 {
		return truncateWidth(path, width)
	}
	tail := path
	if i := strings.LastIndex(path, "/"); i >= 0 {
		tail = path[i:]
	}
	if displayWidth(tail) > width/2 {
		tail = truncateWidth(tail, width/2)
	}
	headW := width - displayWidth(tail) - 1
	if headW < 4 {
		return truncateWidth(path, width)
	}
	return truncateWidth(path, headW) + "…" + tail
}
