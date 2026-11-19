package main

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// newTestModel 构造一个尺寸固定的模型（不启动 TUI）。
func newTestModel(t *testing.T, w, h int) *model {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	cfg := defaultConfig()
	cfg.DownloadDir = "/tmp/queen-test-downloads"
	m := newModel(cfg, defaultSiteDB(), YtdlpInfo{OK: true, Version: "2026.10.26", Path: "/opt/homebrew/bin/yt-dlp"}, nil)
	m.width, m.height = w, h
	m.frame = newFrame(w, h)
	return m
}

// assertWidths 断言每行显示宽度不超过终端宽度。
func assertWidths(t *testing.T, name, out string, width int) {
	t.Helper()
	for i, line := range strings.Split(out, "\n") {
		if got := displayWidth(line); got > width {
			t.Errorf("%s: line %d width %d > %d: %q", name, i, got, width, line)
		}
	}
}

func assertContains(t *testing.T, name, out, want string) {
	t.Helper()
	if !strings.Contains(out, want) {
		t.Errorf("%s: output should contain %q\n%s", name, want, out)
	}
}

func TestViewAllStatesRenderWithinBounds(t *testing.T) {
	for _, sz := range []struct{ w, h int }{{80, 24}, {100, 30}, {60, 16}} {
		m := newTestModel(t, sz.w, sz.h)

		// 菜单
		m.state = stateMenu
		out := m.View()
		assertWidths(t, "menu", out, sz.w)
		assertContains(t, "menu", out, "Queen")
		assertContains(t, "menu", out, tr(m.cfg, "menu_paste"))

		// 设置
		m.state = stateSettings
		out = m.View()
		assertWidths(t, "settings", out, sz.w)
		assertContains(t, "settings", out, tr(m.cfg, "settings_section_content"))
		assertContains(t, "settings", out, tr(m.cfg, "setting_playlist"))

		// 输入
		m.state = stateInput
		m.inputMode = inputURLs
		out = m.View()
		assertWidths(t, "input", out, sz.w)
		assertContains(t, "input", out, "Ctrl+D")

		// 站点档案
		m.state = stateSites
		out = m.View()
		assertWidths(t, "sites", out, sz.w)
		assertContains(t, "sites", out, "Bilibili")
		if sz.w >= 90 {
			assertContains(t, "sites", out, "Rule34Video")
		}

		// 更新日志
		m.state = stateChangelog
		out = m.View()
		assertWidths(t, "changelog", out, sz.w)
		if strings.TrimSpace(out) == "" {
			t.Error("changelog view should not be empty")
		}

		// 帮助
		m.showHelp = true
		out = m.View()
		assertWidths(t, "help", out, sz.w)
		assertContains(t, "help", out, tr(m.cfg, "help_title"))
		m.showHelp = false

		// 下载（含进行中/失败/等待任务）
		m.state = stateDownload
		m.tasks = []Task{
			{Index: 0, URL: "https://example.com/a", Title: "视频 A", Status: StatusDownloading, Percent: 42, DownloadedBytes: 4 << 20, TotalBytes: 10 << 20, SpeedBps: 1 << 20, ETASec: 6, Phase: phaseDownloading},
			{Index: 1, URL: "https://example.com/b", Status: StatusFailed, ErrKind: KindAuth, ErrDetail: "Sign in required"},
			{Index: 2, URL: "https://example.com/c", Status: StatusPending},
		}
		m.batchStart = time.Now()
		out = m.View()
		assertWidths(t, "download", out, sz.w)
		assertContains(t, "download", out, "视频 A")

		// 结果页
		m.state = stateSummary
		out = m.View()
		assertWidths(t, "summary", out, sz.w)
		assertContains(t, "summary", out, tr(m.cfg, "summary_total", 3))
		if sz.w >= 90 {
			assertContains(t, "summary", out, tr(m.cfg, "errkind_auth"))
		}

		// 更新页
		m.state = stateUpdate
		m.updDone = true
		m.updOK = true
		out = m.View()
		assertWidths(t, "update", out, sz.w)
		assertContains(t, "update", out, tr(m.cfg, "update_ok"))
	}
}

func TestMarkdownLazyContinuationJoin(t *testing.T) {
	cases := []struct{ in, want string }{
		{"- 中文硬折行，\n  续行内容\n", "中文硬折行，续行内容"},
		{"- English hard-wrapped,\n  continuation here\n", "English hard-wrapped, continuation here"},
	}
	for _, c := range cases {
		lines := RenderMarkdown(c.in, MDStyles{})
		joined := strings.Join(lines, "")
		if !strings.Contains(joined, c.want) {
			t.Errorf("join(%q) = %q, want to contain %q", c.in, joined, c.want)
		}
	}
}

func TestViewTooSmall(t *testing.T) {
	m := newTestModel(t, 50, 14)
	out := m.View()
	if !strings.Contains(out, tr(m.cfg, "term_too_small", 50, 14, minTermWidth, minTermHeight)) {
		t.Errorf("too-small view missing hint:\n%s", out)
	}
}

func TestViewLongTaskListTruncates(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.state = stateDownload
	for i := 0; i < 40; i++ {
		m.tasks = append(m.tasks, Task{
			Index: i, URL: "https://example.com/" + strings.Repeat("x", 30),
			Title:  "很长的视频标题用于测试截断与滚动 " + strings.Repeat("标题", 10),
			Status: StatusDownloading, Percent: float64(i), Phase: phaseDownloading,
		})
	}
	m.batchStart = time.Now()
	out := m.View()
	assertWidths(t, "download-many", out, 80)
	assertContains(t, "download-many", out, "…")
}

func TestMenuNavigationAndActions(t *testing.T) {
	m := newTestModel(t, 100, 30)
	if m.state != stateMenu || m.menuCursor != 0 {
		t.Fatalf("initial state/cursor = %v/%d", m.state, m.menuCursor)
	}
	m.key(tea.KeyMsg{Type: tea.KeyDown})
	if m.menuCursor != 1 {
		t.Errorf("down cursor = %d", m.menuCursor)
	}
	m.key(tea.KeyMsg{Type: tea.KeyUp})
	if m.menuCursor != 0 {
		t.Errorf("up cursor = %d", m.menuCursor)
	}
	// 4 = 设置
	m.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	if m.state != stateSettings {
		t.Errorf("state after '4' = %v", m.state)
	}
	// Esc 保存并返回菜单
	m.key(tea.KeyMsg{Type: tea.KeyEsc})
	if m.state != stateMenu {
		t.Errorf("state after esc = %v", m.state)
	}
}

func TestThemeCycle(t *testing.T) {
	m := newTestModel(t, 100, 30)
	if m.theme.Name != "auto" {
		t.Fatalf("initial theme = %q", m.theme.Name)
	}
	m.key(tea.KeyMsg{Type: tea.KeyCtrlT})
	if m.theme.Name != "dark" {
		t.Errorf("after ctrl+t theme = %q", m.theme.Name)
	}
	m.key(tea.KeyMsg{Type: tea.KeyCtrlT})
	if m.theme.Name != "light" {
		t.Errorf("after second ctrl+t theme = %q", m.theme.Name)
	}
	if m.cfg.Theme != "light" {
		t.Errorf("config theme = %q", m.cfg.Theme)
	}
}

func TestSettingsCursorSkipsSections(t *testing.T) {
	m := newTestModel(t, 100, 30)
	entries := settingsEntries()
	if !isSection(entries[0]) {
		t.Fatal("first entry should be a section")
	}
	m.settingsCursor = 0
	m.moveSettingsCursor(entries, 1)
	if isSection(entries[m.settingsCursor]) {
		t.Errorf("cursor landed on section: %d", m.settingsCursor)
	}
	// 一直向下不会越过边界
	for i := 0; i < 100; i++ {
		m.moveSettingsCursor(entries, 1)
	}
	if m.settingsCursor < 0 || m.settingsCursor >= len(entries) {
		t.Errorf("cursor out of range: %d", m.settingsCursor)
	}
	if entries[m.settingsCursor].id != "action_save" {
		t.Errorf("bottom entry = %q", entries[m.settingsCursor].id)
	}
}

func TestSettingsToggleAndEdit(t *testing.T) {
	m := newTestModel(t, 100, 30)
	entries := settingsEntries()
	find := func(id string) int {
		for i, e := range entries {
			if e.id == id {
				return i
			}
		}
		t.Fatalf("entry %q not found", id)
		return -1
	}
	// 开关
	idx := find("playlist")
	before := m.cfg.Playlist
	m.changeSetting(entries, idx, 1)
	if m.cfg.Playlist == before {
		t.Error("playlist toggle did not change value")
	}
	// 枚举
	idx = find("merge_format")
	beforeMerge := m.cfg.MergeFormat
	m.changeSetting(entries, idx, 1)
	if m.cfg.MergeFormat == beforeMerge {
		t.Error("merge format did not cycle")
	}
	// 文本编辑：Enter 打开编辑弹层
	idx = find("throttled_rate")
	m.activateSetting(entries, idx)
	if m.edit == nil {
		t.Fatal("edit overlay should open for text setting")
	}
	m.editInput.SetValue("250K")
	m.editKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.cfg.ThrottledRate != "250K" {
		t.Errorf("edited value = %q", m.cfg.ThrottledRate)
	}
	// 编辑状态渲染
	m.activateSetting(entries, find("proxy"))
	if !strings.Contains(m.View(), tr(m.cfg, "edit_hint")) {
		t.Error("edit view should render hint")
	}
	m.editKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.edit != nil {
		t.Error("esc should close edit overlay")
	}
}

func TestStartDownloadBlockedWhileBusy(t *testing.T) {
	m := newTestModel(t, 100, 30)
	m.batchActive = true
	cmd := m.startDownload([]string{"https://example.com/a"})
	if cmd != nil {
		t.Error("startDownload should be refused while a batch is active")
	}
	if !strings.Contains(m.toast, tr(m.cfg, "toast_busy")) {
		t.Errorf("busy toast = %q", m.toast)
	}
}

func TestToastExpires(t *testing.T) {
	m := newTestModel(t, 80, 24)
	m.setToast("hello", false)
	if m.toast != "hello" {
		t.Fatal("toast not set")
	}
	m.toastUntil = time.Now().Add(-time.Second)
	m.Update(tickMsg(time.Now()))
	if m.toast != "" {
		t.Errorf("expired toast should be cleared, got %q", m.toast)
	}
}

func TestFilteredTasks(t *testing.T) {
	m := newTestModel(t, 100, 30)
	m.tasks = []Task{
		{Status: StatusDownloading},
		{Status: StatusFailed},
		{Status: StatusDone},
	}
	m.filter = filterActive
	if got := len(m.filteredTasks()); got != 1 {
		t.Errorf("active filter = %d", got)
	}
	m.filter = filterFailed
	if got := len(m.filteredTasks()); got != 1 {
		t.Errorf("failed filter = %d", got)
	}
	m.filter = filterAll
	if got := len(m.filteredTasks()); got != 3 {
		t.Errorf("all filter = %d", got)
	}
}

func TestCompletionKeepsRealErrorText(t *testing.T) {
	// 回归：v1.4.0 的 bug 是把真实错误覆盖成 "exit status 1"
	d, _, _ := setupDownloader(t, []string{"https://example.com/bad"})
	tasks := runDownloader(t, d)
	if tasks[0].Status != StatusFailed {
		t.Fatalf("status = %v", tasks[0].Status)
	}
	if strings.Contains(tasks[0].ErrDetail, "exit status") {
		t.Errorf("error detail was clobbered by exec status: %q", tasks[0].ErrDetail)
	}
	if !strings.Contains(tasks[0].ErrDetail, "Sign in") {
		t.Errorf("expected real yt-dlp error, got %q", tasks[0].ErrDetail)
	}
}
