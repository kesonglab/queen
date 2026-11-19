package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// streamKind 区分 yt-dlp 的 stdout（结构化进度/阶段信息）与 stderr（日志/错误）。
type streamKind uint8

const (
	streamOut streamKind = iota
	streamErr
)

// ---------- 任务公开快照 ----------

type TaskStatus uint8

const (
	StatusPending TaskStatus = iota
	StatusExtracting
	StatusDownloading
	StatusPostProcessing
	StatusDone
	StatusFailed
	StatusAborted
)

func (s TaskStatus) IsTerminal() bool {
	return s == StatusDone || s == StatusFailed || s == StatusAborted
}

func (s TaskStatus) I18nKey() string {
	switch s {
	case StatusExtracting:
		return "status_extracting"
	case StatusDownloading:
		return "status_downloading"
	case StatusPostProcessing:
		return "status_postprocessing"
	case StatusDone:
		return "status_done"
	case StatusFailed:
		return "status_failed"
	case StatusAborted:
		return "status_aborted"
	default:
		return "status_pending"
	}
}

// Task 是 UI 使用的任务快照（不可变副本，字段全部可比较，便于变更检测）。
type Task struct {
	Index           int
	URL             string
	Title           string
	Site            string
	Status          TaskStatus
	Phase           string
	PhaseDetail     string
	Percent         float64
	DownloadedBytes float64
	TotalBytes      float64
	TotalIsEstimate bool
	SpeedBps        float64
	ETASec          float64
	PlaylistIndex   int
	PlaylistCount   int
	FragIndex       int
	FragCount       int
	ErrKind         ErrorKind
	ErrDetail       string
	LogPath         string
	Attempt         int
	Started         time.Time
	Finished        time.Time
}

// Name 返回展示名：优先标题，其次截断的 URL。
func (t Task) Name() string {
	if strings.TrimSpace(t.Title) != "" {
		return t.Title
	}
	return shortURL(t.URL)
}

// PercentInt 返回四舍五入的整数百分比。
func (t Task) PercentInt() int {
	p := t.Percent
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	return int(p + 0.5)
}

// SizeText 返回「已下载 / 总大小」文本。
func (t Task) SizeText() string {
	if t.TotalBytes > 0 {
		prefix := ""
		if t.TotalIsEstimate {
			prefix = "~"
		}
		return humanBytes(t.DownloadedBytes) + " / " + prefix + humanBytes(t.TotalBytes)
	}
	if t.DownloadedBytes > 0 {
		return humanBytes(t.DownloadedBytes)
	}
	return "—"
}

func (t Task) SpeedText() string {
	if t.Status.IsTerminal() {
		return "—"
	}
	return humanRate(t.SpeedBps)
}

func (t Task) ETAText() string {
	if t.Status.IsTerminal() || t.ETASec <= 0 {
		return "—"
	}
	return fmtSec(int(t.ETASec))
}

// PhaseText 返回当前阶段的 i18n key（无阶段时回退到状态 key）。
func (t Task) PhaseText() string {
	if t.Phase != "" {
		return t.Phase
	}
	return t.Status.I18nKey()
}

// ---------- 内部可变状态 ----------

type taskState struct {
	mu sync.Mutex

	index int
	url   string
	site  string

	status      TaskStatus
	phase       string
	phaseDetail string
	title       string

	percent   float64
	streamPct float64

	downloaded float64
	total      float64
	isEstimate bool
	speed      float64
	eta        float64

	playlistIndex int
	playlistCount int

	expectedStreams int
	streamsDone     int
	itemComplete    bool
	fragIndex       int
	fragCount       int

	started  time.Time
	finished time.Time
	attempt  int

	errKind   ErrorKind
	errDetail string
	logPath   string

	args       []string
	outputPath string
	logLines   []string
	logDropped int

	cancel context.CancelFunc
	proc   *exec.Cmd
}

const maxLogLines = 800

func newTaskState(idx int, url string) *taskState {
	return &taskState{index: idx, url: url, site: hostOf(url), status: StatusPending}
}

// ---------- taskState 方法（全部内部加锁） ----------

func (st *taskState) setStarted() {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.started = time.Now()
	st.status = StatusExtracting
	st.phase = phaseExtracting
}

func (st *taskState) setProc(cmd *exec.Cmd) {
	st.mu.Lock()
	st.proc = cmd
	st.mu.Unlock()
}

// setOutputPath 记录 yt-dlp 实际写出的文件路径（用于结束时取真实文件大小）。
func (st *taskState) setOutputPath(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	st.mu.Lock()
	st.outputPath = path
	st.mu.Unlock()
}

// finalizeOutput 在任务成功后 stat 输出文件，把真实大小写入任务快照。
func (st *taskState) finalizeOutput() {
	st.mu.Lock()
	path := st.outputPath
	st.mu.Unlock()
	if path == "" {
		return
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Size() <= 0 {
		return
	}
	st.mu.Lock()
	st.total = float64(fi.Size())
	st.downloaded = st.total
	st.isEstimate = false
	st.mu.Unlock()
}

func (st *taskState) setArgs(args []string) {
	st.mu.Lock()
	st.args = append([]string(nil), args...)
	st.mu.Unlock()
}

func (st *taskState) getArgs() []string {
	st.mu.Lock()
	defer st.mu.Unlock()
	return append([]string(nil), st.args...)
}

func (st *taskState) setPhase(key, detail string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if key == "" {
		return
	}
	st.phase = key
	st.phaseDetail = detail
	switch key {
	case phaseExtracting:
		if st.status == StatusPending {
			st.status = StatusExtracting
		}
	case phaseDownloading:
		if st.status == StatusPending || st.status == StatusExtracting {
			st.status = StatusDownloading
		}
	default:
		// 后处理阶段
		if st.status != StatusDone && st.status != StatusFailed && st.status != StatusAborted {
			st.status = StatusPostProcessing
		}
	}
	st.recomputePercentLocked()
}

func (st *taskState) setExpectedStreams(n int) {
	if n <= 0 {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if n > st.expectedStreams {
		st.expectedStreams = n
		st.recomputePercentLocked()
	}
}

func (st *taskState) beginPlaylistItem(index, count int) {
	if index <= 0 || count <= 0 {
		return
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if index == st.playlistIndex && count == st.playlistCount {
		return
	}
	st.playlistIndex = index
	st.playlistCount = count
	// 新条目：重置条目内进度
	st.title = ""
	st.streamsDone = 0
	st.expectedStreams = 0
	st.itemComplete = false
	st.streamPct = 0
	st.fragIndex = 0
	st.fragCount = 0
	st.downloaded = 0
	st.total = 0
	st.isEstimate = false
	st.speed = 0
	st.eta = 0
	st.status = StatusExtracting
	st.phase = phaseExtracting
	st.recomputePercentLocked()
}

func (st *taskState) setPlaylistPos(index, count int) {
	if index <= 0 || count <= 0 {
		return
	}
	if index != st.playlistIndex || count != st.playlistCount {
		st.beginPlaylistItem(index, count)
	}
}

func (st *taskState) beginStream(filename string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.title == "" {
		st.title = filename
	}
	if st.playlistCount == 0 && st.title == "" {
		st.title = filename
	}
	st.itemComplete = false
	st.streamPct = 0
	st.fragIndex = 0
	st.fragCount = 0
	if st.status == StatusDone || st.status == StatusFailed || st.status == StatusAborted {
		return
	}
	st.status = StatusDownloading
	st.phase = phaseDownloading
	st.recomputePercentLocked()
}

func (st *taskState) updateBytes(downloaded, total float64, estimate bool, speed, eta float64, fragIndex, fragCount int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.status == StatusDone || st.status == StatusFailed || st.status == StatusAborted {
		return
	}
	if st.phase != phasePostprocessing {
		st.status = StatusDownloading
		st.phase = phaseDownloading
	}
	if downloaded > 0 {
		st.downloaded = downloaded
	}
	if total > 0 {
		st.total = total
		st.isEstimate = estimate
	}
	if speed > 0 {
		st.speed = speed
	}
	if fragIndex > 0 {
		st.fragIndex = fragIndex
	}
	if fragCount > 0 {
		st.fragCount = fragCount
	}
	if st.total > 0 && st.downloaded > 0 {
		st.streamPct = clampFloat(st.downloaded/st.total*100, 0, 100)
	} else if st.fragCount > 0 && st.fragIndex > 0 {
		st.streamPct = clampFloat(float64(st.fragIndex)/float64(st.fragCount)*100, 0, 100)
	}
	if eta > 0 {
		st.eta = eta
	} else if st.speed > 0 && st.total > st.downloaded {
		st.eta = (st.total - st.downloaded) / st.speed
	}
	st.recomputePercentLocked()
}

func (st *taskState) finishStream() {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.status == StatusDone || st.status == StatusFailed || st.status == StatusAborted {
		return
	}
	st.streamsDone++
	st.fragIndex = 0
	st.fragCount = 0
	if st.expectedStreams <= 0 {
		st.expectedStreams = st.streamsDone
	}
	st.itemComplete = st.streamsDone >= st.expectedStreams
	st.streamPct = 0
	st.downloaded = 0
	st.total = 0
	st.speed = 0
	st.eta = 0
	st.recomputePercentLocked()
}

// recomputePercentLocked 计算整体进度：
//
//	条目内进度 = (已完成流 + 当前流百分比) / 预期流数
//	播放列表整体 = (已完成条目 + 条目内进度) / 列表总数
func (st *taskState) recomputePercentLocked() {
	if st.status == StatusDone {
		st.percent = 100
		return
	}
	var item float64
	if st.itemComplete {
		item = 100
	} else {
		denom := st.expectedStreams
		if d := st.streamsDone + 1; d > denom {
			denom = d
		}
		if denom <= 0 {
			denom = 1
		}
		item = (float64(st.streamsDone) + st.streamPct/100) / float64(denom) * 100
	}
	item = clampFloat(item, 0, 100)
	if st.playlistCount > 0 && st.playlistIndex > 0 {
		item = (float64(st.playlistIndex-1) + item/100) / float64(st.playlistCount) * 100
		item = clampFloat(item, 0, 100)
	}
	// 未完成时最多显示 99%，避免「100% 卡住」的观感
	if item >= 99.5 && st.status != StatusDone {
		item = 99
	}
	if item > st.percent || st.status == StatusPostProcessing {
		st.percent = item
	} else {
		st.percent = item
	}
}

func (st *taskState) addErrorLine(line string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	line = strings.TrimSpace(line)
	if i := strings.Index(line, "ERROR:"); i >= 0 {
		line = strings.TrimSpace(line[i+len("ERROR:"):])
	}
	if line == "" {
		return
	}
	if st.errDetail == "" {
		st.errDetail = line
	} else if len(st.errDetail) < 2000 {
		st.errDetail = st.errDetail + " " + line
	}
}

func (st *taskState) appendErrorContinuation(line string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	line = strings.TrimSpace(line)
	if line == "" || st.errDetail == "" || len(st.errDetail) >= 2000 {
		return
	}
	st.errDetail = st.errDetail + " " + line
}

func (st *taskState) appendLog(line string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.logLines) >= maxLogLines {
		st.logLines = st.logLines[maxLogLines/4:]
		st.logDropped += maxLogLines / 4
	}
	st.logLines = append(st.logLines, line)
}

func (st *taskState) setErrorText(text string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.errDetail == "" {
		st.errDetail = oneLine(text)
	}
}

func (st *taskState) errorText() string {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.errDetail
}

func (st *taskState) getLog() []string {
	st.mu.Lock()
	defer st.mu.Unlock()
	out := make([]string, 0, len(st.logLines)+1)
	if st.logDropped > 0 {
		out = append(out, fmt.Sprintf("... (%d lines omitted) ...", st.logDropped))
	}
	return append(out, st.logLines...)
}

func (st *taskState) beginRetry(reason string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.attempt++
	st.errDetail = ""
	st.errKind = ""
	st.status = StatusExtracting
	st.phase = phaseExtracting
	st.phaseDetail = reason
	st.streamsDone = 0
	st.expectedStreams = 0
	st.itemComplete = false
	st.streamPct = 0
	st.percent = 0
	st.downloaded = 0
	st.total = 0
	st.isEstimate = false
	st.speed = 0
	st.eta = 0
}

func (st *taskState) markDone() {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.status = StatusDone
	st.phase = ""
	st.percent = 100
	st.finished = time.Now()
	st.proc = nil
}

func (st *taskState) markFailed(kind ErrorKind, detail, hintKey, logPath string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.status = StatusFailed
	st.errKind = kind
	if detail != "" {
		st.errDetail = detail
	}
	st.finished = time.Now()
	st.proc = nil
	st.logPath = logPath
	_ = hintKey
}

func (st *taskState) markAborted() {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.status.IsTerminal() {
		return
	}
	st.status = StatusAborted
	st.finished = time.Now()
	st.proc = nil
}

func (st *taskState) snapshot() Task {
	st.mu.Lock()
	defer st.mu.Unlock()
	t := Task{
		Index:           st.index,
		URL:             st.url,
		Title:           st.title,
		Site:            st.site,
		Status:          st.status,
		Phase:           st.phase,
		PhaseDetail:     st.phaseDetail,
		Percent:         st.percent,
		DownloadedBytes: st.downloaded,
		TotalBytes:      st.total,
		TotalIsEstimate: st.isEstimate,
		SpeedBps:        st.speed,
		ETASec:          st.eta,
		PlaylistIndex:   st.playlistIndex,
		PlaylistCount:   st.playlistCount,
		FragIndex:       st.fragIndex,
		FragCount:       st.fragCount,
		ErrKind:         st.errKind,
		ErrDetail:       st.errDetail,
		LogPath:         st.logPath,
		Attempt:         st.attempt,
		Started:         st.started,
		Finished:        st.finished,
	}
	return t
}

// ---------- 事件 ----------

// dlTaskMsg 单个任务快照更新。
type dlTaskMsg struct {
	Batch string
	Task  Task
}

// dlBatchMsg 批量聚合进度。
type dlBatchMsg struct {
	Batch      string
	Done       int
	Failed     int
	Total      int
	Active     int
	Elapsed    time.Duration
	RunningPct float64
}

// dlDoneMsg 批次结束。
type dlDoneMsg struct {
	Batch   string
	Elapsed time.Duration
	Done    int
	Failed  int
}

// dlClosedMsg 事件通道已关闭（正常结束或中止兜底）。
type dlClosedMsg struct{ Batch string }

// ---------- 连接预算 / 同域限流 ----------

// connBudget 是全局并发连接预算：任务启动前按需要申请 -N 额度。
type connBudget struct {
	total int
	sem   chan struct{}
}

func newConnBudget(total int) *connBudget {
	if total < 1 {
		total = 1
	}
	return &connBudget{total: total, sem: make(chan struct{}, total)}
}

func (b *connBudget) acquire(ctx context.Context, want int) (int, bool) {
	if want > b.total {
		want = b.total
	}
	if want < 1 {
		want = 1
	}
	got := 0
	for got < want {
		select {
		case b.sem <- struct{}{}:
			got++
		case <-ctx.Done():
			b.release(got)
			return 0, false
		}
	}
	return got, true
}

func (b *connBudget) release(n int) {
	for i := 0; i < n; i++ {
		select {
		case <-b.sem:
		default:
			return
		}
	}
}

// hostLimiter 限制同一站点的并行任务数，避免批量下载被站点限流/风控。
type hostLimiter struct {
	mu    sync.Mutex
	limit int
	chans map[string]chan struct{}
}

func newHostLimiter(limit int) *hostLimiter {
	if limit < 1 {
		limit = 1
	}
	return &hostLimiter{limit: limit, chans: make(map[string]chan struct{})}
}

func (h *hostLimiter) chanFor(host string) chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch, ok := h.chans[host]
	if !ok {
		ch = make(chan struct{}, h.limit)
		h.chans[host] = ch
	}
	return ch
}

func (h *hostLimiter) acquire(ctx context.Context, host string) bool {
	if host == "" {
		return true
	}
	ch := h.chanFor(host)
	select {
	case ch <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (h *hostLimiter) release(host string) {
	if host == "" {
		return
	}
	ch := h.chanFor(host)
	select {
	case <-ch:
	default:
	}
}

// ---------- Downloader ----------

// Downloader 负责一个批次：调度、进程管理、进度解析、自动重试、失败日志。
type Downloader struct {
	id    string
	cfg   *Config
	db    *SiteDB
	ytdlp YtdlpInfo
	urls  []string
	tasks []*taskState

	ch          chan tea.Msg
	kick        chan struct{}
	stopEmitter chan struct{}
	emitterDone chan struct{}
	closed      chan struct{}

	ctx    context.Context
	cancel context.CancelFunc

	budget *connBudget
	hosts  *hostLimiter

	mu      sync.Mutex
	procs   map[int]*exec.Cmd
	cancels map[int]context.CancelFunc

	wg    sync.WaitGroup
	start time.Time

	lastSent []Task
	notif    notificationThrottle
}

// NewDownloader 创建批次（urls 需已去重）。
func NewDownloader(cfg *Config, db *SiteDB, ytdlp YtdlpInfo, urls []string) *Downloader {
	ctx, cancel := context.WithCancel(context.Background())
	d := &Downloader{
		id:          fmt.Sprintf("b%d", time.Now().UnixNano()),
		cfg:         cfg,
		db:          db,
		ytdlp:       ytdlp,
		urls:        urls,
		ch:          make(chan tea.Msg, 512),
		kick:        make(chan struct{}, 1),
		stopEmitter: make(chan struct{}),
		emitterDone: make(chan struct{}),
		closed:      make(chan struct{}),
		ctx:         ctx,
		cancel:      cancel,
		budget:      newConnBudget(cfg.ConnectionBudget),
		hosts:       newHostLimiter(cfg.SameSiteLimit),
		procs:       make(map[int]*exec.Cmd),
		cancels:     make(map[int]context.CancelFunc),
		lastSent:    make([]Task, len(urls)),
	}
	d.tasks = make([]*taskState, len(urls))
	for i, u := range urls {
		d.tasks[i] = newTaskState(i, u)
	}
	return d
}

// ID 返回批次 ID（UI 用来丢弃过期批次的消息）。
func (d *Downloader) ID() string { return d.id }

// Events 返回事件通道。
func (d *Downloader) Events() <-chan tea.Msg { return d.ch }

// Run 启动调度器并阻塞到全部任务结束。应在独立 goroutine 中调用。
func (d *Downloader) Run() {
	d.start = time.Now()
	go d.emitter()

	sem := make(chan struct{}, clampInt(d.cfg.Concurrency, 1, 16))
	for i := range d.tasks {
		d.wg.Add(1)
		go func(idx int) {
			defer d.wg.Done()
			select {
			case <-d.ctx.Done():
				d.tasks[idx].markAborted()
				d.poke()
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			d.worker(idx)
		}(i)
	}
	d.wg.Wait()

	// 收尾顺序很重要：先停掉 emitter（它也会 flush），再在主 goroutine 做最终 flush，
	// 避免两个 goroutine 并发写 lastSent（-race 回归测试覆盖）。
	close(d.stopEmitter)
	<-d.emitterDone
	d.flushAll(true)
	done, failed := d.countResults()
	d.send(dlDoneMsg{Batch: d.id, Elapsed: time.Since(d.start), Done: done, Failed: failed})
	close(d.ch)
	close(d.closed)
}

func (d *Downloader) countResults() (int, int) {
	done, failed := 0, 0
	for _, st := range d.tasks {
		switch st.snapshot().Status {
		case StatusDone:
			done++
		case StatusFailed:
			failed++
		}
	}
	return done, failed
}

func (d *Downloader) emitter() {
	defer close(d.emitterDone)
	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-d.stopEmitter:
			return
		case <-d.kick:
			d.flushAll(false)
		case <-ticker.C:
			d.flushAll(false)
		}
	}
}

// flushAll 把有变化的任务快照推给 UI，并推送批量聚合进度。
func (d *Downloader) flushAll(force bool) {
	total := len(d.tasks)
	done, failed, active := 0, 0, 0
	runningPctSum := 0.0
	for i, st := range d.tasks {
		snap := st.snapshot()
		switch snap.Status {
		case StatusDone:
			done++
		case StatusFailed:
			failed++
		case StatusAborted:
			done++
		}
		if !snap.Status.IsTerminal() {
			active++
			runningPctSum += clampFloat(snap.Percent, 0, 100)
		}
		if force || snap != d.lastSent[i] {
			d.lastSent[i] = snap
			d.send(dlTaskMsg{Batch: d.id, Task: snap})
		}
	}
	runningPct := 0.0
	if active > 0 {
		runningPct = runningPctSum / float64(active)
	}
	d.send(dlBatchMsg{
		Batch:      d.id,
		Done:       done,
		Failed:     failed,
		Total:      total,
		Active:     active,
		Elapsed:    time.Since(d.start),
		RunningPct: runningPct,
	})
}

func (d *Downloader) poke() {
	select {
	case d.kick <- struct{}{}:
	default:
	}
}

// send 投递事件；批次取消后直接丢弃（UI 已经不再需要）。
func (d *Downloader) send(m tea.Msg) {
	select {
	case d.ch <- m:
	case <-d.ctx.Done():
	}
}

// Abort 中止整个批次：取消上下文并杀掉全部进程组。
func (d *Downloader) Abort() {
	d.cancel()
	d.mu.Lock()
	pids := make([]int, 0, len(d.procs))
	for _, c := range d.procs {
		if c != nil && c.Process != nil {
			pids = append(pids, c.Process.Pid)
		}
	}
	d.mu.Unlock()
	for _, pid := range pids {
		killProcessGroup(pid)
	}
}

// CancelTask 中止单个任务（标记为已中止，不写入失败链接）。
func (d *Downloader) CancelTask(idx int) {
	if idx < 0 || idx >= len(d.tasks) {
		return
	}
	d.tasks[idx].markAborted()
	d.mu.Lock()
	cancel := d.cancels[idx]
	delete(d.cancels, idx)
	proc := d.procs[idx]
	d.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if proc != nil && proc.Process != nil {
		killProcessGroup(proc.Process.Pid)
	}
	d.poke()
}

// ---------- worker ----------

func (d *Downloader) worker(idx int) {
	st := d.tasks[idx]
	url := st.url

	// 任务可能在排队期间被用户单独取消
	if st.snapshot().Status.IsTerminal() {
		d.poke()
		return
	}

	if !d.hosts.acquire(d.ctx, st.site) {
		st.markAborted()
		d.poke()
		return
	}
	defer d.hosts.release(st.site)

	eff := d.cfg.Effective(d.db, url)
	fragments, ok := d.budget.acquire(d.ctx, eff.Fragments)
	if !ok {
		st.markAborted()
		d.poke()
		return
	}
	defer d.budget.release(fragments)

	if st.snapshot().Status.IsTerminal() {
		d.poke()
		return
	}
	st.setStarted()
	d.poke()

	err := d.runOnce(idx, st, eff, fragments)
	hadCookies := eff.CookiesBrowser != "" || eff.CookiesFile != ""

	// 自动兜底 1：认证类失败且未使用 cookies → 带浏览器 cookies 重试一次
	if err != nil && d.ctx.Err() == nil && !hadCookies {
		diag := ClassifyError(st.errorText(), hadCookies)
		if diag.RetryWithCookies {
			if browser := d.autoCookieBrowser(); browser != "" {
				retryEff := eff
				retryEff.CookiesBrowser = browser
				st.beginRetry("retry_cookies")
				d.poke()
				err = d.runOnce(idx, st, retryEff, fragments)
				eff = retryEff
				hadCookies = true
			}
		}
	}
	// 自动兜底 2：Cloudflare 类失败且未启用 impersonate → chrome 重试一次
	if err != nil && d.ctx.Err() == nil && eff.Impersonate == "" {
		diag := ClassifyError(st.errorText(), hadCookies)
		if diag.RetryWithImpersonate {
			retryEff := eff
			retryEff.Impersonate = "chrome"
			st.beginRetry("retry_impersonate")
			d.poke()
			err = d.runOnce(idx, st, retryEff, fragments)
			eff = retryEff
		}
	}

	switch {
	case errors.Is(err, context.Canceled) || d.ctx.Err() != nil:
		st.markAborted()
	case err != nil:
		d.finishFailed(st, eff, err)
	default:
		st.finalizeOutput()
		st.markDone()
		removeFromRetry(d.cfg.DownloadDir, url)
	}
	d.poke()
}

func (d *Downloader) autoCookieBrowser() string {
	if b := d.cfg.CookieBrowser; b != "" {
		return b
	}
	return detectBrowser()
}

// runOnce 执行一次 yt-dlp 子进程并等待结束。
func (d *Downloader) runOnce(idx int, st *taskState, eff EffectiveSite, fragments int) error {
	if err := os.MkdirAll(d.cfg.DownloadDir, 0o755); err != nil {
		st.setErrorText(err.Error())
		return err
	}
	args := d.cfg.BuildArgs(st.url, eff, fragments)
	st.setArgs(args)

	taskCtx, cancel := context.WithCancel(d.ctx)
	defer cancel()
	d.mu.Lock()
	d.cancels[idx] = cancel
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		delete(d.cancels, idx)
		d.mu.Unlock()
	}()

	cmd := exec.CommandContext(taskCtx, d.ytdlp.Path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		st.setErrorText(err.Error())
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		st.setErrorText(err.Error())
		return err
	}

	d.mu.Lock()
	d.procs[idx] = cmd
	err = cmd.Start()
	d.mu.Unlock()
	if err != nil {
		st.setErrorText(err.Error())
		return err
	}
	st.setProc(cmd)

	parser := newProgressParser(st)
	var readers sync.WaitGroup
	readers.Add(2)
	go scanStream(stdout, streamOut, parser, &readers)
	go scanStream(stderr, streamErr, parser, &readers)

	waitErr := cmd.Wait()
	readers.Wait()

	d.mu.Lock()
	delete(d.procs, idx)
	d.mu.Unlock()

	if taskCtx.Err() != nil || d.ctx.Err() != nil {
		return context.Canceled
	}
	return waitErr
}

// scanStream 逐行读取子进程输出。使用 bufio.Reader 而不是 Scanner，
// 避免单行超出 Scanner 上限时停止读取、令子进程写管道阻塞造成假死。
func scanStream(r readCloserLike, kind streamKind, p *progressParser, wg *sync.WaitGroup) {
	defer wg.Done()
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			p.feed(kind, strings.TrimRight(line, "\r\n"))
		}
		if err != nil {
			return
		}
	}
}

type readCloserLike interface {
	Read([]byte) (int, error)
}

// finishFailed 记录失败：分类、写日志、失败链接、通知。
func (d *Downloader) finishFailed(st *taskState, eff EffectiveSite, err error) {
	text := st.errorText()
	if text == "" {
		text = err.Error()
	}
	diag := ClassifyError(text, eff.CookiesBrowser != "" || eff.CookiesFile != "")
	detail := oneLine(diag.Detail)
	if detail == "" {
		detail = oneLine(text)
	}
	if detail == "" {
		detail = err.Error()
	}
	logPath := ""
	if d.cfg.KeepFailureLogs {
		logPath, _ = writeFailureLog(d.cfg, failureInput{
			URL:     st.url,
			Title:   st.title,
			Site:    st.site,
			Args:    st.getArgs(),
			Output:  st.getLog(),
			Kind:    diag.Kind,
			Detail:  detail,
			Version: d.ytdlp.Version,
			Privacy: eff.Privacy || d.cfg.PrivacyMode,
		})
	}
	st.markFailed(diag.Kind, detail, "errhint_"+string(diag.Kind), logPath)
	if eff.Privacy || d.cfg.PrivacyMode {
		// 隐私站点：失败链接仍写入（便于重试），但通知里不显示 URL
	}
	appendToRetry(d.cfg.DownloadDir, st.url)
	if d.notif.allow(st.url) {
		notifyTaskFailed(d.cfg, d.db, st.url, detail, diag.Kind)
	}
}
