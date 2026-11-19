package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// settingsEntry 是设置页的一行。id 为空表示分节标题（不可选中）。
type settingsEntry struct {
	id       string
	labelKey string
}

// settingsEntries 定义设置页的结构（顺序即显示顺序）。
func settingsEntries() []settingsEntry {
	return []settingsEntry{
		{"", "settings_section_content"},
		{"playlist", "setting_playlist"},
		{"playlist_folder", "setting_playlist_folder"},
		{"audio_only", "setting_audio_only"},
		{"audio_format", "setting_audio_format"},
		{"subs", "setting_subs"},
		{"auto_subs", "setting_auto_subs"},
		{"embed", "setting_embed"},
		{"", "settings_section_format"},
		{"format_preset", "setting_format_preset"},
		{"format", "setting_format"},
		{"merge_format", "setting_merge"},
		{"", "settings_section_network"},
		{"concurrency", "setting_concurrency"},
		{"fragments", "setting_fragments"},
		{"connection_budget", "setting_budget"},
		{"same_site", "setting_same_site"},
		{"retry", "setting_retry"},
		{"retry_sleep", "setting_retry_sleep"},
		{"throttled_rate", "setting_throttled_rate"},
		{"limit_rate", "setting_limit_rate"},
		{"proxy", "setting_proxy"},
		{"archive", "setting_archive"},
		{"", "settings_section_auth"},
		{"cookie", "setting_cookie"},
		{"cookies_file", "setting_cookies_file"},
		{"impersonate", "setting_impersonate"},
		{"extractor_args", "setting_extractor_args"},
		{"user_agent", "setting_user_agent"},
		{"referer", "setting_referer"},
		{"", "settings_section_general"},
		{"dir", "setting_dir"},
		{"trim_filenames", "setting_trim_filenames"},
		{"ignore_config", "setting_ignore_config"},
		{"privacy", "setting_privacy"},
		{"failure_logs", "setting_failure_logs"},
		{"update_check", "setting_update_check"},
		{"lang", "setting_lang"},
		{"theme", "setting_theme"},
		{"", ""},
		{"action_sites", "settings_action_sites"},
		{"action_reset", "settings_action_reset"},
		{"action_save", "settings_action_save"},
	}
}

// ---------- 光标 ----------

func isSection(e settingsEntry) bool { return e.id == "" }

func (m *model) firstSettingsIndex(entries []settingsEntry) int {
	for i, e := range entries {
		if !isSection(e) {
			return i
		}
	}
	return 0
}

func (m *model) lastSettingsIndex(entries []settingsEntry) int {
	for i := len(entries) - 1; i >= 0; i-- {
		if !isSection(entries[i]) {
			return i
		}
	}
	return 0
}

// moveSettingsCursor 上下移动并跳过不可选中的分节标题。
func (m *model) moveSettingsCursor(entries []settingsEntry, delta int) {
	if len(entries) == 0 {
		return
	}
	idx := clampInt(m.settingsCursor, 0, len(entries)-1)
	step := 1
	if delta < 0 {
		step = -1
		delta = -delta
	}
	for i := 0; i < delta; i++ {
		next := idx
		for j := 0; j < len(entries); j++ {
			next += step
			if next < 0 || next >= len(entries) {
				next = idx
				break
			}
			if !isSection(entries[next]) {
				break
			}
		}
		idx = next
	}
	m.settingsCursor = idx
}

// clampSettingsCursor 保证光标落在可选中的行上。
func (m *model) clampSettingsCursor(entries []settingsEntry) {
	if len(entries) == 0 {
		m.settingsCursor = 0
		return
	}
	m.settingsCursor = clampInt(m.settingsCursor, 0, len(entries)-1)
	if isSection(entries[m.settingsCursor]) {
		m.settingsCursor = m.firstSettingsIndex(entries)
	}
}

// ---------- 值展示 ----------

func orPlaceholder(cfg *Config, v string) string {
	if strings.TrimSpace(v) == "" {
		return tr(cfg, "none_value")
	}
	return v
}

func cookieBrowserLabel(cfg *Config, v string) string {
	if v == "" {
		return tr(cfg, "auto_detect")
	}
	return v
}

func (m *model) settingValue(e settingsEntry) string {
	c := m.cfg
	switch e.id {
	case "playlist":
		return onOff(c, c.Playlist)
	case "playlist_folder":
		return onOff(c, c.PlaylistFolder)
	case "audio_only":
		return onOff(c, c.AudioOnly)
	case "audio_format":
		return c.AudioFormat
	case "subs":
		return onOff(c, c.Subs)
	case "auto_subs":
		return onOff(c, c.AutoSubs)
	case "embed":
		return onOff(c, c.Embed)
	case "format_preset":
		return presetLabel(c, c.FormatPreset)
	case "format":
		return c.effectiveFormat()
	case "merge_format":
		return c.MergeFormat
	case "concurrency":
		return fmt.Sprintf("%d", c.Concurrency)
	case "fragments":
		return fmt.Sprintf("%d", c.Fragments)
	case "connection_budget":
		return fmt.Sprintf("%d", c.ConnectionBudget)
	case "same_site":
		return fmt.Sprintf("%d", c.SameSiteLimit)
	case "retry":
		return fmt.Sprintf("%d %s", c.RetryTimes, tr(c, "times"))
	case "retry_sleep":
		return fmt.Sprintf("%d %s", c.RetrySleep, tr(c, "seconds_unit"))
	case "throttled_rate":
		return orPlaceholder(c, c.ThrottledRate)
	case "limit_rate":
		return orPlaceholder(c, c.LimitRate)
	case "proxy":
		return orPlaceholder(c, c.Proxy)
	case "archive":
		return onOff(c, c.Archive)
	case "cookie":
		return cookieBrowserLabel(c, c.CookieBrowser)
	case "cookies_file":
		return orPlaceholder(c, shortenMiddle(c.CookiesFile, 28))
	case "impersonate":
		return orPlaceholder(c, c.Impersonate)
	case "extractor_args":
		return orPlaceholder(c, renderTrunc(c.ExtractorArgs, 28))
	case "user_agent":
		return orPlaceholder(c, renderTrunc(c.UserAgent, 28))
	case "referer":
		return orPlaceholder(c, renderTrunc(c.Referer, 28))
	case "dir":
		return shortenMiddle(expandHome(c.DownloadDir), 34)
	case "trim_filenames":
		if c.TrimFilenames <= 0 {
			return tr(c, "unlimited")
		}
		return fmt.Sprintf("%d", c.TrimFilenames)
	case "ignore_config":
		return onOff(c, c.IgnoreYtdlpConfig)
	case "privacy":
		return onOff(c, c.PrivacyMode)
	case "failure_logs":
		return onOff(c, c.KeepFailureLogs)
	case "update_check":
		return onOff(c, c.UpdateCheck)
	case "lang":
		return langName(c.Lang)
	case "theme":
		return themeLabel(c, c.Theme)
	}
	return ""
}

func renderTrunc(s string, width int) string {
	return truncateWidth(oneLine(s), width)
}

// ---------- 修改 ----------

// changeSetting 处理 ←→：开关取反、枚举循环、数值增减。
func (m *model) changeSetting(entries []settingsEntry, idx, delta int) {
	if idx < 0 || idx >= len(entries) {
		return
	}
	e := entries[idx]
	if isSection(e) {
		return
	}
	c := m.cfg
	switch e.id {
	case "playlist":
		c.Playlist = !c.Playlist
	case "playlist_folder":
		c.PlaylistFolder = !c.PlaylistFolder
	case "audio_only":
		c.AudioOnly = !c.AudioOnly
	case "audio_format":
		c.AudioFormat = cycleChoice(c.AudioFormat, audioFormatOptions, delta)
	case "subs":
		c.Subs = !c.Subs
	case "auto_subs":
		c.AutoSubs = !c.AutoSubs
	case "embed":
		c.Embed = !c.Embed
	case "format_preset":
		c.FormatPreset = cycleChoice(c.FormatPreset, formatPresetOptions, delta)
	case "merge_format":
		c.MergeFormat = cycleChoice(c.MergeFormat, mergeFormatOptions, delta)
	case "concurrency":
		c.Concurrency = clampInt(c.Concurrency+delta, 1, 16)
	case "fragments":
		c.Fragments = cycleInt(c.Fragments, fragmentOptions, delta)
	case "connection_budget":
		c.ConnectionBudget = clampInt(c.ConnectionBudget+4*delta, 4, 64)
	case "same_site":
		c.SameSiteLimit = clampInt(c.SameSiteLimit+delta, 1, 8)
	case "retry":
		c.RetryTimes = clampInt(c.RetryTimes+delta, 0, 10)
	case "retry_sleep":
		c.RetrySleep = clampInt(c.RetrySleep+delta, 0, 60)
	case "archive":
		c.Archive = !c.Archive
	case "cookie":
		c.CookieBrowser = cycleChoice(c.CookieBrowser, cookieBrowserOptions, delta)
	case "impersonate":
		c.Impersonate = cycleChoice(c.Impersonate, impersonateOptions, delta)
	case "trim_filenames":
		c.TrimFilenames = cycleInt(c.TrimFilenames, []int{0, 120, 200, 240}, delta)
	case "ignore_config":
		c.IgnoreYtdlpConfig = !c.IgnoreYtdlpConfig
	case "privacy":
		c.PrivacyMode = !c.PrivacyMode
	case "failure_logs":
		c.KeepFailureLogs = !c.KeepFailureLogs
	case "update_check":
		c.UpdateCheck = !c.UpdateCheck
	case "lang":
		c.Lang = nextLang(c.Lang)
	case "theme":
		c.Theme = cycleChoice(c.Theme, themeOptions, delta)
		m.rebuildTheme()
	case "format":
		// 自定义格式只能编辑；如果当前不是 custom，先切过去
		c.FormatPreset = "custom"
		m.editSetting(e)
	default:
		// 文本项与动作项左右键无效，统一交给 Enter
		m.activateSetting(entries, idx)
	}
	c.normalize()
}

// activateSetting 处理 Enter：文本项打开编辑，动作项执行，枚举项前进一格。
func (m *model) activateSetting(entries []settingsEntry, idx int) tea.Cmd {
	if idx < 0 || idx >= len(entries) {
		return nil
	}
	e := entries[idx]
	if isSection(e) {
		return nil
	}
	switch e.id {
	case "action_sites":
		m.sitesScroll = 0
		m.sitesBack = stateSettings
		m.state = stateSites
		return nil
	case "action_reset":
		m.cfg.resetToDefaults()
		m.saveConfig()
		m.rebuildTheme()
		m.setToast(tr(m.cfg, "toast_reset_ok"), false)
		return nil
	case "action_save":
		m.saveConfig()
		m.state = stateMenu
		m.setToast(tr(m.cfg, "toast_saved"), false)
		return nil
	case "dir":
		m.editSetting(e)
		return nil
	case "format", "throttled_rate", "limit_rate", "proxy", "cookies_file",
		"extractor_args", "user_agent", "referer":
		m.editSetting(e)
		return nil
	}
	m.changeSetting(entries, idx, 1)
	return nil
}

// editSetting 打开文本项的单行编辑弹层。
func (m *model) editSetting(e settingsEntry) tea.Cmd {
	c := m.cfg
	var value string
	switch e.id {
	case "format":
		value = c.Format
		if strings.TrimSpace(value) == "" {
			value = c.effectiveFormat()
		}
	case "throttled_rate":
		value = c.ThrottledRate
	case "limit_rate":
		value = c.LimitRate
	case "proxy":
		value = c.Proxy
	case "cookies_file":
		value = c.CookiesFile
	case "extractor_args":
		value = c.ExtractorArgs
	case "user_agent":
		value = c.UserAgent
	case "referer":
		value = c.Referer
	case "dir":
		value = expandHome(c.DownloadDir)
	}
	p := &editPrompt{
		titleKey: e.labelKey,
		descKey:  "edit_hint",
		value:    value,
	}
	switch e.id {
	case "dir":
		p.onSave = func(v string) {
			v = expandHome(v)
			if v == "" {
				m.setToast(tr(m.cfg, "toast_dir_empty"), true)
				return
			}
			if !strings.HasPrefix(v, "/") {
				m.setToast(tr(m.cfg, "input_dir_invalid"), true)
				return
			}
			if err := ensureDir(v); err != nil {
				m.setToast(tr(m.cfg, "toast_dir_fail")+": "+oneLine(err.Error()), true)
				return
			}
			m.cfg.DownloadDir = v
			m.saveConfig()
			m.setToast(tr(m.cfg, "toast_dir_ok")+": "+v, false)
		}
	case "format":
		p.onSave = func(v string) {
			m.cfg.Format = strings.TrimSpace(v)
			m.cfg.FormatPreset = "custom"
			m.cfg.normalize()
			m.saveConfig()
		}
	default:
		setter := m.stringSetter(e.id)
		p.onSave = func(v string) {
			setter(strings.TrimSpace(v))
			m.cfg.normalize()
			m.saveConfig()
		}
	}
	return m.openEdit(p)
}

// stringSetter 返回某个文本项的赋值函数。
func (m *model) stringSetter(id string) func(string) {
	c := m.cfg
	switch id {
	case "throttled_rate":
		return func(v string) { c.ThrottledRate = v }
	case "limit_rate":
		return func(v string) { c.LimitRate = v }
	case "proxy":
		return func(v string) { c.Proxy = v }
	case "cookies_file":
		return func(v string) { c.CookiesFile = v }
	case "extractor_args":
		return func(v string) { c.ExtractorArgs = v }
	case "user_agent":
		return func(v string) { c.UserAgent = v }
	case "referer":
		return func(v string) { c.Referer = v }
	}
	return func(string) {}
}

// ---------- 渲染 ----------

func (m *model) viewSettings() ([]string, []keyHint) {
	th := m.theme
	width := m.frame.contentWidth()
	entries := settingsEntries()
	m.clampSettingsCursor(entries)

	labelW := 22
	valueW := 30
	rows := make([]string, len(entries))
	for i, e := range entries {
		if isSection(e) {
			if e.labelKey == "" {
				rows[i] = ruleRow(th, width)
			} else {
				rows[i] = sectionHeader(th, tr(m.cfg, e.labelKey), width)
			}
			continue
		}
		sel := i == m.settingsCursor
		labelStyle := th.TextStyle
		if sel {
			labelStyle = th.Selected
		}
		value := m.settingValue(e)
		valueStyle := th.AccentText
		label := padWidth(truncateWidth(tr(m.cfg, e.labelKey), labelW), labelW)
		switch {
		case strings.HasPrefix(e.id, "action_"):
			valueStyle = th.InfoText
		case e.id == "dir":
			valueStyle = th.TextStyle
		}
		row := marker(th, sel) + labelStyle.Render(label) + " " +
			valueStyle.Render(clipStyled(value, valueW))
		rows[i] = clipStyled(row, width)
	}

	bodyH := m.frame.contentHeight()
	visible := maxInt(bodyH-1, 3)
	m.settingsScroll = keepVisible(m.settingsScroll, m.settingsCursor, visible, len(rows))
	window, offset := scrollWindow(rows, m.settingsScroll, visible)
	m.settingsScroll = offset

	body := append([]string{}, window...)
	if below := len(rows) - (offset + len(window)); below > 0 {
		body = append(body, th.Dim.Render("  "+tr(m.cfg, "more_items", below)))
	}

	hints := []keyHint{
		{"↑↓", "help_key_move"},
		{"←→", "help_key_toggle"},
		{"Enter", "help_key_edit"},
		{"Esc", "help_key_save"},
	}
	return body, hints
}
