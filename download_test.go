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
		mu        sync.Mutex
		msgs      []tea.Msg
		gotProg   bool
		gotBatch  bool
		gotDone   bool
		maxPct    float64
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
