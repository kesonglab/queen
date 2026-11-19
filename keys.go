package main

import (
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// key 是各页面的键位分发中心。
func (m *model) key(msg tea.KeyMsg) tea.Cmd {
	k := msg.String()

	// 全局键
	switch k {
	case "ctrl+c":
		if m.batchActive && m.state == stateDownload {
			m.abortBatch()
		}
		return tea.Quit
	case "ctrl+t":
		m.cfg.Theme = nextThemeName(m.cfg.Theme)
		m.rebuildTheme()
		m.saveConfig()
		m.setToast(tr(m.cfg, "toast_theme")+": "+themeLabel(m.cfg, m.cfg.Theme), false)
		return nil
	case "?":
		if m.state != stateInput {
			m.showHelp = true
			m.helpScroll = 0
			return nil
		}
	}

	switch m.state {
	case stateMenu:
		return m.menuKey(k)
	case stateInput:
		return m.inputKey(msg, k)
	case stateSettings:
		return m.settingsKey(k)
	case stateSites:
		return m.sitesKey(k)
	case stateDownload:
		return m.downloadKey(k)
	case stateSummary:
		return m.summaryKey(k)
	case stateUpdate:
		return m.updateKey(k)
	case stateChangelog:
		return m.changelogKey(k)
	}
	return nil
}

// ---------- 主菜单 ----------

func (m *model) menuKey(k string) tea.Cmd {
	n := len(menuItems)
	switch k {
	case "up", "k", "shift+tab":
		m.menuCursor = (m.menuCursor - 1 + n) % n
	case "down", "j", "tab":
		m.menuCursor = (m.menuCursor + 1) % n
	case "enter", " ":
		return m.menuAction(m.menuCursor)
	case "q":
		return tea.Quit
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		for i := range menuItems {
			if menuItems[i].num == k {
				m.menuCursor = i
				return m.menuAction(i)
			}
		}
	}
	return nil
}

func (m *model) menuAction(idx int) tea.Cmd {
	if idx < 0 || idx >= len(menuItems) {
		return nil
	}
	switch menuItems[idx].id {
	case "paste":
		m.inputMode = inputURLs
		m.input.SetValue("")
		m.input.Placeholder = tr(m.cfg, "input_placeholder")
		m.input.Focus()
		m.state = stateInput
		return m.input.Focus()
	case "clipboard":
		out, err := clipboardRead()
		if err != nil {
			m.setToast(tr(m.cfg, "toast_clip_fail"), true)
			return nil
		}
		urls := parseURLs(out)
		if len(urls) == 0 {
			m.setToast(tr(m.cfg, "toast_no_urls"), true)
			return nil
		}
		m.setToast(tr(m.cfg, "toast_clip_ok", len(urls)), false)
		return m.startDownload(urls)
	case "retry":
		urls := readRetry(m.cfg.DownloadDir)
		if len(urls) == 0 {
			m.setToast(tr(m.cfg, "toast_no_retry"), true)
			return nil
		}
		m.setToast(tr(m.cfg, "toast_retry_ok", len(urls)), false)
		return m.startDownload(urls)
	case "settings":
		m.settingsCursor = 0
		m.settingsScroll = 0
		m.state = stateSettings
		return nil
	case "sites":
		m.sitesScroll = 0
		m.sitesBack = stateMenu
		m.state = stateSites
		return nil
	case "opendir":
		m.openDownloadDir()
		return nil
	case "update":
		return m.startUpdate()
	case "changelog":
		m.chgScroll = 0
		m.chgLines = nil
		m.state = stateChangelog
		return nil
	case "quit":
		return tea.Quit
	}
	return nil
}

// ---------- 输入 ----------

func (m *model) inputKey(msg tea.KeyMsg, k string) tea.Cmd {
	switch k {
	case "esc":
		if m.inputMode == inputDir {
			m.state = stateSettings
		} else {
			m.state = stateMenu
		}
		return nil
	case "ctrl+d":
		return m.submitInput()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return cmd
}

func (m *model) submitInput() tea.Cmd {
	value := strings.TrimSpace(m.input.Value())
	if m.inputMode == inputDir {
		dir := expandHome(value)
		if dir == "" {
			m.setToast(tr(m.cfg, "toast_dir_empty"), true)
			return nil
		}
		if !strings.HasPrefix(dir, "/") {
			m.setToast(tr(m.cfg, "input_dir_invalid"), true)
			return nil
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			m.setToast(tr(m.cfg, "toast_dir_fail")+": "+oneLine(err.Error()), true)
			return nil
		}
		m.cfg.DownloadDir = dir
		m.saveConfig()
		m.state = stateSettings
		m.setToast(tr(m.cfg, "toast_dir_ok")+": "+dir, false)
		return nil
	}
	urls := parseURLs(value)
	if len(urls) == 0 {
		m.setToast(tr(m.cfg, "toast_no_urls"), true)
		return nil
	}
	m.state = stateMenu
	return m.startDownload(urls)
}

// ---------- 设置 ----------

func (m *model) settingsKey(k string) tea.Cmd {
	entries := settingsEntries()
	switch k {
	case "esc":
		m.saveConfig()
		m.state = stateMenu
		return nil
	case "up", "k":
		m.moveSettingsCursor(entries, -1)
	case "down", "j":
		m.moveSettingsCursor(entries, 1)
	case "pgup":
		m.moveSettingsCursor(entries, -5)
	case "pgdown":
		m.moveSettingsCursor(entries, 5)
	case "g":
		m.settingsCursor = m.firstSettingsIndex(entries)
	case "G":
		m.settingsCursor = m.lastSettingsIndex(entries)
	case "left", "h":
		m.changeSetting(entries, m.settingsCursor, -1)
	case "right", "l", " ":
		m.changeSetting(entries, m.settingsCursor, 1)
	case "enter":
		return m.activateSetting(entries, m.settingsCursor)
	}
	m.clampSettingsCursor(entries)
	return nil
}

// ---------- 站点档案 ----------

func (m *model) sitesKey(k string) tea.Cmd {
	total := len(m.db.List())
	switch k {
	case "esc", "q", "enter":
		m.state = m.sitesBack
		if m.state != stateSettings {
			m.state = stateMenu
		}
	case "up", "k":
		m.sitesScroll--
	case "down", "j":
		m.sitesScroll++
	case "pgup":
		m.sitesScroll -= 10
	case "pgdown":
		m.sitesScroll += 10
	case "g":
		m.sitesScroll = 0
	case "G":
		m.sitesScroll = total
	}
	m.sitesScroll = clampScroll(m.sitesScroll, total, maxInt(m.frame.contentHeight()-4, 1))
	return nil
}

// ---------- 下载 ----------

func (m *model) downloadKey(k string) tea.Cmd {
	switch k {
	case "q", "esc":
		m.abortBatch()
		return nil
	case "up", "k":
		m.moveTaskCursor(-1)
	case "down", "j":
		m.moveTaskCursor(1)
	case "pgup":
		m.moveTaskCursor(-5)
	case "pgdown":
		m.moveTaskCursor(5)
	case "g":
		m.taskCursor = 0
	case "G":
		m.taskCursor = maxInt(len(m.filteredTasks())-1, 0)
	case "f":
		m.filter = (m.filter + 1) % 3
		m.taskCursor = 0
		m.taskScroll = 0
	case "x":
		tasks := m.filteredTasks()
		if m.taskCursor >= 0 && m.taskCursor < len(tasks) && m.dl != nil {
			m.dl.CancelTask(tasks[m.taskCursor].Index)
		}
	}
	return nil
}

// moveTaskCursor 移动任务光标（跳过过滤后的列表）。
func (m *model) moveTaskCursor(delta int) {
	n := len(m.filteredTasks())
	if n == 0 {
		m.taskCursor = 0
		return
	}
	m.taskCursor = clampInt(m.taskCursor+delta, 0, n-1)
}

// filteredTasks 按当前筛选项返回任务列表。
func (m *model) filteredTasks() []Task {
	out := make([]Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		switch m.filter {
		case filterActive:
			if t.Status.IsTerminal() {
				continue
			}
		case filterFailed:
			if t.Status != StatusFailed && t.Status != StatusAborted {
				continue
			}
		}
		out = append(out, t)
	}
	return out
}

// ---------- 结果页 ----------

func (m *model) summaryKey(k string) tea.Cmd {
	switch k {
	case "enter", "q", "esc":
		m.state = stateMenu
		return nil
	case "up", "k":
		m.summaryScroll--
	case "down", "j":
		m.summaryScroll++
	case "pgup":
		m.summaryScroll -= 10
	case "pgdown":
		m.summaryScroll += 10
	case "g":
		m.summaryScroll = 0
	case "G":
		m.summaryScroll = len(m.tasks)
	case "r":
		urls := m.retryFailedURLs()
		if len(urls) == 0 {
			m.setToast(tr(m.cfg, "summary_no_failures"), false)
			return nil
		}
		m.setToast(tr(m.cfg, "summary_retried"), false)
		return m.startDownload(urls)
	case "o":
		m.openDownloadDir()
		return nil
	case "c":
		m.copyDiagnostics()
		return nil
	case "l":
		m.openLogsDir()
		return nil
	}
	m.summaryScroll = clampScroll(m.summaryScroll, len(m.tasks), maxInt(m.frame.contentHeight()-5, 1))
	return nil
}

// ---------- 更新 ----------

func (m *model) updateKey(k string) tea.Cmd {
	switch k {
	case "esc", "q", "enter":
		if !m.updRunning {
			m.state = stateMenu
		}
	case "up", "k":
		m.updScroll--
	case "down", "j":
		m.updScroll++
	case "pgup":
		m.updScroll -= 10
	case "pgdown":
		m.updScroll += 10
	}
	if m.updScroll < 0 {
		m.updScroll = 0
	}
	return nil
}

// startUpdate 启动 yt-dlp 升级任务。
func (m *model) startUpdate() tea.Cmd {
	if m.updRunning {
		return nil
	}
	m.updRunning = true
	m.updDone = false
	m.updOK = false
	m.updOut = ""
	m.updScroll = 0
	m.state = stateUpdate
	ytdlp := m.ytdlp
	return func() tea.Msg {
		if !ytdlp.OK {
			return updateDoneMsg{out: trLang(m.cfg.Lang, "toast_ytdlp_missing"), ok: false}
		}
		out, ok := runYtdlpUpgrade(ytdlp)
		return updateDoneMsg{out: out, ok: ok}
	}
}

// ---------- 更新日志 ----------

func (m *model) changelogKey(k string) tea.Cmd {
	m.ensureChangelog()
	visible := m.frame.contentHeight() - 2
	switch k {
	case "esc", "q":
		m.state = stateMenu
	case "up", "k":
		m.chgScroll--
	case "down", "j":
		m.chgScroll++
	case "pgup":
		m.chgScroll -= visible
	case "pgdown":
		m.chgScroll += visible
	case "g":
		m.chgScroll = 0
	case "G":
		m.chgScroll = len(m.chgLines)
	}
	m.chgScroll = clampScroll(m.chgScroll, len(m.chgLines), maxInt(visible, 1))
	return nil
}

// ---------- 帮助 ----------

func (m *model) helpKey(msg tea.KeyMsg) tea.Cmd {
	k := msg.String()
	switch k {
	case "esc", "?", "q", "enter":
		m.showHelp = false
	case "up", "k":
		m.helpScroll--
	case "down", "j":
		m.helpScroll++
	case "pgup":
		m.helpScroll -= 10
	case "pgdown":
		m.helpScroll += 10
	case "g":
		m.helpScroll = 0
	case "G":
		m.helpScroll = len(helpLines(m.cfg, m.theme))
	}
	if m.helpScroll < 0 {
		m.helpScroll = 0
	}
	return nil
}

// ---------- 编辑弹层 ----------

func (m *model) editKey(msg tea.KeyMsg) tea.Cmd {
	k := msg.String()
	switch k {
	case "esc":
		m.edit = nil
		return nil
	case "enter":
		if m.edit == nil {
			return nil
		}
		value := strings.TrimSpace(m.editInput.Value())
		if m.edit.validate != nil {
			if msg := m.edit.validate(value); msg != "" {
				m.setToast(msg, true)
				return nil
			}
		}
		save := m.edit.onSave
		m.edit = nil
		if save != nil {
			save(value)
		}
		return nil
	case "ctrl+u":
		m.editInput.SetValue("")
		return nil
	}
	var cmd tea.Cmd
	m.editInput, cmd = m.editInput.Update(msg)
	return cmd
}

// openEdit 打开通用单行编辑弹层。
func (m *model) openEdit(p *editPrompt) tea.Cmd {
	m.edit = p
	m.editInput.SetValue(p.value)
	m.editInput.CursorEnd()
	return m.editInput.Focus()
}

// ---------- 鼠标 ----------

func (m *model) mouseKey(msg tea.MouseMsg) tea.Cmd {
	if msg.Action != tea.MouseActionPress {
		return nil
	}
	delta := 0
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		delta = -3
	case tea.MouseButtonWheelDown:
		delta = 3
	default:
		return nil
	}
	if m.edit != nil {
		return nil
	}
	switch m.state {
	case stateSettings:
		m.moveSettingsCursor(settingsEntries(), delta)
		m.clampSettingsCursor(settingsEntries())
	case stateDownload:
		m.moveTaskCursor(delta)
	case stateSummary:
		m.summaryScroll += delta
	case stateSites:
		m.sitesScroll += delta
		m.sitesScroll = clampScroll(m.sitesScroll, len(m.db.List()), maxInt(m.frame.contentHeight()-4, 1))
	case stateChangelog:
		m.ensureChangelog()
		m.chgScroll += delta
		m.chgScroll = clampScroll(m.chgScroll, len(m.chgLines), maxInt(m.frame.contentHeight()-2, 1))
	case stateUpdate:
		m.updScroll = maxInt(m.updScroll+delta, 0)
	case stateMenu:
		m.menuCursor = clampInt(m.menuCursor+delta, 0, len(menuItems)-1)
	}
	if m.showHelp {
		m.helpScroll = maxInt(m.helpScroll+delta, 0)
	}
	return nil
}
