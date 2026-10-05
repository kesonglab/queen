package main

import (
	"context"
	"os/exec"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// fakeYtDlp 写出与 yt-dlp 类似格式的进度行，用于验证解析与消息投递
func fakeYtDlp(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", `
echo "[download]   0.0% of    1.23MiB at  Unknown B/s ETA Unknown"
sleep 0.1
echo "[download]  50.0% of    1.23MiB at  500KiB/s ETA 00:01"
sleep 0.1
echo "[download] 100% of    1.23MiB in 00:00:02 at  600KiB/s"
echo "[download] Destination: test [abc].mp4"
`)
	return cmd
}

func TestDownloaderPipeline(t *testing.T) {
	cfg := defaultConfig()
	urls := []string{"http://example.com/a", "http://example.com/b"}

	old := ytDlpRunner
	ytDlpRunner = fakeYtDlp
	defer func() { ytDlpRunner = old }()

	d := newDownloader(cfg, urls)
	go d.run()

	var (
		mu       sync.Mutex
		msgs     []tea.Msg
		gotProg  bool
		gotBatch bool
		gotDone  bool
		maxPct   float64
	)
	timeout := time.After(5 * time.Second)
loop:
	for {
		select {
		case m, ok := <-d.ch:
			if !ok {
				break loop
			}
			mu.Lock()
			msgs = append(msgs, m)
			switch m.(type) {
			case taskUpdateMsg:
				gotProg = true
				if tm, ok := m.(taskUpdateMsg); ok && tm.t.Percent > maxPct {
					maxPct = tm.t.Percent
				}
			case batchProgressMsg:
				gotBatch = true
			case batchDoneMsg:
				gotDone = true
			}
			mu.Unlock()
		case <-timeout:
			break loop
		}
	}

	t.Logf("total messages: %d", len(msgs))
	if !gotProg {
		t.Error("no taskUpdateMsg (progress) received")
	}
	if maxPct < 100 {
		t.Errorf("expected progress to reach 100%%, got %.1f", maxPct)
	}
	if !gotBatch {
		t.Error("no batchProgressMsg received")
	}
	if !gotDone {
		t.Error("no batchDoneMsg received")
	}
}

func TestAbortDoesNotFail(t *testing.T) {
	cfg := defaultConfig()
	cfg.DownloadDir = t.TempDir()
	urls := []string{"http://example.com/a", "http://example.com/b", "http://example.com/c"}

	old := ytDlpRunner
	ytDlpRunner = func(ctx context.Context, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 5")
	}
	defer func() { ytDlpRunner = old }()

	cfg.Concurrency = 1
	d := newDownloader(cfg, urls)
	go d.run()

	// 等第一个任务进入下载
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		d.mu.Lock()
		started := false
		for _, tsk := range d.tasks {
			if tsk != nil && tsk.Status == StatusDownloading {
				started = true
				break
			}
		}
		d.mu.Unlock()
		if started {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	d.abort()

	timeout := time.After(3 * time.Second)
	for {
		select {
		case _, ok := <-d.ch:
			if !ok {
				goto done
			}
		case <-timeout:
			t.Fatal("timeout waiting for downloader to finish after abort")
		}
	}
done:
	d.mu.Lock()
	defer d.mu.Unlock()
	for i, tsk := range d.tasks {
		if tsk == nil {
			t.Fatalf("task %d is nil", i)
		}
		if tsk.Status == StatusFailed {
			t.Errorf("task %d marked Failed after abort", i)
		}
		if tsk.Status != StatusAborted && tsk.Status != StatusDone {
			t.Errorf("task %d status = %v, want Aborted (or Done)", i, tsk.Status)
		}
	}
	if lines := readRetry(cfg.DownloadDir); len(lines) != 0 {
		t.Errorf("abort should not write retry file, got %v", lines)
	}
}

func TestMultiStreamProgressReset(t *testing.T) {
	cfg := defaultConfig()
	d := newDownloader(cfg, []string{"http://example.com/a"})
	d.tasks = []*Task{{URL: "http://example.com/a", Status: StatusDownloading}}

	d.handleLine(0, "[download] 100.0% of 10.00MiB at 1.00MiB/s ETA 00:00")
	d.mu.Lock()
	if d.tasks[0].Percent < 100 {
		d.mu.Unlock()
		t.Fatalf("expected 100%% after first stream, got %.1f", d.tasks[0].Percent)
	}
	d.mu.Unlock()

	// 新 Destination 应重置进度
	d.handleLine(0, "[download] Destination: audio.m4a")
	d.mu.Lock()
	if d.tasks[0].Percent != 0 {
		d.mu.Unlock()
		t.Fatalf("expected reset to 0 after Destination, got %.1f", d.tasks[0].Percent)
	}
	d.mu.Unlock()

	d.handleLine(0, "[download]  25.0% of  2.00MiB at 500KiB/s ETA 00:03")
	d.mu.Lock()
	pct := d.tasks[0].Percent
	d.mu.Unlock()
	if pct < 24 || pct > 26 {
		t.Fatalf("expected ~25%% on second stream, got %.1f", pct)
	}
}

func TestCleanErrUTF8(t *testing.T) {
	r := make([]rune, 250)
	for i := range r {
		r[i] = '测'
	}
	out := cleanErr(string(r))
	runes := []rune(out)
	// 200 字符 + "..."
	if len(runes) != 203 {
		t.Fatalf("cleanErr length = %d runes, want 203", len(runes))
	}
	if out[len(out)-3:] != "..." {
		t.Fatalf("expected trailing ..., got %q", out[len(out)-3:])
	}
	for _, rr := range runes[:200] {
		if rr != '测' {
			t.Fatalf("unexpected rune %q (possible mid-UTF-8 cut)", rr)
		}
	}
}

func TestRenderBarNegative(t *testing.T) {
	// 不应 panic
	_ = renderBar(-10, 20)
	_ = renderBar(150, 20)
}
