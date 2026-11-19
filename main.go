package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

//go:embed CHANGELOG.md
var changelogMD string

//go:embed CHANGELOG.zh-CN.md
var changelogMDZh string

// version 是源码内置的发布版本。构建时可用 -ldflags "-X main.version=..." 覆盖
// （Makefile 使用 git describe，CI release 使用去掉 v 前缀的 tag）；versionLabel
// 负责把 v1.6.0 / 1.6.0 / dev 统一成最终展示形式。
var version = "v1.6.0"

// ytdlpStaleDays 超过该天数未更新的 yt-dlp 会在界面上给出提示。
const ytdlpStaleDays = 45

type appState int

const (
	stateMenu appState = iota
	stateInput
	stateSettings
	stateSites
	stateDownload
	stateSummary
	stateUpdate
	stateChangelog
)

type inputMode int

const (
	inputURLs inputMode = iota
	inputDir
)

type taskFilter int

const (
	filterAll taskFilter = iota
	filterActive
	filterFailed
)

// ---------- 消息 ----------

type tickMsg time.Time

type updateDoneMsg struct {
	out string
	ok  bool
}

// editPrompt 是通用单行文本编辑弹层。
type editPrompt struct {
	titleKey string
	descKey  string
	value    string
	onSave   func(string)
	validate func(string) string // 返回非空表示错误提示（已翻译文本）
}

// model 是 Bubble Tea 的全局模型。
type model struct {
	cfg   *Config
	db    *SiteDB
	ytdlp YtdlpInfo
	theme *Theme

	state appState

	width, height int
	frame         frame

	showHelp   bool
	helpScroll int

	toast      string
	toastUntil time.Time
	toastErr   bool

	spin int

	// 主菜单
	menuCursor int

	// 输入
	input     textarea.Model
	inputMode inputMode

	// 设置
	settingsCursor int
	settingsScroll int

	// 编辑弹层
	edit      *editPrompt
	editInput textarea.Model

	// 下载
	dl          *Downloader
	batchID     string
	batchActive bool
	tasks       []Task
	batchStart  time.Time
	aborted     bool
	taskCursor  int
	taskScroll  int
	filter      taskFilter

	// 结果页
	summaryScroll int

	// 更新日志
	chgLines  []string
	chgWidth  int
	chgScroll int

	// 站点档案
	sitesScroll int
	sitesBack   appState

	// 更新
	updOut     string
	updDone    bool
	updOK      bool
	updScroll  int
	updRunning bool
}

// ---------- 构造 ----------

func newModel(cfg *Config, db *SiteDB, ytdlp YtdlpInfo, notices []string) *model {
	m := &model{
		cfg:    cfg,
		db:     db,
		ytdlp:  ytdlp,
		theme:  NewTheme(cfg.Theme),
		state:  stateMenu,
		frame:  newFrame(80, 24),
		width:  80,
		height: 24,
	}

	m.input = textarea.New()
	m.input.Placeholder = tr(cfg, "input_placeholder")
	m.input.CharLimit = 0
	m.input.ShowLineNumbers = false
	m.input.SetWidth(76)
	m.input.SetHeight(6)

	m.editInput = textarea.New()
	m.editInput.CharLimit = 0
	m.editInput.ShowLineNumbers = false
	m.editInput.SetHeight(1)
	m.editInput.SetWidth(40)

	if len(notices) > 0 {
		m.setToast(strings.Join(notices, " · "), false)
	}
	return m
}

func (m *model) Init() tea.Cmd {
	return m.tick()
}

// tick 驱动 spinner 与 toast 过期。空闲时降频，避免无意义重绘。
func (m *model) tick() tea.Cmd {
	d := 400 * time.Millisecond
	if m.batchActive || m.toast != "" || m.updRunning {
		d = 120 * time.Millisecond
	}
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// ---------- Update ----------

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.frame = newFrame(msg.Width, msg.Height)
		m.resizeInputs()
		return m, nil

	case tickMsg:
		m.spin++
		if m.toast != "" && !m.toastUntil.IsZero() && time.Now().After(m.toastUntil) {
			m.toast = ""
		}
		return m, m.tick()

	case tea.KeyMsg:
		// 全局退出：编辑弹层/帮助页也要生效
		if msg.String() == "ctrl+c" {
			if m.batchActive && m.state == stateDownload {
				m.abortBatch()
			}
			return m, tea.Quit
		}
		if m.edit != nil {
			return m, m.editKey(msg)
		}
		if m.showHelp {
			return m, m.helpKey(msg)
		}
		return m, m.key(msg)

	case tea.MouseMsg:
		return m, m.mouseKey(msg)

	case dlTaskMsg:
		if m.dl == nil || msg.Batch != m.batchID {
			return m, nil
		}
		m.applyTask(msg.Task)
		return m, m.waitDL()

	case dlBatchMsg:
		if m.dl == nil || msg.Batch != m.batchID {
			return m, nil
		}
		return m, m.waitDL()

	case dlDoneMsg:
		if m.dl == nil || msg.Batch != m.batchID {
			return m, nil
		}
		m.batchActive = false
		m.state = stateSummary
		m.summaryScroll = 0
		m.setToast(tr(m.cfg, "toast_done", msg.Done, msg.Failed), msg.Failed > 0)
		notifyBatchDone(m.cfg, msg.Done, msg.Failed)
		return m, nil

	case dlClosedMsg:
		if m.batchID != "" && msg.Batch != m.batchID {
			return m, nil
		}
		if m.batchActive {
			m.batchActive = false
			if m.state == stateDownload {
				m.state = stateSummary
				m.summaryScroll = 0
			}
		}
		return m, nil

	case updateDoneMsg:
		m.updOut = msg.out
		m.updDone = true
		m.updOK = msg.ok
		m.updRunning = false
		if msg.ok {
			m.setToast(tr(m.cfg, "update_ok"), false)
			m.ytdlp = detectYtdlp()
		} else {
			m.setToast(tr(m.cfg, "update_fail"), true)
		}
		return m, nil
	}
	return m, nil
}

// waitDL 读取下载器事件通道的一条消息；通道关闭时返回 dlClosedMsg。
func (m *model) waitDL() tea.Cmd {
	if m.dl == nil {
		return nil
	}
	ch := m.dl.Events()
	batch := m.batchID
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return dlClosedMsg{Batch: batch}
		}
		return msg
	}
}

func (m *model) applyTask(t Task) {
	if t.Index < 0 || t.Index >= len(m.tasks) {
		return
	}
	m.tasks[t.Index] = t
	if t.Status == StatusFailed && m.dl != nil {
		// 通知由下载器负责；这里只保证 UI 状态同步
	}
}

// ---------- View ----------

func (m *model) View() string {
	var body []string
	var hints []keyHint

	switch {
	case m.frame.tooSmall():
		return m.viewTooSmall()
	case m.edit != nil:
		body, hints = m.viewEdit()
	case m.showHelp:
		body, hints = m.viewHelp()
	default:
		switch m.state {
		case stateMenu:
			body, hints = m.viewMenu()
		case stateInput:
			body, hints = m.viewInput()
		case stateSettings:
			body, hints = m.viewSettings()
		case stateSites:
			body, hints = m.viewSites()
		case stateDownload:
			body, hints = m.viewDownload()
		case stateSummary:
			body, hints = m.viewSummary()
		case stateUpdate:
			body, hints = m.viewUpdate()
		case stateChangelog:
			body, hints = m.viewChangelog()
		}
	}
	return m.render(body, hints)
}

// render 组合「标题 + 分隔线 + 空行 + 内容 + toast + 状态栏」，并做水平居中。
func (m *model) render(body []string, hints []keyHint) string {
	width := m.frame.contentWidth()
	height := m.height
	if height < 1 {
		height = 1
	}

	toastLine := ""
	if m.toast != "" {
		style := m.theme.Toast
		prefix := "ℹ "
		if m.toastErr {
			style = m.theme.ToastErr
			prefix = "✖ "
		}
		toastLine = style.Render(clipStyled(prefix+m.toast, width))
	}
	toastCount := 0
	if toastLine != "" {
		toastCount = 1
	}

	bodyH := height - headerLines - 1 - toastCount - statusLines
	if bodyH < 0 {
		bodyH = 0
	}

	lines := make([]string, 0, height)
	lines = append(lines, m.headerLines(width)...)
	lines = append(lines, "")
	lines = append(lines, fitLines(body, bodyH, width)...)
	if toastCount > 0 {
		lines = append(lines, toastLine)
	}
	lines = append(lines, m.statusLine(width, hints))
	lines = centerLines(lines, m.frame.leftMargin(), width)
	return strings.Join(lines, "\n")
}

func (m *model) headerLines(width int) []string {
	title := m.theme.AppName.Render("👑 "+tr(m.cfg, "app_name")) + " " +
		m.theme.Dim.Render(versionLabel()) + "  " +
		m.theme.Title.Render(m.subtitle())
	right := m.theme.Dim.Render(m.headerRight())
	return []string{
		renderStatusLine(m.theme, title, right, width),
		m.theme.rule(width),
	}
}

func (m *model) statusLine(width int, hints []keyHint) string {
	left := renderHints(m.theme, m.cfg, hints, maxInt(width-displayWidth(tr(m.cfg, "nav_hint"))-2, 4))
	right := m.theme.StatusBar.Render(tr(m.cfg, "nav_hint"))
	return renderStatusLine(m.theme, left, right, width)
}

func (m *model) subtitle() string {
	switch m.state {
	case stateMenu:
		return tr(m.cfg, "menu_title")
	case stateInput:
		if m.inputMode == inputDir {
			return tr(m.cfg, "input_dir_title")
		}
		return tr(m.cfg, "input_urls_title")
	case stateSettings:
		return tr(m.cfg, "settings_title")
	case stateSites:
		return tr(m.cfg, "sites_title")
	case stateDownload:
		return tr(m.cfg, "download_title")
	case stateSummary:
		return tr(m.cfg, "summary_title")
	case stateUpdate:
		return tr(m.cfg, "update_title")
	case stateChangelog:
		return tr(m.cfg, "changelog_title")
	}
	return ""
}

// headerRight 显示 yt-dlp 版本与并发数；版本过旧时给出警告。
func (m *model) headerRight() string {
	var parts []string
	if m.ytdlp.OK && m.ytdlp.Version != "" {
		v := "yt-dlp " + m.ytdlp.Version
		if m.ytdlp.AgeDays > ytdlpStaleDays {
			v += " ⚠"
		}
		parts = append(parts, v)
	} else {
		parts = append(parts, tr(m.cfg, "toast_ytdlp_missing"))
	}
	parts = append(parts, fmt.Sprintf("%s %d", tr(m.cfg, "menu_info_concurrency"), m.cfg.Concurrency))
	return strings.Join(parts, " · ")
}

func (m *model) viewTooSmall() string {
	msg := tr(m.cfg, "term_too_small", m.width, m.height, minTermWidth, minTermHeight)
	lines := wrapPlain(msg, maxInt(m.width-2, 10))
	pad := maxInt((m.height-len(lines))/2, 0)
	var b strings.Builder
	for i := 0; i < pad; i++ {
		b.WriteString("\n")
	}
	b.WriteString(strings.Join(lines, "\n"))
	return b.String()
}

// versionLabel 统一版本显示：dev → dev，v1.6.0 → v1.6.0，1.6.0 → v1.6.0。
func versionLabel() string {
	if version == "" || version == "dev" {
		return "dev"
	}
	if strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}

// ---------- 小工具 ----------

func (m *model) setToast(s string, isErr bool) {
	if s == "" {
		return
	}
	m.toast = s
	m.toastErr = isErr
	// 错误提示停留更久
	d := 4 * time.Second
	if isErr {
		d = 6 * time.Second
	}
	m.toastUntil = time.Now().Add(d)
}

func (m *model) resizeInputs() {
	w := maxInt(m.frame.contentWidth()-6, 20)
	m.input.SetWidth(w)
	m.input.SetHeight(clampInt(m.frame.contentHeight()/2, 3, 12))
	m.editInput.SetWidth(maxInt(w-4, 20))
}

// ensureChangelog 按当前宽度重新渲染更新日志（宽度变化时才重算）。
func (m *model) ensureChangelog() {
	width := m.frame.contentWidth()
	if m.chgLines != nil && m.chgWidth == width {
		return
	}
	src := changelogMD
	if m.cfg.Lang == "zh" {
		src = changelogMDZh
	}
	m.chgLines = RenderMarkdown(src, m.theme.mdStyles(maxInt(width-2, 20)))
	m.chgWidth = width
	if m.chgScroll > len(m.chgLines) {
		m.chgScroll = 0
	}
}

// retryFailedURLs 返回当前批次中失败的 URL。
func (m *model) retryFailedURLs() []string {
	var urls []string
	for _, t := range m.tasks {
		if t.Status == StatusFailed {
			urls = append(urls, t.URL)
		}
	}
	return dedupeURLs(urls)
}

// ---------- 动作 ----------

func (m *model) startDownload(urls []string) tea.Cmd {
	urls = dedupeURLs(urls)
	if len(urls) == 0 {
		m.setToast(tr(m.cfg, "toast_no_urls"), true)
		return nil
	}
	if m.batchActive {
		m.setToast(tr(m.cfg, "toast_busy"), true)
		return nil
	}

	m.dl = NewDownloader(m.cfg, m.db, m.ytdlp, urls)
	m.batchID = m.dl.ID()
	m.tasks = make([]Task, len(urls))
	for i, u := range urls {
		m.tasks[i] = Task{Index: i, URL: u, Status: StatusPending, Site: hostOf(u)}
	}
	m.batchStart = time.Now()
	m.aborted = false
	m.batchActive = true
	m.filter = filterAll
	m.taskCursor = 0
	m.taskScroll = 0
	m.state = stateDownload

	go m.dl.Run()
	return m.waitDL()
}

func (m *model) abortBatch() {
	if m.dl != nil {
		m.dl.Abort()
	}
	m.aborted = true
	m.batchActive = false
	for i := range m.tasks {
		if !m.tasks[i].Status.IsTerminal() {
			m.tasks[i].Status = StatusAborted
		}
	}
	m.state = stateSummary
	m.summaryScroll = 0
}

func (m *model) saveConfig() {
	if err := m.cfg.save(); err != nil {
		m.setToast(tr(m.cfg, "toast_save_fail")+": "+oneLine(err.Error()), true)
		return
	}
}

func (m *model) rebuildTheme() {
	m.theme = NewTheme(m.cfg.Theme)
	m.chgLines = nil // 主题变化后更新日志需要重渲染
}

// copyDiagnostics 复制诊断报告到剪贴板。
func (m *model) copyDiagnostics() {
	report := diagnosticsReport(m.cfg, m.tasks, m.ytdlp)
	if err := clipboardWrite(report); err != nil {
		m.setToast(tr(m.cfg, "summary_copy_fail"), true)
		return
	}
	m.setToast(tr(m.cfg, "summary_copied"), false)
}

func (m *model) openDownloadDir() {
	dir := expandHome(m.cfg.DownloadDir)
	if err := openPath(dir); err != nil {
		m.setToast(tr(m.cfg, "summary_open_fail"), true)
		return
	}
	m.openDirToast()
}

func (m *model) openDirToast() {
	dir := expandHome(m.cfg.DownloadDir)
	if m.cfg.PrivacyMode {
		m.setToast(tr(m.cfg, "toast_open_ok"), false)
		return
	}
	m.setToast(tr(m.cfg, "toast_open_ok")+": "+tr(m.cfg, "menu_info_dir")+" "+dir, false)
}

func (m *model) openLogsDir() {
	dir := logsDir()
	if latestFailureLog() == "" {
		m.setToast(tr(m.cfg, "summary_log_missing"), false)
		return
	}
	if err := openPath(dir); err != nil {
		m.setToast(tr(m.cfg, "summary_open_fail"), true)
		return
	}
	m.setToast(tr(m.cfg, "summary_logs_opened"), false)
}

// ---------- main ----------

func main() {
	ensurePath()

	// 支持在非交互环境里查询版本/用法（不需要 TTY）
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--version", "-v", "version":
			fmt.Println(versionLabel())
			return
		case "--help", "-h", "help":
			printUsage()
			return
		}
	}

	// 显式支持 NO_COLOR（无论终端是否支持颜色都输出纯文本），
	// 这比依赖 lipgloss/termenv 的隐式探测更可靠。
	if noColor() {
		lipgloss.SetColorProfile(termenv.Ascii)
	}

	cfg, notices := loadConfig()
	noticeStrs := make([]string, 0, len(notices)+2)
	for _, n := range notices {
		noticeStrs = append(noticeStrs, tr(cfg, n.key, n.args...))
	}

	ytdlp := detectYtdlp()
	if !ytdlp.OK {
		if !installDependencies(cfg, []string{"yt-dlp"}) {
			fmt.Println(trLang(cfg.Lang, "dep_retry_hint"))
			os.Exit(1)
		}
		ytdlp = detectYtdlp()
		if !ytdlp.OK {
			fmt.Println(trLang(cfg.Lang, "toast_ytdlp_missing"))
			os.Exit(1)
		}
	}
	if !hasCmd("ffmpeg") {
		if installDependencies(cfg, []string{"ffmpeg"}) {
			// 安装完继续；即使失败，yt-dlp 的部分功能仍可用
		} else {
			noticeStrs = append(noticeStrs, tr(cfg, "notice_ffmpeg_missing"))
		}
	}
	if cfg.UpdateCheck && ytdlp.AgeDays > ytdlpStaleDays {
		noticeStrs = append(noticeStrs, tr(cfg, "notice_ytdlp_stale", ytdlp.AgeDays))
	}

	migrateRetryFile(cfg.DownloadDir)

	db := LoadSiteDB()
	if warn := db.Warnings(); len(warn) > 0 {
		noticeStrs = append(noticeStrs, tr(cfg, "notice_sites_warning", oneLine(warn[0])))
	}

	m := newModel(cfg, db, ytdlp, noticeStrs)
	if _, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run(); err != nil {
		fmt.Println("program error:", err)
		os.Exit(1)
	}
}

// printUsage 打印命令行用法。
func printUsage() {
	fmt.Printf("%s %s — %s\n\n", trLang("en", "app_name"), versionLabel(), trLang("en", "app_tagline"))
	fmt.Println("Usage: queen [--version] [--help]")
	fmt.Println()
	fmt.Println("Configuration: ~/.config/queen/config.json")
	fmt.Println("Site profiles: ~/.config/queen/sites.json")
	fmt.Println("Failure logs:  ~/.local/state/queen/logs")
}

// installDependencies 在缺少依赖时提示用 brew 安装。
func installDependencies(cfg *Config, missing []string) bool {
	fmt.Println(trLang(cfg.Lang, "dep_missing") + ": " + strings.Join(missing, ", "))
	if !hasCmd("brew") {
		fmt.Println(trLang(cfg.Lang, "dep_no_brew"))
		return false
	}
	fmt.Print(trLang(cfg.Lang, "dep_install_q") + " ")
	var ans string
	if _, err := fmt.Scanln(&ans); err != nil {
		return false
	}
	ans = strings.ToLower(strings.TrimSpace(ans))
	if ans != "y" && ans != "yes" {
		return false
	}
	args := append([]string{"install"}, missing...)
	cmd := exec.Command("brew", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Println(trLang(cfg.Lang, "dep_install_fail") + " " + strings.Join(missing, " "))
		return false
	}
	return true
}
