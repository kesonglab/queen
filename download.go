package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TaskStatus 下载任务状态
type TaskStatus int

const (
	StatusPending TaskStatus = iota
	StatusDownloading
	StatusDone
	StatusFailed
	StatusAborted
)

// Task 单个下载任务
type Task struct {
	URL        string
	Status     TaskStatus
	Percent    float64
	Downloaded string
	Total      string
	Speed      string
	ETA        string
	Title      string
	Error      string
}

// taskUpdateMsg 单个任务进度更新
type taskUpdateMsg struct {
	idx int
	t   Task
}

// batchProgressMsg 批量进度
type batchProgressMsg struct {
	done, total int
	elapsed     time.Duration
}

// batchDoneMsg 全部完成
type batchDoneMsg struct{}

// abortMsg 用户中止
type abortMsg struct{}

var (
	// 进度行可能带有未知速度 / 未知 ETA（如下载初期），需宽松匹配
	rePct   = regexp.MustCompile(`\[download\]\s+([\d.]+)%`)
	reSize  = regexp.MustCompile(`of\s+~?([\d.]+)\s*([KMGiB]*B?)`)
	reSpeed = regexp.MustCompile(`at\s+([\d.]+ ?[KMGiB]*B/s|Unknown B/s|Unknown)`)
	reETA   = regexp.MustCompile(`ETA\s+([\d:]+|Unknown)`)
	reDest  = regexp.MustCompile(`\[download\]\s+Destination: (.+)`)
	reInfo  = regexp.MustCompile(`\[info\]\s+Downloading item (\d+) of (\d+)`)
	reError = regexp.MustCompile(`(?i)(ERROR:\s*|\[error\]\s*)(.+)`)

	// 解析 yt-dlp 自带速度字符串，如 "1.23MiB/s"
	reSpeedVal = regexp.MustCompile(`([\d.]+)\s*([KMGiB]*B?)/s`)
)

// progTrack 单个任务的平滑/锁存状态，避免数字前后跳动
type progTrack struct {
	maxPct    float64 // 进度单调不回退
	totalB    float64 // 锁存首次得到的真实总大小
	totalLock bool
	lastB     float64 // 上一次累计下载字节
	lastT     time.Time
	smoothBps float64 // 指数平滑后的速度（字节/秒）
	haveSpeed bool
}

// ensureTrack 获取（或创建）某任务的内部跟踪器
func (d *downloader) ensureTrack(idx int) *progTrack {
	if d.tracks == nil {
		d.tracks = make(map[int]*progTrack)
	}
	tr, ok := d.tracks[idx]
	if !ok {
		tr = &progTrack{}
		d.tracks[idx] = tr
	}
	return tr
}

// resetTrack 重置进度跟踪器（新流 / 新目标文件开始时）
func (d *downloader) resetTrack(idx int) *progTrack {
	if d.tracks == nil {
		d.tracks = make(map[int]*progTrack)
	}
	tr := &progTrack{}
	d.tracks[idx] = tr
	return tr
}

// unitBytes 把 ytdlp 的大小单位换算成字节乘数
func unitBytes(unit string) float64 {
	u := strings.ToUpper(strings.TrimSpace(unit))
	ib := strings.HasSuffix(u, "IB")
	switch {
	case u == "" || u == "B":
		return 1
	case strings.HasPrefix(u, "K"):
		if ib {
			return 1024
		}
		return 1000
	case strings.HasPrefix(u, "M"):
		if ib {
			return 1024 * 1024
		}
		return 1000 * 1000
	case strings.HasPrefix(u, "G"):
		if ib {
			return 1024 * 1024 * 1024
		}
		return 1000 * 1000 * 1000
	case strings.HasPrefix(u, "T"):
		if ib {
			return 1024 * 1024 * 1024 * 1024
		}
		return 1000 * 1000 * 1000 * 1000
	}
	return 1
}

// parseSizeToBytes 把 "100" + "MiB" 解析成字节数
func parseSizeToBytes(val, unit string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
	if err != nil {
		return 0
	}
	return v * unitBytes(unit)
}

// parseSpeedToBytes 解析 ytdlp 速度字符串（如 "1.23MiB/s"）为字节/秒
func parseSpeedToBytes(s string) (float64, bool) {
	m := reSpeedVal.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	return v * unitBytes(m[2]), true
}

// humanBytes 把字节数格式化为人类可读字符串
func humanBytes(b float64) string {
	if b <= 0 {
		return "—"
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	i := 0
	for b >= 1024 && i < len(units)-1 {
		b /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%dB", int(b))
	}
	return fmt.Sprintf("%.1f%s", b, units[i])
}

// downloader 多线程并发执行所有任务
type downloader struct {
	cfg    *Config
	urls   []string
	ch     chan tea.Msg
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	cmds   map[int]*exec.Cmd

	tasks  []*Task
	tracks map[int]*progTrack
	start  time.Time
	total  int
	done   int32
}

func newDownloader(cfg *Config, urls []string) *downloader {
	ctx, cancel := context.WithCancel(context.Background())
	tasks := make([]*Task, len(urls))
	for i, u := range urls {
		tasks[i] = &Task{URL: u, Status: StatusPending}
	}
	return &downloader{
		cfg:    cfg,
		urls:   urls,
		ch:     make(chan tea.Msg, 128),
		ctx:    ctx,
		cancel: cancel,
		cmds:   make(map[int]*exec.Cmd),
		tasks:  tasks,
		total:  len(urls),
	}
}

// run 启动一个 worker 池并发下载
func (d *downloader) run() {
	d.start = time.Now()
	total := len(d.urls)
	d.total = total

	concurrency := d.cfg.Concurrency
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > 16 {
		concurrency = 16
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			select {
			case <-d.ctx.Done():
				d.updateTask(idx, func(t *Task) {
					t.Status = StatusAborted
				})
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			d.worker(idx)
		}(i)
	}

	wg.Wait()
	// 无论正常完成还是被中止，都投递完成消息并关闭 channel：
	// 否则中止时 waitForUpdates 会一直阻塞在 <-ch 上导致协程泄漏。
	// send 内部已通过 select 在 ctx 取消时跳过投递，此处仍需 close 唤醒读协程。
	d.send(batchDoneMsg{})
	close(d.ch)
}

func (d *downloader) worker(idx int) {
	url := d.urls[idx]
	d.updateTask(idx, func(t *Task) {
		t.Status = StatusDownloading
	})
	err := d.runYtdlp(idx, url)
	canceled := errors.Is(err, context.Canceled) || d.ctx.Err() != nil
	d.updateTask(idx, func(t *Task) {
		if canceled {
			t.Status = StatusAborted
			t.Error = ""
		} else if err != nil {
			t.Status = StatusFailed
			t.Error = cleanErr(err.Error())
		} else {
			t.Status = StatusDone
			t.Percent = 100
		}
	})
	if canceled {
		// 用户中止：不写入失败链接
	} else if err != nil {
		appendToRetry(d.cfg.DownloadDir, url)
	} else {
		removeFromRetry(d.cfg.DownloadDir, url)
	}
	done := atomic.AddInt32(&d.done, 1)
	d.send(batchProgressMsg{done: int(done), total: d.total, elapsed: time.Since(d.start)})
}

func (d *downloader) abort() {
	d.cancel()
	d.mu.Lock()
	pids := make([]int, 0, len(d.cmds))
	for _, c := range d.cmds {
		if c != nil && c.Process != nil {
			pids = append(pids, c.Process.Pid)
		}
	}
	d.mu.Unlock()
	for _, pid := range pids {
		// 杀掉整个进程组，避免 ffmpeg 等子进程残留
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}

func (d *downloader) send(m tea.Msg) {
	select {
	case d.ch <- m:
	case <-d.ctx.Done():
	}
}

// updateTask 在锁内修改任务状态并发送更新副本
func (d *downloader) updateTask(idx int, fn func(*Task)) {
	d.mu.Lock()
	t := d.tasks[idx]
	if t == nil {
		t = &Task{URL: d.urls[idx]}
		d.tasks[idx] = t
	}
	fn(t)
	cp := *t
	d.mu.Unlock()
	d.send(taskUpdateMsg{idx: idx, t: cp})
}

// ytDlpRunner 创建 yt-dlp 进程（可在测试中替换）
var ytDlpRunner = func(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "yt-dlp", args...)
}

func (d *downloader) runYtdlp(idx int, url string) error {
	args := d.cfg.ytdlpArgs(url)
	cmd := ytDlpRunner(d.ctx, args...)
	// 独立进程组，便于 abort 时一并杀掉 ffmpeg 等子进程
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	// 与 abort 共用同一把锁，避免 Start 写入 Process 时并发 Kill
	d.mu.Lock()
	d.cmds[idx] = cmd
	errStart := cmd.Start()
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		delete(d.cmds, idx)
		d.mu.Unlock()
	}()

	if errStart != nil {
		pw.Close()
		pr.Close()
		return errStart
	}

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			d.handleLine(idx, sc.Text())
		}
	}()

	err := cmd.Wait()
	pw.Close()
	<-readDone
	pr.Close()

	if d.ctx.Err() != nil {
		return context.Canceled
	}
	if err != nil {
		return err
	}
	return nil
}

// handleLine 解析 yt-dlp 输出行
func (d *downloader) handleLine(idx int, line string) {
	if m := rePct.FindStringSubmatch(line); m != nil {
		pct, _ := strconv.ParseFloat(m[1], 64)
		d.updateTask(idx, func(t *Task) {
			t.Status = StatusDownloading

			track := d.ensureTrack(idx)

			// 新流开始：进度从接近 100% 跳回低位时重置跟踪器
			// （bestvideo+bestaudio 等常见于视频流完成后开始音频流）
			if track.maxPct >= 90 && pct < 30 {
				track = d.resetTrack(idx)
			}

			// 进度单调不回退，避免进度条前后波动
			if pct > track.maxPct {
				track.maxPct = pct
			}
			t.Percent = track.maxPct

			// 锁存首次得到的真实总大小，避免估算值反复跳动
			if sm := reSize.FindStringSubmatch(line); sm != nil {
				if totalB := parseSizeToBytes(sm[1], sm[2]); totalB > 0 && !track.totalLock {
					track.totalB = totalB
					track.totalLock = true
				}
			}

			if track.totalB > 0 {
				// 用「已下载字节 = 单调进度 × 总大小」算出真实吞吐，抑制抖动
				downB := track.maxPct / 100 * track.totalB
				now := time.Now()
				if !track.lastT.IsZero() {
					dt := now.Sub(track.lastT).Seconds()
					if dt > 0.05 && downB >= track.lastB {
						sample := (downB - track.lastB) / dt
						if sample > 0 {
							if !track.haveSpeed {
								track.smoothBps = sample
								track.haveSpeed = true
							} else {
								// 指数平滑，稳定速度显示
								track.smoothBps = track.smoothBps*0.8 + sample*0.2
							}
						}
						track.lastB = downB
						track.lastT = now
					}
				} else {
					track.lastB = downB
					track.lastT = now
				}

				t.Downloaded = humanBytes(downB)
				t.Total = humanBytes(track.totalB)
				t.Speed = humanBytes(track.smoothBps) + "/s"
				if track.haveSpeed && track.smoothBps > 0 && track.maxPct < 100 {
					remain := track.totalB - downB
					if remain < 0 {
						remain = 0
					}
					t.ETA = fmtSec(int(remain / track.smoothBps))
				}
			} else {
				// 无总大小时退化为平滑的 ytdlp 自带速度与 ETA
				t.Downloaded = "—"
				t.Total = "—"
				if spm := reSpeed.FindStringSubmatch(line); spm != nil {
					s := strings.TrimSpace(spm[1])
					if s != "Unknown" && s != "Unknown B/s" {
						if bps, ok := parseSpeedToBytes(s); ok {
							if !track.haveSpeed {
								track.smoothBps = bps
								track.haveSpeed = true
							} else {
								track.smoothBps = track.smoothBps*0.8 + bps*0.2
							}
						}
						t.Speed = humanBytes(track.smoothBps) + "/s"
					}
				}
				if em := reETA.FindStringSubmatch(line); em != nil {
					e := em[1]
					if e != "Unknown" {
						t.ETA = e
					} else if pct >= 100 {
						t.ETA = tr(d.cfg, "eta_done")
					}
				} else if pct >= 100 {
					t.ETA = tr(d.cfg, "eta_done")
				}
			}

			if track.maxPct >= 100 {
				t.ETA = tr(d.cfg, "eta_done")
				t.Speed = "—"
			}
		})
		return
	}
	if m := reDest.FindStringSubmatch(line); m != nil {
		d.updateTask(idx, func(t *Task) {
			t.Title = m[1]
			// 新目标文件：重置进度，避免上一流的 100% 锁死后续流
			d.resetTrack(idx)
			t.Percent = 0
			t.Downloaded = "—"
			t.Total = "—"
			t.Speed = "—"
			t.ETA = ""
		})
		return
	}
	if m := reInfo.FindStringSubmatch(line); m != nil {
		d.updateTask(idx, func(t *Task) {
			t.Status = StatusDownloading
		})
		return
	}
	if m := reError.FindStringSubmatch(line); m != nil {
		d.updateTask(idx, func(t *Task) {
			t.Error = strings.TrimSpace(m[2])
		})
		return
	}
}

func cleanErr(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "\n"); i >= 0 {
		s = s[:i]
	}
	r := []rune(s)
	if len(r) > 200 {
		s = string(r[:200]) + "..."
	}
	return s
}

// waitForUpdates 订阅下载器消息
func waitForUpdates(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		m, ok := <-ch
		if !ok {
			return batchDoneMsg{}
		}
		return m
	}
}
