package main

import (
	"strings"
)

func (m *model) viewSites() ([]string, []keyHint) {
	th := m.theme
	width := m.frame.contentWidth()

	builtin := len(m.db.List()) - m.db.UserProfileCount()
	body := []string{
		th.Dim.Render(tr(m.cfg, "sites_builtin_count", builtin) + " · " +
			tr(m.cfg, "sites_user_count", m.db.UserProfileCount())),
		"",
	}

	profiles := m.db.List()
	nameW, domainW, policyW, flagW := 14, 26, 24, 16
	if width < 90 {
		nameW, domainW, policyW, flagW = 12, 20, 18, 12
	}
	rows := make([][]tableCell, 0, len(profiles)+1)
	rows = append(rows, []tableCell{
		textCell(tr(m.cfg, "sites_col_name"), th.TableHead),
		textCell(tr(m.cfg, "sites_col_match"), th.TableHead),
		textCell(tr(m.cfg, "sites_col_policy"), th.TableHead),
		textCell(tr(m.cfg, "sites_col_flags"), th.TableHead),
	})
	for _, p := range profiles {
		name := p.Name
		if name == "" {
			name = "—"
		}
		domains := strings.Join(p.Match, ", ")
		var policy []string
		if p.Cookies != "" {
			c := p.Cookies
			if c == "auto" {
				c = tr(m.cfg, "auto_detect")
			}
			policy = append(policy, tr(m.cfg, "sites_policy_cookies", c))
		}
		if p.Impersonate != "" {
			policy = append(policy, tr(m.cfg, "sites_policy_impersonate", p.Impersonate))
		}
		if p.Fragments > 0 {
			policy = append(policy, tr(m.cfg, "sites_policy_frag", p.Fragments))
		}
		if len(policy) == 0 {
			policy = append(policy, "—")
		}
		var flags []string
		if p.Privacy {
			flags = append(flags, tr(m.cfg, "sites_policy_privacy"))
		}
		if p.Embed != nil && !*p.Embed {
			flags = append(flags, tr(m.cfg, "sites_policy_embed_off"))
		}
		if len(flags) == 0 {
			flags = append(flags, "—")
		}
		rows = append(rows, []tableCell{
			textCell(name, th.TextStyle),
			textCell(domains, th.Dim),
			textCell(strings.Join(policy, " · "), th.InfoText),
			textCell(strings.Join(flags, " · "), th.WarnText),
		})
	}
	widths := []int{nameW, domainW, policyW, flagW}
	rendered := make([]string, 0, len(rows))
	for _, row := range rows {
		rendered = append(rendered, "  "+clipStyled(renderTableRow(row, widths, 2), width-2))
	}
	// skip header row from scroll window
	visible := maxInt(m.frame.contentHeight()-7, 2)
	bodyRows := rendered[1:]
	m.sitesScroll = clampScroll(m.sitesScroll, len(bodyRows), visible)
	window, offset := scrollWindow(bodyRows, m.sitesScroll, visible)
	m.sitesScroll = offset

	body = append(body, rendered[0], "  "+th.rule(width-2))
	body = append(body, window...)
	if len(bodyRows) > visible {
		body = append(body, "  "+th.Dim.Render(tr(m.cfg, "more_items", len(bodyRows)-visible)))
	}

	// 底部说明：用户档案路径 + 覆盖规则
	body = append(body, "")
	note := tr(m.cfg, "sites_note", sitesPath())
	for _, l := range wrapPlain(note, maxInt(width-4, 10)) {
		body = append(body, "  "+th.Dim.Render(l))
	}
	for _, w := range m.db.Warnings() {
		body = append(body, "  "+th.WarnText.Render(tr(m.cfg, "sites_warning", oneLine(w))))
	}

	hints := []keyHint{
		{"↑↓", "help_key_move"},
		{"Esc", "help_key_back"},
	}
	return body, hints
}

func (m *model) viewUpdate() ([]string, []keyHint) {
	th := m.theme
	width := m.frame.contentWidth()

	cur := m.ytdlp
	versionText := cur.Version
	if versionText == "" {
		versionText = tr(m.cfg, "menu_info_not_checked")
	}
	body := []string{
		"  " + th.Dim.Render(tr(m.cfg, "update_current")+": ") +
			th.TextStyle.Render(versionText) + "  " + th.Dim.Render(cur.Path),
		"  " + th.Dim.Render(tr(m.cfg, "update_cmd")+": ") +
			th.TextStyle.Render(cur.UpgradeLabel()),
		"",
	}

	switch {
	case m.updRunning:
		body = append(body, "  "+th.InfoText.Render(spinnerFrame(m.spin)+" "+tr(m.cfg, "update_running")))
	case m.updDone && m.updOK:
		body = append(body, "  "+th.SuccessText.Render("✓ "+tr(m.cfg, "update_ok")))
	case m.updDone:
		body = append(body, "  "+th.DangerText.Render("✗ "+tr(m.cfg, "update_fail")))
	}
	body = append(body, "")

	lines := strings.Split(strings.TrimRight(m.updOut, "\n"), "\n")
	visible := maxInt(m.frame.contentHeight()-8, 2)
	m.updScroll = clampScroll(m.updScroll, len(lines), visible)
	window, offset := scrollWindow(lines, m.updScroll, visible)
	m.updScroll = offset
	for _, line := range window {
		body = append(body, "  "+th.Dim.Render(clipStyled(line, width-4)))
	}
	if len(lines) > visible {
		body = append(body, "  "+th.Dim.Render(tr(m.cfg, "more_items", len(lines)-visible)))
	}

	body = append(body, "")
	body = append(body, "  "+th.Dim.Render(tr(m.cfg, "update_hint")))
	if cur.BrewManaged {
		body = append(body, "  "+th.Dim.Render(tr(m.cfg, "update_brew_note")))
	}

	hints := []keyHint{{"Esc", "help_key_back"}}
	return body, hints
}

func (m *model) viewChangelog() ([]string, []keyHint) {
	m.ensureChangelog()
	width := m.frame.contentWidth()

	visible := m.frame.contentHeight()
	m.chgScroll = clampScroll(m.chgScroll, len(m.chgLines), visible)
	window, offset := scrollWindow(m.chgLines, m.chgScroll, visible)
	m.chgScroll = offset

	body := make([]string, 0, len(window))
	for _, line := range window {
		body = append(body, clipStyled(line, width))
	}
	hints := []keyHint{
		{"↑↓", "help_key_move"},
		{"Esc", "help_key_back"},
	}
	return body, hints
}

// helpLine 是帮助弹层的一行。
type helpLine struct {
	sectionKey string // 非空 = 分节标题
	key        string
	descKey    string
}

func helpLineItems() []helpLine {
	return []helpLine{
		{sectionKey: "help_global"},
		{"", "Ctrl+C", "help_key_quit"},
		{"", "?", "help_key_help"},
		{"", "Ctrl+T", "help_key_theme"},
		{sectionKey: "help_menu"},
		{"", "↑↓ / j k", "help_key_move"},
		{"", "1-8", "help_key_select"},
		{"", "Enter", "help_key_confirm"},
		{"", "q", "help_key_quit"},
		{sectionKey: "help_input"},
		{"", "Ctrl+D", "help_key_confirm"},
		{"", "Esc", "help_key_back"},
		{sectionKey: "help_settings"},
		{"", "↑↓", "help_key_move"},
		{"", "←→", "help_key_toggle"},
		{"", "Enter", "help_key_edit"},
		{"", "Esc", "help_key_save"},
		{sectionKey: "help_download"},
		{"", "j / k", "help_key_move"},
		{"", "x", "help_key_cancel_task"},
		{"", "f", "help_key_filter"},
		{"", "q / Esc", "help_key_abort"},
		{sectionKey: "help_summary"},
		{"", "r", "help_key_retry"},
		{"", "o", "help_key_open"},
		{"", "c", "help_key_copy"},
		{"", "l", "help_key_logs"},
	}
}

// helpLines 渲染帮助内容（keys.go 里用于 G 跳转，因此独立成函数）。
func helpLines(cfg *Config, th *Theme) []string {
	width := minInt(maxContentWidth, 90)
	items := helpLineItems()
	out := make([]string, 0, len(items)+2)
	for _, it := range items {
		if it.sectionKey != "" {
			out = append(out, sectionHeader(th, tr(cfg, it.sectionKey), width))
			continue
		}
		out = append(out, "  "+padWidth(it.key, 14)+"  "+tr(cfg, it.descKey))
	}
	return out
}

func (m *model) viewHelp() ([]string, []keyHint) {
	th := m.theme
	lines := helpLines(m.cfg, th)
	visible := m.frame.contentHeight() - 1
	m.helpScroll = clampScroll(m.helpScroll, len(lines), visible)
	window, offset := scrollWindow(lines, m.helpScroll, visible)
	m.helpScroll = offset

	body := []string{th.Title.Render(tr(m.cfg, "help_title")), ""}
	body = append(body, window...)
	if len(lines) > visible {
		body = append(body, th.Dim.Render(tr(m.cfg, "more_items", len(lines)-visible)))
	}
	hints := []keyHint{{"?", "help_key_back"}, {"↑↓", "help_key_move"}}
	return body, hints
}

func (m *model) viewEdit() ([]string, []keyHint) {
	th := m.theme
	width := m.frame.contentWidth()
	var body []string
	if m.edit == nil {
		return body, nil
	}
	body = append(body, "  "+th.Title.Render(tr(m.cfg, m.edit.titleKey)))
	body = append(body, "")
	panelW := maxInt(width-2, 20)
	m.editInput.SetWidth(maxInt(panelW-4, 10))
	box := m.theme.Panel.Width(panelW).Render(m.editInput.View())
	body = append(body, strings.Split(box, "\n")...)
	body = append(body, "")
	body = append(body, "  "+th.Dim.Render(tr(m.cfg, "edit_hint")))
	hints := []keyHint{
		{"Enter", "help_key_save"},
		{"Esc", "help_key_back"},
	}
	return body, hints
}
