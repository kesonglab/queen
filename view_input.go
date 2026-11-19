package main

import (
	"strings"
)

func (m *model) viewInput() ([]string, []keyHint) {
	th := m.theme
	width := m.frame.contentWidth()

	var body []string
	if m.inputMode == inputURLs {
		urls := parseURLs(m.input.Value())
		if len(urls) == 0 {
			body = append(body, th.Dim.Render(tr(m.cfg, "input_url_hint")))
		} else {
			body = append(body, th.Dim.Render(tr(m.cfg, "input_count", len(urls))))
		}
	} else {
		body = append(body, th.Dim.Render(tr(m.cfg, "input_dir_hint")))
	}
	body = append(body, "")

	// 输入框：总宽度 = 内容宽度（lipgloss 的 Width 不含边框，边框另加 2 列）
	panelW := maxInt(width-2, 20)
	innerW := maxInt(panelW-4, 10)
	m.input.SetWidth(innerW)
	m.input.Placeholder = tr(m.cfg, "input_placeholder")
	if m.inputMode == inputDir {
		m.input.Placeholder = tr(m.cfg, "input_dir_placeholder")
	}
	box := m.theme.Panel.Width(panelW).Render(m.input.View())
	body = append(body, strings.Split(box, "\n")...)

	hints := []keyHint{
		{"Ctrl+D", "help_key_confirm"},
		{"Esc", "help_key_back"},
	}
	return body, hints
}
