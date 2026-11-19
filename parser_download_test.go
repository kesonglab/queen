package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func feedLines(p *progressParser, kind streamKind, lines ...string) {
	for _, l := range lines {
		p.feed(kind, l)
	}
}

// QDL 行构造器：status|downloaded|total|estimate|speed|eta|pl_idx|pl_count|frag_idx|frag_count
func qdl(status, downloaded, total, estimate, speed, eta, plIdx, plCount, fragIdx, fragCount string) string {
	return "QDL|" + strings.Join([]string{status, downloaded, total, estimate, speed, eta, plIdx, plCount, fragIdx, fragCount}, "|")
}

func TestParserStructuredProgress(t *testing.T) {
	st := newTaskState(0, "https://example.com/v")
	p := newProgressParser(st)
	p.feed(streamOut, qdl("downloading", "25000000", "100000000", "NA", "5000000", "15", "NA", "NA", "NA", "NA"))
	snap := st.snapshot()
	if snap.Status != StatusDownloading {
		t.Errorf("status = %v", snap.Status)
	}
	if snap.Percent < 24.9 || snap.Percent > 25.1 {
		t.Errorf("percent = %v", snap.Percent)
	}
	if snap.TotalBytes != 100000000 || snap.DownloadedBytes != 25000000 {
		t.Errorf("bytes = %v/%v", snap.DownloadedBytes, snap.TotalBytes)
	}
	if snap.SpeedBps != 5000000 {
		t.Errorf("speed = %v", snap.SpeedBps)
	}
	if snap.TotalIsEstimate {
		t.Error("exact total should not be marked as estimate")
	}
}

func TestParserEstimatedTotal(t *testing.T) {
	st := newTaskState(0, "https://example.com/v")
	p := newProgressParser(st)
	// 只有估算大小（对应 yt-dlp 的 "of ~ 1.23MiB" 场景）
	p.feed(streamOut, qdl("downloading", "500", "NA", "1000", "100", "NA", "NA", "NA", "NA", "NA"))
	snap := st.snapshot()
	if !snap.TotalIsEstimate {
		t.Error("estimate should be flagged")
	}
	if snap.TotalBytes != 1000 || snap.DownloadedBytes != 500 {
		t.Errorf("bytes = %v/%v", snap.DownloadedBytes, snap.TotalBytes)
	}
	if snap.Percent < 49.9 || snap.Percent > 50.1 {
		t.Errorf("percent = %v", snap.Percent)
	}
}

func TestParserMultiStreamAndPhases(t *testing.T) {
	st := newTaskState(0, "https://example.com/v")
	p := newProgressParser(st)

	p.feed(streamOut, "[info] abc123: Downloading 1 format(s): 248+251")
	p.feed(streamOut, "[download] Destination: /tmp/out/video [abc].f248.webm")
	snap := st.snapshot()
	if snap.Title != "video [abc].f248.webm" {
		t.Errorf("title = %q", snap.Title)
	}

	p.feed(streamOut, qdl("downloading", "50", "100", "NA", "10", "NA", "NA", "NA", "NA", "NA"))
	if got := st.snapshot().Percent; got < 24.9 || got > 25.1 {
		t.Errorf("first stream percent = %v (want 25: 0.5/2)", got)
	}
	p.feed(streamOut, qdl("finished", "100", "100", "NA", "NA", "NA", "NA", "NA", "NA", "NA"))
	if got := st.snapshot().Percent; got < 49.9 || got > 50.1 {
		t.Errorf("after first stream percent = %v (want 50)", got)
	}

	p.feed(streamOut, "[download] Destination: /tmp/out/audio [abc].f251.webm")
	p.feed(streamOut, qdl("downloading", "50", "100", "NA", "10", "NA", "NA", "NA", "NA", "NA"))
	if got := st.snapshot().Percent; got < 74.9 || got > 75.1 {
		t.Errorf("second stream percent = %v (want 75)", got)
	}
	p.feed(streamOut, qdl("finished", "100", "100", "NA", "NA", "NA", "NA", "NA", "NA", "NA"))
	if got := st.snapshot().Percent; got < 99 {
		t.Errorf("completed percent = %v (want 99 until done)", got)
	}

	p.feed(streamOut, "QPP|started|Merger")
	snap = st.snapshot()
	if snap.Status != StatusPostProcessing {
		t.Errorf("status after merger = %v", snap.Status)
	}
	if snap.Phase != phaseMerging {
		t.Errorf("phase = %q", snap.Phase)
	}
}

func TestParserPlaylistProgress(t *testing.T) {
	st := newTaskState(0, "https://example.com/playlist")
	p := newProgressParser(st)
	p.feed(streamOut, "[download] Downloading item 2 of 5")
	p.feed(streamOut, "[info] abc: Downloading 1 format(s): 18")
	p.feed(streamOut, "[download] Destination: /tmp/out/item2.mp4")
	p.feed(streamOut, qdl("downloading", "50", "100", "NA", "10", "NA", "2", "5", "NA", "NA"))
	snap := st.snapshot()
	if snap.PlaylistIndex != 2 || snap.PlaylistCount != 5 {
		t.Fatalf("playlist = %d/%d", snap.PlaylistIndex, snap.PlaylistCount)
	}
	// 整体进度 = ((2-1) + 0.5) / 5 = 30%
	if snap.Percent < 29.5 || snap.Percent > 30.5 {
		t.Errorf("playlist percent = %v (want 30)", snap.Percent)
	}
}

func TestParserErrorCapture(t *testing.T) {
	st := newTaskState(0, "https://example.com/v")
	p := newProgressParser(st)
	p.feed(streamErr, "ERROR: [youtube] abc123: Sign in to confirm you're not a bot")
	p.feed(streamErr, "  Retry with --cookies-from-browser")
	snap := st.snapshot()
	detail := st.errorText()
	if !strings.Contains(detail, "Sign in to confirm") {
		t.Errorf("error detail = %q", detail)
	}
	if !strings.Contains(detail, "Retry with --cookies") {
		t.Errorf("continuation not captured: %q", detail)
	}
	_ = snap
}

func TestParserDestinationAndFileSize(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(out, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	st := newTaskState(0, "https://example.com/v")
	p := newProgressParser(st)
	p.feed(streamOut, "[download] Destination: "+out)
	p.feed(streamOut, qdl("finished", "10", "10", "NA", "NA", "NA", "NA", "NA", "NA", "NA"))
	if got := st.snapshot().Title; got != "video.mp4" {
		t.Errorf("title = %q", got)
	}
	st.finalizeOutput()
	if got := st.snapshot().TotalBytes; got != 10 {
		t.Errorf("finalized size = %v", got)
	}
}

func TestParserMergerOutputPath(t *testing.T) {
	dir := t.TempDir()
	merged := filepath.Join(dir, "merged.mp4")
	if err := os.WriteFile(merged, []byte("0123456789012345678901234567890123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	st := newTaskState(0, "https://example.com/v")
	p := newProgressParser(st)
	p.feed(streamOut, `[Merger] Merging formats into "`+merged+`"`)
	st.finalizeOutput()
	if got := st.snapshot().TotalBytes; got != 40 {
		t.Errorf("merged size = %v", got)
	}
}

func TestParserFragmentFallback(t *testing.T) {
	st := newTaskState(0, "https://example.com/live")
	p := newProgressParser(st)
	// 无总大小，只有分片信息
	p.feed(streamOut, qdl("downloading", "0", "NA", "NA", "NA", "NA", "NA", "NA", "3", "10"))
	if got := st.snapshot().Percent; got < 25 {
		t.Errorf("fragment percent = %v (want 30)", got)
	}
}

func TestParserIgnoresGarbage(t *testing.T) {
	st := newTaskState(0, "https://example.com/v")
	p := newProgressParser(st)
	feedLines(p, streamOut, "QDL|", "QDL|downloading", "random output", "QDL|downloading|abc|def")
	if snap := st.snapshot(); snap.Status != StatusPending {
		t.Errorf("truncated/garbage lines should not change status: %v", snap.Status)
	}
	// 字段完整但数值不可解析：进入下载态但进度为 0
	p.feed(streamOut, "QDL|downloading|abc|def|NA|NA|NA|NA|NA|NA")
	snap := st.snapshot()
	if snap.Status != StatusDownloading {
		t.Errorf("status = %v, want downloading", snap.Status)
	}
	if snap.Percent != 0 || snap.TotalBytes != 0 {
		t.Errorf("invalid numbers should be ignored: %+v", snap)
	}
}

// ---------- 下载器集成测试 ----------

const fakeYtdlpTemplate = `#!/bin/sh
if [ "$1" = "--version" ]; then echo "2026.10.26"; exit 0; fi
url=""
for a in "$@"; do url="$a"; done
echo "RUN" >> %[1]s
printf '%s\n' "$@" >> %[1]s
case "$url" in
  *bad*)
    echo "ERROR: [youtube] abc: Sign in to confirm you're not a bot" >&2
    exit 1
    ;;
  *flaky*)
    case "$*" in
      *--cookies-from-browser*)
        ;;
      *)
        echo "ERROR: [youtube] abc: Sign in required" >&2
        exit 1
        ;;
    esac
    ;;
esac
echo "[info] abc: Downloading 1 format(s): 18"
echo "[download] Destination: %[2]s/$(basename "$url").mp4"
echo "QDL|downloading|500|1000|NA|100|NA|NA|NA|NA"
echo "QDL|finished|1000|1000|NA|NA|NA|NA|NA|NA"
printf 'x' > "%[2]s/$(basename "$url").mp4"
exit 0
`

func setupDownloader(t *testing.T, urls []string) (*Downloader, string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	work := t.TempDir()
	outDir := filepath.Join(work, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	argsLog := filepath.Join(work, "args.log")
	script := filepath.Join(work, "yt-dlp")
	body := strings.ReplaceAll(fakeYtdlpTemplate, "%[1]s", argsLog)
	body = strings.ReplaceAll(body, "%[2]s", outDir)
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.DownloadDir = outDir
	cfg.KeepFailureLogs = true
	cfg.RetryTimes = 0
	cfg.RetrySleep = 0
	cfg.ThrottledRate = ""
	d := NewDownloader(cfg, defaultSiteDB(), YtdlpInfo{Path: script, OK: true, Version: "2026.10.26"}, urls)
	return d, outDir, argsLog
}

func runDownloader(t *testing.T, d *Downloader) map[int]Task {
	t.Helper()
	go d.Run()
	tasks := make(map[int]Task)
	deadline := time.After(15 * time.Second)
	for {
		select {
		case msg, ok := <-d.Events():
			if !ok {
				return tasks
			}
			switch m := msg.(type) {
			case dlTaskMsg:
				tasks[m.Task.Index] = m.Task
			case dlDoneMsg:
				// 继续读直到通道关闭，确保最终快照都收到
			}
		case <-deadline:
			t.Fatal("timeout waiting for downloader")
		}
	}
}

func TestDownloaderSuccessAndFailure(t *testing.T) {
	oldDetect := detectBrowser
	detectBrowser = func() string { return "" } // 不触发自动 cookie 重试
	defer func() { detectBrowser = oldDetect }()

	d, outDir, _ := setupDownloader(t, []string{"https://example.com/good", "https://example.com/bad"})
	tasks := runDownloader(t, d)

	if tasks[0].Status != StatusDone {
		t.Errorf("good task status = %v", tasks[0].Status)
	}
	if tasks[1].Status != StatusFailed {
		t.Errorf("bad task status = %v", tasks[1].Status)
	}
	if tasks[1].ErrKind != KindAuth {
		t.Errorf("bad task kind = %v, want auth", tasks[1].ErrKind)
	}
	if !strings.Contains(tasks[1].ErrDetail, "Sign in") {
		t.Errorf("bad task detail = %q", tasks[1].ErrDetail)
	}
	if tasks[1].LogPath == "" {
		t.Error("failure log path should be set")
	} else if data, err := os.ReadFile(tasks[1].LogPath); err != nil || !strings.Contains(string(data), "Sign in") {
		t.Errorf("failure log should contain the real error (err=%v)", err)
	}
	// 失败链接文件只包含坏链接
	retry := readRetry(outDir)
	if len(retry) != 1 || !strings.Contains(retry[0], "bad") {
		t.Errorf("retry file = %v", retry)
	}
	// 输出文件存在
	if _, err := os.Stat(filepath.Join(outDir, "good.mp4")); err != nil {
		t.Errorf("expected output file: %v", err)
	}
}

func TestDownloaderCookieAutoRetry(t *testing.T) {
	oldDetect := detectBrowser
	detectBrowser = func() string { return "firefox" }
	defer func() { detectBrowser = oldDetect }()

	d, _, argsLog := setupDownloader(t, []string{"https://example.com/flaky"})
	tasks := runDownloader(t, d)
	if tasks[0].Status != StatusDone {
		t.Fatalf("flaky task should succeed after cookie retry, got %v (%s)", tasks[0].Status, tasks[0].ErrDetail)
	}
	data, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatal(err)
	}
	runs := strings.Count(string(data), "RUN")
	if runs < 2 {
		t.Fatalf("expected at least 2 yt-dlp runs, got %d", runs)
	}
	if !strings.Contains(string(data), "--cookies-from-browser") {
		t.Error("retry should include --cookies-from-browser")
	}
}

func TestDownloaderAbort(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	work := t.TempDir()
	outDir := filepath.Join(work, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(work, "yt-dlp")
	body := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 2026.10.26; exit 0; fi\nsleep 5\nexit 0\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.DownloadDir = outDir
	cfg.RetryTimes = 0
	d := NewDownloader(cfg, defaultSiteDB(), YtdlpInfo{Path: script, OK: true}, []string{"https://example.com/a", "https://example.com/b"})
	go d.Run()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for task start")
		default:
		}
		if sn := d.tasks[0].snapshot().Status; sn == StatusDownloading || sn == StatusExtracting {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	d.Abort()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-d.Events():
			if !ok {
				goto finished
			}
		case <-timeout:
			t.Fatal("downloader did not stop after abort")
		}
	}
finished:
	for i := 0; i < 2; i++ {
		if sn := d.tasks[i].snapshot().Status; sn == StatusFailed {
			t.Errorf("task %d marked failed after abort", i)
		}
	}
	if lines := readRetry(outDir); len(lines) != 0 {
		t.Errorf("abort should not write retry file: %v", lines)
	}
}

func TestDiagnosticsReportPrivacy(t *testing.T) {
	cfg := defaultConfig()
	cfg.PrivacyMode = true
	tasks := []Task{{Index: 0, URL: "https://secret.example/v/1", Status: StatusFailed, ErrKind: KindNetwork, ErrDetail: "failed: https://secret.example/v/1"}}
	report := diagnosticsReport(cfg, tasks, YtdlpInfo{Version: "2026.10.26"})
	if strings.Contains(report, "secret.example") {
		t.Errorf("privacy mode should redact URLs in diagnostics:\n%s", report)
	}
	if !strings.Contains(report, "privacy mode") {
		t.Errorf("report should mention privacy redaction:\n%s", report)
	}
	cfg.PrivacyMode = false
	report = diagnosticsReport(cfg, tasks, YtdlpInfo{Version: "2026.10.26"})
	if !strings.Contains(report, "secret.example") {
		t.Error("non-privacy report should include the URL")
	}
}

func TestDownloaderPrivacyLogRedaction(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	work := t.TempDir()
	outDir := filepath.Join(work, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(work, "yt-dlp")
	body := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 2026.10.26; exit 0; fi\necho \"ERROR: unable to download video data: HTTP Error 502\" >&2\nexit 1\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.DownloadDir = outDir
	cfg.PrivacyMode = true
	cfg.RetryTimes = 0
	d := NewDownloader(cfg, defaultSiteDB(), YtdlpInfo{Path: script, OK: true}, []string{"https://secret.example/v/123"})
	tasks := runDownloader(t, d)
	if tasks[0].Status != StatusFailed {
		t.Fatalf("status = %v", tasks[0].Status)
	}
	if tasks[0].LogPath == "" {
		t.Fatal("log path empty")
	}
	data, err := os.ReadFile(tasks[0].LogPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret.example") {
		t.Errorf("privacy mode should redact URL from log:\n%s", data)
	}
	if !strings.Contains(string(data), "privacy mode") {
		t.Errorf("log should note privacy redaction:\n%s", data)
	}
}
