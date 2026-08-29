package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

//go:embed CHANGELOG.md
var changelogMD string

//go:embed CHANGELOG.zh-CN.md
var changelogMDZh string

// version 由构建时通过 -ldflags "-X main.version=vX.Y.Z" 注入，见 Makefile。
var version = "1.2.1"

type state int

const (
	stMenu state = iota
	stInput
	stSettings
	stDownload
	stSummary
	stUpdate
	stChangelog
)

type inputMode int

const (
	modeURLs inputMode = iota
	modeDir
)

type model struct {
	state state
	cfg   *Config

	width, height int

	// 菜单
	menuCursor int
	toast      string
	toastUntil time.Time

	// 输入
	input     textarea.Model
	inputMode inputMode

	// 设置
	settingsCursor int

	// 下载
	dl         *downloader
	tasks      []*Task
	batch      batchProgressMsg
	batchStart time.Time
	aborted    bool
	sp         spinner.Model

	// 更新 yt-dlp
	updLines strings.Builder
	updDone  bool
	updOK    bool

	// 更新日志滚动
	chgOffset int
}

// ---------- 样式 ----------
var (
	green  = lipgloss.Color("#00ff87")
	blue   = lipgloss.Color("#00a6ff")
	red    = lipgloss.Color("#ff4d4f")
	yellow = lipgloss.Color("#ffcc00")
	gray   = lipgloss.Color("#888888")

	styleTitle  = lipgloss.NewStyle().Foreground(green).Bold(true)
	styleAccent = lipgloss.NewStyle().Foreground(blue).Bold(true)
	styleGray   = lipgloss.NewStyle().Foreground(gray)
	styleRed    = lipgloss.NewStyle().Foreground(red).Bold(true)
	styleYellow = lipgloss.NewStyle().Foreground(yellow)
	styleGreen  = lipgloss.NewStyle().Foreground(green).Bold(true)
	styleSel    = lipgloss.NewStyle().Foreground(green).Bold(true)
	styleDim    = lipgloss.NewStyle().Foreground(gray)
	styleSep    = lipgloss.NewStyle().Foreground(gray).Faint(true)
	styleBox    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(blue).Padding(0, 1)
)

// ---------- 菜单项 ----------
type menuItem struct {
	num      string
	labelKey string
	descKey  string
	run      func(*model) tea.Cmd
}

func (m *model) menuItems() []menuItem {
	return []menuItem{
		{"1", "menu_paste", "menu_paste_d", func(m *model) tea.Cmd {
			m.inputMode = modeURLs
			m.input.SetValue("")
			m.input.Placeholder = tr(m.cfg, "input_placeholder")
			m.input.Focus()
			m.state = stInput
			return m.input.Focus()
		}},
		{"2", "menu_clip", "menu_clip_d", func(m *model) tea.Cmd {
			out, err := runCommand("pbpaste")
			if err != nil {
				m.setToast(tr(m.cfg, "toast_clip_fail") + ": " + err.Error())
				return nil
			}
			urls := parseURLs(out)
			if len(urls) == 0 {
				m.setToast(tr(m.cfg, "toast_no_urls"))
				return nil
			}
			m.setToast(fmt.Sprintf(tr(m.cfg, "toast_clip_ok"), len(urls)))
			return m.startDownload(urls)
		}},
		{"3", "menu_retry", "menu_retry_d", func(m *model) tea.Cmd {
			urls := readRetry()
			if len(urls) == 0 {
				m.setToast(tr(m.cfg, "toast_no_retry"))
				return nil
			}
			m.setToast(fmt.Sprintf(tr(m.cfg, "toast_retry_ok"), len(urls)))
			return m.startDownload(urls)
		}},
		{"4", "menu_settings", "menu_settings_d", func(m *model) tea.Cmd {
			m.settingsCursor = 0
			m.state = stSettings
			return nil
		}},
		{"5", "menu_opendir", "menu_opendir_d", func(m *model) tea.Cmd {
			cmd := exec.Command("open", m.cfg.DownloadDir)
			if err := cmd.Run(); err != nil {
				m.setToast(tr(m.cfg, "toast_open_fail") + ": " + err.Error())
			} else {
				m.setToast(tr(m.cfg, "toast_open_ok") + ": " + m.cfg.DownloadDir)
			}
			return nil
		}},
		{"6", "menu_update", "menu_update_d", func(m *model) tea.Cmd {
			m.updLines.Reset()
			m.updDone = false
			m.updOK = false
			m.state = stUpdate
			return runUpdateCmd()
		}},
		{"7", "menu_changelog", "menu_changelog_d", func(m *model) tea.Cmd {
			m.chgOffset = 0
			m.state = stChangelog
			return nil
		}},
		{"q", "menu_quit", "menu_quit_d", func(m *model) tea.Cmd {
			return tea.Quit
		}},
	}
}

func (m *model) setToast(s string) {
	m.toast = s
	m.toastUntil = time.Now().Add(4 * time.Second)
}

func parseURLs(s string) []string {
	seen := map[string]bool{}
	var out []string
	for _, line := range strings.Fields(s) {
		line = strings.TrimSpace(line)
		if (strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://")) && !seen[line] {
			seen[line] = true
			out = append(out, line)
		}
	}
	return out
}

func (m *model) startDownload(urls []string) tea.Cmd {
	m.dl = newDownloader(m.cfg, urls)
	m.tasks = make([]*Task, len(urls))
	for i, u := range urls {
		m.tasks[i] = &Task{URL: u, Status: StatusPending}
	}
	m.batch = batchProgressMsg{done: 0, total: len(urls)}
	m.batchStart = time.Now()
	m.aborted = false
	m.sp = spinner.New()
	m.sp.Spinner = spinner.Dot
	m.sp.Style = lipgloss.NewStyle().Foreground(blue)
	m.state = stDownload
	go m.dl.run()
	return tea.Batch(waitForUpdates(m.dl.ch), m.sp.Tick)
}

// runUpdateCmd 执行 yt-dlp -U
func runUpdateCmd() tea.Cmd {
	return func() tea.Msg {
		out, err := runCommand("yt-dlp", "-U")
		if err != nil {
			return updateDoneMsg{ok: false, out: out + "\n" + err.Error()}
		}
		return updateDoneMsg{ok: true, out: out}
	}
}

type updateDoneMsg struct {
	ok  bool
	out string
}

// ---------- Init ----------
func (m model) Init() tea.Cmd {
	return nil
}

// ---------- Update ----------
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	mm := &m
	cmd, _ := mm.update(msg)
	return mm, cmd
}

func (m *model) update(msg tea.Msg) (tea.Cmd, error) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.input.SetWidth(maxInt(msg.Width-4, 10))
		m.input.SetHeight(maxInt(msg.Height/3, 4))
		return nil, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case taskUpdateMsg:
		if msg.idx >= 0 && msg.idx < len(m.tasks) {
			prev := m.tasks[msg.idx]
			t := msg.t
			m.tasks[msg.idx] = &t
			if t.Status == StatusFailed && (prev == nil || prev.Status != StatusFailed) {
				notify(tr(m.cfg, "notify_fail_title"), shortURL(t.URL)+": "+t.Error, "Basso")
			}
		}
		if m.state == stDownload {
			// 关键修复：同时续接 spinner 的 Tick，否则进度静默期（如
			// ffmpeg 合并/嵌入阶段）UI 不会重绘，表现为“卡住不动”。
			return tea.Batch(waitForUpdates(m.dl.ch), m.sp.Tick), nil
		}
		return nil, nil

	case batchProgressMsg:
		m.batch = msg
		if m.state == stDownload {
			return tea.Batch(waitForUpdates(m.dl.ch), m.sp.Tick), nil
		}
		return nil, nil

	case batchDoneMsg:
		if m.state == stDownload {
			m.state = stSummary
			succ, fail := m.countResults()
			m.setToast(fmt.Sprintf(tr(m.cfg, "toast_done"), succ, fail))
			if fail == 0 {
				notify(tr(m.cfg, "notify_done_title"), fmt.Sprintf(tr(m.cfg, "notify_done_ok"), succ), "Glass")
			} else {
				notify(tr(m.cfg, "notify_done_title"), fmt.Sprintf(tr(m.cfg, "notify_done_fail"), succ, fail), "Basso")
			}
		}
		return nil, nil

	case abortMsg:
		m.aborted = true
		m.state = stSummary
		return nil, nil

	case updateDoneMsg:
		m.updDone = true
		m.updOK = msg.ok
		m.updLines.WriteString(msg.out)
		return nil, nil

	case spinner.TickMsg:
		if m.state == stDownload || m.state == stUpdate {
			var cmd tea.Cmd
			m.sp, cmd = m.sp.Update(msg)
			return cmd, nil
		}
		return nil, nil
	}
	return nil, nil
}

func (m *model) handleKey(msg tea.KeyMsg) (tea.Cmd, error) {
	switch m.state {
	case stMenu:
		return m.menuKey(msg)
	case stInput:
		return m.inputKey(msg)
	case stSettings:
		return m.settingsKey(msg)
	case stDownload:
		return m.downloadKey(msg)
	case stSummary:
		if msg.String() == "enter" || msg.String() == " " || msg.String() == "q" || msg.String() == "esc" {
			m.state = stMenu
		}
		return nil, nil
	case stUpdate:
		if msg.String() == "enter" || msg.String() == "q" || msg.String() == "esc" {
			m.state = stMenu
		}
		return nil, nil
	case stChangelog:
		return m.changelogKey(msg)
	}
	return nil, nil
}

func (m *model) menuKey(msg tea.KeyMsg) (tea.Cmd, error) {
	items := m.menuItems()
	switch msg.String() {
	case "ctrl+c", "q":
		return tea.Quit, nil
	case "up", "k", "shift+tab":
		m.menuCursor = (m.menuCursor - 1 + len(items)) % len(items)
	case "down", "j", "tab":
		m.menuCursor = (m.menuCursor + 1) % len(items)
	case "enter", " ":
		return items[m.menuCursor].run(m), nil
	case "1", "2", "3", "4", "5", "6", "7":
		for _, item := range items {
			if item.num == msg.String() {
				return item.run(m), nil
			}
		}
	}
	return nil, nil
}

func (m *model) inputKey(msg tea.KeyMsg) (tea.Cmd, error) {
	switch msg.String() {
	case "ctrl+c":
		return tea.Quit, nil
	case "esc":
		m.state = stMenu
		return nil, nil
	case "ctrl+d":
		if m.inputMode == modeDir {
			dir := strings.TrimSpace(m.input.Value())
			dir = expandHome(dir)
			if dir == "" {
				m.setToast(tr(m.cfg, "toast_dir_empty"))
			} else {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					m.setToast(tr(m.cfg, "toast_dir_fail") + ": " + err.Error())
				} else {
					m.cfg.DownloadDir = dir
					m.cfg.save()
					m.setToast(tr(m.cfg, "toast_dir_ok") + ": " + dir)
				}
			}
			m.state = stSettings
			return nil, nil
		}
		urls := parseURLs(m.input.Value())
		if len(urls) == 0 {
			m.setToast(tr(m.cfg, "toast_no_urls"))
			return nil, nil
		}
		return m.startDownload(urls), nil
	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return cmd, nil
	}
}

func (m *model) settingsKey(msg tea.KeyMsg) (tea.Cmd, error) {
	items := m.settingsItems()
	switch msg.String() {
	case "ctrl+c":
		return tea.Quit, nil
	case "esc":
		m.cfg.save()
		m.state = stMenu
		return nil, nil
	case "enter":
		if m.settingsCursor == len(items)-1 {
			m.cfg.save()
			m.state = stMenu
			return nil, nil
		}
		if m.settingsCursor == 7 {
			return m.editDir(), nil
		}
	case "up", "k":
		m.settingsCursor = (m.settingsCursor - 1 + len(items)) % len(items)
	case "down", "j":
		m.settingsCursor = (m.settingsCursor + 1) % len(items)
	case "left", "h":
		m.toggleSetting(m.settingsCursor, -1)
	case "right", "l", " ":
		m.toggleSetting(m.settingsCursor, 1)
	}
	return nil, nil
}

func (m *model) editDir() tea.Cmd {
	m.inputMode = modeDir
	m.input.SetValue(m.cfg.DownloadDir)
	m.input.Placeholder = tr(m.cfg, "input_dir_placeholder")
	m.input.Focus()
	m.state = stInput
	return m.input.Focus()
}

func (m *model) downloadKey(msg tea.KeyMsg) (tea.Cmd, error) {
	switch msg.String() {
	case "ctrl+c":
		// 先中止子进程再退出，避免残留 yt-dlp/ffmpeg 僵尸进程
		if m.dl != nil {
			m.dl.abort()
		}
		return tea.Quit, nil
	case "q", "esc":
		if m.dl != nil {
			m.dl.abort()
			m.aborted = true
			m.state = stSummary
		}
		return nil, nil
	}
	return nil, nil
}

func (m *model) changelogKey(msg tea.KeyMsg) (tea.Cmd, error) {
	switch msg.String() {
	case "ctrl+c":
		return tea.Quit, nil
	case "esc", "q":
		m.state = stMenu
		return nil, nil
	case "up", "k":
		if m.chgOffset > 0 {
			m.chgOffset--
		}
	case "down", "j":
		lines := strings.Split(changelogMD, "\n")
		visible := maxInt(m.height-6, 5)
		maxOff := maxInt(len(lines)-visible, 0)
		if m.chgOffset < maxOff {
			m.chgOffset++
		}
	}
	return nil, nil
}

// ---------- 设置 ----------
type settingItem struct {
	labelKey string
	value    string
	action   bool // 是否按下 Enter 触发操作（编辑目录）
}

func (m *model) settingsItems() []settingItem {
	c := m.cfg
	return []settingItem{
		{"setting_playlist", yesno(c, c.Playlist), false},
		{"setting_audio", yesno(c, c.AudioOnly), false},
		{"setting_subs", yesno(c, c.Subs), false},
		{"setting_embed", yesno(c, c.Embed), false},
		{"setting_cookie", cookieName(c, c.CookieBrowser), false},
		{"setting_retry", fmt.Sprintf("%d %s", c.RetryTimes, tr(c, "times")), false},
		{"setting_concurrency", fmt.Sprintf("%d", c.Concurrency), false},
		{"setting_dir", c.DownloadDir, true},
		{"setting_lang", langName(c.Lang), false},
		{"setting_save", "", false},
	}
}

func (m *model) toggleSetting(idx, delta int) {
	c := m.cfg
	switch idx {
	case 0:
		c.Playlist = !c.Playlist
	case 1:
		c.AudioOnly = !c.AudioOnly
	case 2:
		c.Subs = !c.Subs
	case 3:
		c.Embed = !c.Embed
	case 4:
		c.CookieBrowser = cycleCookie(c.CookieBrowser, delta)
	case 5:
		c.RetryTimes = (c.RetryTimes + delta + 6) % 6
	case 6:
		c.Concurrency = c.Concurrency + delta
		// 左右键调到边界时也要 clamp，否则会出现 0/负数并发
		if c.Concurrency < 1 {
			c.Concurrency = 1
		}
		if c.Concurrency > 16 {
			c.Concurrency = 16
		}
	case 7:
		// 下载目录为动作项，左右键不处理
	case 8:
		c.Lang = nextLang(c.Lang)
	}
}

func yesno(cfg *Config, b bool) string {
	if b {
		return tr(cfg, "on")
	}
	return tr(cfg, "off")
}

func cookieName(cfg *Config, s string) string {
	if s == "" {
		return tr(cfg, "cookie_auto")
	}
	return s
}

func langName(code string) string {
	for _, l := range languages {
		if l.code == code {
			return l.name
		}
	}
	return code
}

func cycleCookie(cur string, delta int) string {
	opts := []string{"", "chrome", "firefox", "safari", "edge"}
	idx := 0
	for i, o := range opts {
		if o == cur {
			idx = i
		}
	}
	idx = (idx + delta + len(opts)) % len(opts)
	return opts[idx]
}

// ---------- 渲染 ----------
func (m *model) View() string {
	switch m.state {
	case stMenu:
		return m.menuView()
	case stInput:
		return m.inputView()
	case stSettings:
		return m.settingsView()
	case stDownload:
		return m.downloadView()
	case stSummary:
		return m.summaryView()
	case stUpdate:
		return m.updateView()
	case stChangelog:
		return m.changelogView()
	}
	return ""
}

func (m *model) banner() string {
	w := maxInt(m.width, 50)
	sep := styleSep.Render(strings.Repeat("━", w))
	title := styleTitle.Render("  👑 " + tr(m.cfg, "app_name") + " v" + version + "  ")
	author := styleGray.Render(tr(m.cfg, "author"))
	return fmt.Sprintf("%s\n%s\n%s", sep, title+author, sep)
}

func (m *model) menuView() string {
	b := m.banner()
	var rows []string
	items := m.menuItems()
	for i, item := range items {
		sel := i == m.menuCursor
		marker := "  "
		if sel {
			marker = styleGreen.Render("▶ ")
		}
		key := styleAccent.Render("[" + item.num + "]")
		label := tr(m.cfg, item.labelKey)
		if sel {
			label = styleSel.Render(label)
		}
		desc := styleDim.Render(tr(m.cfg, item.descKey))
		rows = append(rows, marker+key+" "+label+"  "+desc)
	}
	menu := strings.Join(rows, "\n")
	footer := styleDim.Render(tr(m.cfg, "menu_nav"))
	var toast string
	if time.Now().Before(m.toastUntil) && m.toast != "" {
		toast = "\n\n" + styleYellow.Render("  ℹ "+m.toast)
	}
	return b + "\n\n" + menu + "\n\n" + footer + toast
}

func (m *model) inputView() string {
	b := m.banner()
	box := styleBox.Width(maxInt(minInt(m.width-2, 80), 30)).Render(m.input.View())
	clue := tr(m.cfg, "input_url_hint")
	if m.inputMode == modeDir {
		clue = tr(m.cfg, "input_dir_hint")
	}
	footer := styleDim.Render(clue)
	return b + "\n\n" + box + "\n\n" + footer
}

func (m *model) settingsView() string {
	b := m.banner()
	items := m.settingsItems()
	var rows []string
	for i, it := range items {
		sel := i == m.settingsCursor
		marker := "  "
		if sel {
			marker = styleGreen.Render("▶ ")
		}
		label := tr(m.cfg, it.labelKey)
		if sel {
			label = styleSel.Render(label)
		}
		val := styleAccent.Render(it.value)
		rows = append(rows, marker+label+" : "+val)
	}
	footer := styleDim.Render(tr(m.cfg, "setting_nav"))
	return b + "\n\n" + strings.Join(rows, "\n") + "\n\n" + footer
}

// 定宽对齐，防止进度数值变化时文字跳动
func (m *model) downloadView() string {
	b := m.banner()
	var rows []string
	pending := 0
	for _, t := range m.tasks {
		if t == nil {
			continue
		}
		if t.Status == StatusPending {
			pending++
			continue
		}
		rows = append(rows, m.taskRow(t))
	}
	if pending > 0 {
		rows = append(rows, styleDim.Render("  "+tr(m.cfg, "waiting")+" × "+strconv.Itoa(pending)))
	}
	list := strings.Join(rows, "\n")

	// 批量进度
	bp := m.batch
	pct := 0
	if bp.total > 0 {
		pct = bp.done * 100 / bp.total
	}
	batchBar := renderBar(pct, 40)
	eta := 0
	if bp.done > 0 {
		// 用浮点计算平均耗时，避免单任务时整数除法把 avg 截断成 0
		avg := time.Since(m.batchStart).Seconds() / float64(bp.done)
		eta = int(avg * float64(bp.total-bp.done))
	}
	batchLine := fmt.Sprintf("  %s %s %3d%%  (%d/%d)",
		tr(m.cfg, "batch_progress"), batchBar, pct, bp.done, bp.total)
	batchLine += "\n" + styleGray.Render("  "+tr(m.cfg, "batch_elapsed")+" "+fmtSec(int(time.Since(m.batchStart).Seconds()))+
		" | "+tr(m.cfg, "batch_eta")+" "+fmtSec(eta)+" | "+tr(m.cfg, "batch_abort"))

	return b + "\n\n" + list + "\n\n" + batchLine
}

func (m *model) taskRow(t *Task) string {
	var line string
	switch t.Status {
	case StatusPending:
		line = "  ⏳ " + styleDim.Render(shortURL(t.URL)) + "  " + styleDim.Render(tr(m.cfg, "waiting"))
	case StatusDownloading:
		sp := m.sp.View()
		bar := renderBar(int(t.Percent), 24)
		// 固定宽度：百分比5.1f、大小11、速度11、ETA8 —— 防止跳动
		info := fmt.Sprintf("%s %5.1f%% %-11s | %s %-11s | %s %8s",
			styleAccent.Render(bar),
			t.Percent,
			styleGray.Render(padW(t.Downloaded, 11)),
			tr(m.cfg, "speed"),
			styleGray.Render(padW(t.Speed, 11)),
			tr(m.cfg, "eta"),
			styleGray.Render(padW(t.ETA, 8)),
		)
		line = "  " + sp + " " + shortURL(t.URL) + "\n      " + info
	case StatusDone:
		line = "  " + styleGreen.Render("✓ ") + shortURL(t.URL)
	case StatusFailed:
		line = "  " + styleRed.Render("✗ ") + shortURL(t.URL)
		if t.Error != "" {
			line += "\n      " + styleRed.Render(t.Error)
		}
	}
	return line
}

// padW 按显示宽度补齐到 n 列（英文空格填充）
func padW(s string, n int) string {
	w := lipgloss.Width(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}

func shortURL(u string) string {
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	r := []rune(u)
	if len(r) > 60 {
		u = string(r[:57]) + "..."
	}
	return u
}

func renderBar(pct, width int) string {
	if pct > 100 {
		pct = 100
	}
	filled := pct * width / 100
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func (m *model) summaryView() string {
	b := m.banner()
	succ, fail := m.countResults()
	rows := []string{styleTitle.Render("  ── " + tr(m.cfg, "download_title") + " ──")}
	if m.aborted {
		rows = append(rows, styleYellow.Render("  "+tr(m.cfg, "download_aborted")))
	}
	rows = append(rows, fmt.Sprintf("  %s %s   %s %s",
		tr(m.cfg, "download_success"), styleGreen.Render(fmt.Sprint(succ)),
		tr(m.cfg, "download_fail"), styleRed.Render(fmt.Sprint(fail))))
	if fail > 0 {
		rows = append(rows, styleGray.Render("  "+tr(m.cfg, "download_retry_file")+": "+retryFile()))
	}
	footer := styleDim.Render(tr(m.cfg, "download_summary_nav"))
	return b + "\n\n" + strings.Join(rows, "\n") + "\n\n" + footer
}

func (m *model) countResults() (int, int) {
	succ, fail := 0, 0
	for _, t := range m.tasks {
		if t == nil {
			continue
		}
		switch t.Status {
		case StatusDone:
			succ++
		case StatusFailed:
			fail++
		}
	}
	return succ, fail
}

func (m *model) updateView() string {
	b := m.banner()
	rows := []string{styleTitle.Render("  ── " + tr(m.cfg, "updating_title") + " ──")}
	rows = append(rows, m.updLines.String())
	if m.updDone {
		if m.updOK {
			rows = append(rows, styleGreen.Render("  "+tr(m.cfg, "update_ok")))
		} else {
			rows = append(rows, styleYellow.Render("  "+tr(m.cfg, "update_brew")))
		}
		rows = append(rows, styleDim.Render("  "+tr(m.cfg, "update_nav")))
	} else {
		rows = append(rows, m.sp.View()+"  "+tr(m.cfg, "updating"))
	}
	return b + "\n\n" + strings.Join(rows, "\n")
}

func (m *model) changelogView() string {
	b := m.banner()
	content := changelogMD
	if m.cfg.Lang == "zh" {
		content = changelogMDZh
	}
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	title := styleTitle.Render("  ── " + tr(m.cfg, "changelog_title") + " ──")
	visible := maxInt(m.height-6, 5)
	maxOff := maxInt(len(lines)-visible, 0)
	if m.chgOffset > maxOff {
		m.chgOffset = maxOff
	}
	end := minInt(len(lines), m.chgOffset+visible)
	start := m.chgOffset
	var body []string
	for i := start; i < end; i++ {
		body = append(body, lines[i])
	}
	footer := styleDim.Render(tr(m.cfg, "changelog_nav"))
	return b + "\n\n" + title + "\n\n" + strings.Join(body, "\n") + "\n\n" + footer
}

// ---------- 辅助 ----------
func fmtSec(s int) string {
	if s < 0 {
		s = 0
	}
	h := s / 3600
	s %= 3600
	mm := s / 60
	s %= 60
	return fmt.Sprintf("%d:%02d:%02d", h, mm, s)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ---------- 依赖检测 ----------
func checkDeps(cfg *Config) bool {
	missing := []string{}
	if !hasCmd("yt-dlp") {
		missing = append(missing, "yt-dlp")
	}
	if !hasCmd("ffmpeg") {
		missing = append(missing, "ffmpeg")
	}
	if len(missing) == 0 {
		return true
	}
	fmt.Println(styleRed.Render(tr(cfg, "dep_missing")+": "+strings.Join(missing, ", ")))
	if !hasCmd("brew") {
		fmt.Println(styleRed.Render(tr(cfg, "dep_no_brew")))
		return false
	}
	fmt.Print(styleYellow.Render(tr(cfg, "dep_install_q") + " "))
	var ans string
	_, err := fmt.Scanln(&ans)
	if err != nil || (strings.ToLower(ans) != "y" && strings.ToLower(ans) != "yes") {
		return false
	}
	cmd := exec.Command("brew", append([]string{"install"}, missing...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Println(styleRed.Render(tr(cfg, "dep_install_fail")+" "+strings.Join(missing, " ")))
		return false
	}
	return true
}

// ---------- main ----------
func main() {
	cfg := loadConfig()
	if !checkDeps(cfg) {
		fmt.Println(styleDim.Render(tr(cfg, "dep_retry_hint")))
		os.Exit(1)
	}
	fmt.Println(styleGreen.Render("✓ " + tr(cfg, "dep_installed")))

	m := &model{
		state: stMenu,
		cfg:   cfg,
		input: textarea.New(),
		sp:    spinner.New(),
	}
	m.input.Placeholder = tr(cfg, "input_placeholder")
	m.input.CharLimit = 0
	m.input.ShowLineNumbers = false
	m.input.SetWidth(78)
	m.input.SetHeight(8)
	m.input.Focus()
	m.sp.Spinner = spinner.Dot
	m.sp.Style = lipgloss.NewStyle().Foreground(blue)

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Println("program error:", err)
		os.Exit(1)
	}
}