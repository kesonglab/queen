package main

import (
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ---------- yt-dlp 可执行文件信息 ----------

// YtdlpInfo 描述本机 yt-dlp 的版本与安装方式。
type YtdlpInfo struct {
	Path        string
	Version     string
	ReleaseDate time.Time
	AgeDays     int
	BrewManaged bool
	OK          bool
}

// detectYtdlp 探测 yt-dlp 路径与版本（-U 与 --version 都是毫秒级）。
func detectYtdlp() YtdlpInfo {
	info := YtdlpInfo{}
	path, err := lookupCmd("yt-dlp")
	if err != nil {
		return info
	}
	info.Path = path
	info.BrewManaged = isBrewManaged(path)
	info.OK = true
	out, err := runCommandTimeout(5*time.Second, "yt-dlp", "--version")
	if err != nil {
		return info
	}
	info.Version = firstLine(out)
	if d, ok := parseYtdlpVersion(info.Version); ok {
		info.ReleaseDate = d
		info.AgeDays = int(time.Since(d).Hours() / 24)
	}
	return info
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// parseYtdlpVersion 解析 "2026.10.26" / "2026.10.26.232746" 这类版本号。
func parseYtdlpVersion(v string) (time.Time, bool) {
	v = strings.TrimSpace(strings.TrimPrefix(v, "v"))
	parts := strings.Split(v, ".")
	if len(parts) < 3 {
		return time.Time{}, false
	}
	y, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	d, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return time.Time{}, false
	}
	if y < 2000 || m < 1 || m > 12 || d < 1 || d > 31 {
		return time.Time{}, false
	}
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC), true
}

// UpgradeCommand 返回适合该安装方式的升级命令。
func (y YtdlpInfo) UpgradeCommand() []string {
	if y.BrewManaged {
		return []string{"brew", "upgrade", "yt-dlp"}
	}
	return []string{"yt-dlp", "-U"}
}

// UpgradeLabel 返回给人看的升级方式描述。
func (y YtdlpInfo) UpgradeLabel() string {
	if y.BrewManaged {
		return "brew upgrade yt-dlp"
	}
	return "yt-dlp -U"
}

// ---------- 参数构建 ----------

// progressTemplateArgs 返回结构化的进度模板参数。
// 使用固定分隔符与纯数值/枚举字段，彻底摆脱对人类可读文本的正则解析。
func progressTemplateArgs() []string {
	const sep = "|"
	dl := "download:QDL" + sep + strings.Join([]string{
		"%(progress.status)s",
		"%(progress.downloaded_bytes)s",
		"%(progress.total_bytes)s",
		"%(progress.total_bytes_estimate)s",
		"%(progress.speed)s",
		"%(progress.eta)s",
		"%(info.playlist_index)s",
		"%(info.playlist_count)s",
		"%(progress.fragment_index)s",
		"%(progress.fragment_count)s",
	}, sep)
	pp := "postprocess:QPP" + sep + strings.Join([]string{
		"%(progress.status)s",
		"%(progress.postprocessor)s",
	}, sep)
	return []string{"--progress-template", dl, "--progress-template", pp}
}

// outputTemplate 返回输出文件名模板。
func outputTemplate(cfg *Config) string {
	if cfg.Playlist && cfg.PlaylistFolder {
		return "%(playlist_title)s/%(playlist_index)03d - %(title)s [%(id)s].%(ext)s"
	}
	return "%(title)s [%(id)s].%(ext)s"
}

// BuildArgs 组装一次 yt-dlp 调用的完整参数列表（不含可执行文件名）。
// fragments 由连接预算动态决定；eff 为站点档案合并后的有效配置。
func (c *Config) BuildArgs(url string, eff EffectiveSite, fragments int) []string {
	args := make([]string, 0, 64)

	if c.IgnoreYtdlpConfig {
		args = append(args, "--ignore-config")
	}
	// 机器解析场景：禁用颜色与动态进度条，改为逐行输出
	args = append(args, "--color", "never", "--newline", "--progress")
	args = append(args, progressTemplateArgs()...)

	// 格式与容器
	if c.AudioOnly {
		args = append(args, "-x", "--audio-format", c.AudioFormat, "--audio-quality", "0")
	} else {
		args = append(args, "-f", c.effectiveFormat())
		if c.MergeFormat != "" {
			args = append(args, "--merge-output-format", c.MergeFormat)
		}
	}
	if c.Embed {
		args = append(args, "--embed-metadata", "--embed-thumbnail")
	}
	if c.Subs && !c.AudioOnly {
		args = append(args, "--write-subs", "--sub-langs", c.SubLangs, "--embed-subs")
		if c.AutoSubs {
			args = append(args, "--write-auto-subs")
		}
	}

	// 输出位置与命名
	args = append(args, "-o", outputTemplate(c), "-P", c.DownloadDir)
	args = append(args, "--no-overwrites")
	if c.TrimFilenames > 0 {
		args = append(args, "--trim-filenames", strconv.Itoa(c.TrimFilenames))
	}
	if c.Playlist {
		args = append(args, "--yes-playlist")
	} else {
		args = append(args, "--no-playlist")
	}
	if c.Archive {
		args = append(args, "--download-archive", archivePath(c.DownloadDir))
	}

	// 重试与网络
	retries := clampInt(c.RetryTimes, 0, 10)
	args = append(args,
		"--retries", strconv.Itoa(retries),
		"--fragment-retries", strconv.Itoa(retries),
		"--extractor-retries", strconv.Itoa(maxInt(retries, 2)),
		"--file-access-retries", "3",
		"--socket-timeout", "30",
	)
	if c.RetrySleep > 0 {
		args = append(args, "--retry-sleep", strconv.Itoa(c.RetrySleep))
	}
	if c.ThrottledRate != "" {
		// 反限速：速率持续低于阈值时自动重新提取（对 YouTube 等限速站点效果显著）
		args = append(args, "--throttled-rate", c.ThrottledRate)
	}
	if c.LimitRate != "" {
		args = append(args, "--limit-rate", c.LimitRate)
	}
	if fragments > 0 {
		args = append(args, "--concurrent-fragments", strconv.Itoa(fragments))
	}

	// 认证 / 站点兼容
	if eff.CookiesFile != "" {
		args = append(args, "--cookies", eff.CookiesFile)
	} else if eff.CookiesBrowser != "" {
		args = append(args, "--cookies-from-browser", eff.CookiesBrowser)
	}
	if eff.Impersonate != "" {
		args = append(args, "--impersonate", eff.Impersonate)
	}
	if eff.ExtractorArgs != "" {
		args = append(args, "--extractor-args", eff.ExtractorArgs)
	}
	if proxy := strings.TrimSpace(c.Proxy); proxy != "" {
		args = append(args, "--proxy", proxy)
	}
	if ua := chooseNonEmpty(eff.UserAgent, c.UserAgent); ua != "" {
		args = append(args, "--user-agent", ua)
	}
	if ref := chooseNonEmpty(eff.Referer, c.Referer); ref != "" {
		args = append(args, "--referer", ref)
	}

	args = append(args, url)
	return args
}

func chooseNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// ---------- 进度解析 ----------

const (
	statusDownloading = "downloading"
	statusFinished    = "finished"
	statusError       = "error"
)

// 阶段（i18n key 后缀）。
const (
	phaseExtracting     = "phase_extracting"
	phaseDownloading    = "phase_downloading"
	phaseMerging        = "phase_merging"
	phaseConverting     = "phase_converting"
	phaseEmbedding      = "phase_embedding"
	phaseMetadata       = "phase_metadata"
	phaseFixup          = "phase_fixup"
	phasePostprocessing = "phase_postprocessing"
)

var (
	reFormatCount  = regexp.MustCompile(`Downloading \d+ format\(s\): (.+)$`)
	rePlaylistItem = regexp.MustCompile(`Downloading item (\d+) of (\d+)`)
	reQuotedPath   = regexp.MustCompile(`"([^"]+)"`)
)

// progressParser 解析单个任务的一行输出，并把结果写入 taskState。
type progressParser struct {
	st *taskState
}

func newProgressParser(st *taskState) *progressParser { return &progressParser{st: st} }

// feed 处理一行输出。kind 用于区分 stdout（进度/结构化模板）与 stderr（日志/错误）。
func (p *progressParser) feed(kind streamKind, raw string) {
	line := stripANSI(strings.TrimRight(raw, "\r"))
	if strings.TrimSpace(line) == "" {
		return
	}
	switch {
	case strings.HasPrefix(line, "QDL|"):
		p.parseDownload(strings.TrimPrefix(line, "QDL|"))
		return
	case strings.HasPrefix(line, "QPP|"):
		p.parsePostprocess(strings.TrimPrefix(line, "QPP|"))
		return
	}
	p.st.appendLog(line)

	if m := rePlaylistItem.FindStringSubmatch(line); m != nil {
		idx, _ := strconv.Atoi(m[1])
		count, _ := strconv.Atoi(m[2])
		p.st.beginPlaylistItem(idx, count)
		return
	}
	if m := reFormatCount.FindStringSubmatch(line); m != nil {
		p.st.setExpectedStreams(1 + strings.Count(m[1], "+"))
		return
	}
	switch {
	case strings.HasPrefix(line, "[download] Destination:"):
		dest := strings.TrimSpace(strings.TrimPrefix(line, "[download] Destination:"))
		if dest != "" {
			p.st.beginStream(filepath.Base(dest))
			p.st.setOutputPath(dest)
		}
		return
	case strings.Contains(line, "] Destination: "):
		// 后处理器（如 ExtractAudio）也会输出 Destination
		dest := strings.TrimSpace(line[strings.Index(line, "] Destination: ")+len("] Destination: "):])
		p.st.setOutputPath(dest)
		return
	case strings.Contains(line, "Merging formats into"):
		if m := reQuotedPath.FindStringSubmatch(line); m != nil {
			p.st.setOutputPath(m[1])
		}
		return
	case strings.Contains(line, "Extracting URL"):
		p.st.setPhase(phaseExtracting, "")
		return
	case strings.Contains(line, "Downloading 1 format"), strings.Contains(line, "Downloading format"):
		p.st.setPhase(phaseExtracting, "")
		return
	}
	// 后处理阶段：yt-dlp 的后处理器输出形如 [Merger] Merging formats into "..."
	if name, ok := postprocessorName(line); ok {
		p.st.setPhase(phaseForPostprocessor(name), name)
		return
	}
	// 错误：ERROR: 开头，后续缩进行属于同一条错误
	if strings.HasPrefix(line, "ERROR:") || strings.Contains(line, "ERROR:") {
		p.st.addErrorLine(strings.TrimSpace(line))
		return
	}
	if strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t") {
		p.st.appendErrorContinuation(strings.TrimSpace(line))
	}
}

// parseDownload 处理结构化下载进度。
// 字段：status|downloaded|total|total_estimate|speed|eta|pl_index|pl_count|frag_index|frag_count
func (p *progressParser) parseDownload(payload string) {
	parts := strings.Split(payload, "|")
	if len(parts) < 6 {
		return
	}
	status := strings.TrimSpace(parts[0])
	downloaded, _ := parseNum(parts[1])
	total, hasTotal := parseNum(parts[2])
	estimate, hasEstimate := parseNum(parts[3])
	speed, _ := parseNum(parts[4])
	eta, _ := parseNum(parts[5])
	var plIndex, plCount, fragIndex, fragCount float64
	if len(parts) > 7 {
		plIndex, _ = parseNum(parts[6])
		plCount, _ = parseNum(parts[7])
	}
	if len(parts) > 9 {
		fragIndex, _ = parseNum(parts[8])
		fragCount, _ = parseNum(parts[9])
	}
	if plIndex > 0 && plCount > 0 {
		p.st.setPlaylistPos(int(plIndex), int(plCount))
	}
	switch status {
	case statusFinished:
		p.st.finishStream()
	case statusError:
		p.st.setPhase(phaseDownloading, "")
	default:
		unitTotal := 0.0
		if hasTotal {
			unitTotal = total
		} else if hasEstimate {
			unitTotal = estimate
		}
		p.st.updateBytes(downloaded, unitTotal, hasEstimate && !hasTotal, speed, eta, int(fragIndex), int(fragCount))
	}
}

// parsePostprocess 处理后处理进度：status|postprocessor
func (p *progressParser) parsePostprocess(payload string) {
	parts := strings.Split(payload, "|")
	if len(parts) < 2 {
		return
	}
	pp := strings.TrimSpace(parts[1])
	p.st.setPhase(phaseForPostprocessor(pp), pp)
}

// postprocessorName 从输出行中提取后处理器名称，例如 [Merger] / [ExtractAudio]。
func postprocessorName(line string) (string, bool) {
	if !strings.HasPrefix(line, "[") {
		return "", false
	}
	end := strings.IndexByte(line, ']')
	if end <= 1 {
		return "", false
	}
	name := line[1:end]
	switch name {
	case "Merger", "ExtractAudio", "EmbedThumbnail", "ThumbnailsConvertor",
		"Metadata", "EmbedSubtitle", "WriteSubs", "FixupM3u8", "VideoRemuxer",
		"FixupStretched", "FixupDuplicateMoov", "FixupTimestamp", "SplitChapters",
		"ModifyChapters", "MoveFiles", "FFmpegCopyStream", "FFmpegConcat":
		return name, true
	}
	return "", false
}

// phaseForPostprocessor 把后处理器名称映射为阶段。
func phaseForPostprocessor(name string) string {
	switch name {
	case "Merger", "FFmpegCopyStream", "FFmpegConcat":
		return phaseMerging
	case "ExtractAudio":
		return phaseConverting
	case "EmbedThumbnail", "ThumbnailsConvertor":
		return phaseEmbedding
	case "Metadata", "EmbedSubtitle", "WriteSubs":
		return phaseMetadata
	case "FixupM3u8", "VideoRemuxer", "FixupStretched", "FixupDuplicateMoov", "FixupTimestamp":
		return phaseFixup
	case "SplitChapters", "ModifyChapters", "MoveFiles":
		return phasePostprocessing
	}
	return phasePostprocessing
}

// ---------- 升级 ----------

// runYtdlpUpgrade 执行升级并返回输出。
func runYtdlpUpgrade(info YtdlpInfo) (string, bool) {
	cmdArgs := info.UpgradeCommand()
	if len(cmdArgs) == 0 {
		return "no upgrade command available", false
	}
	name := cmdArgs[0]
	args := cmdArgs[1:]
	if !hasCmd(name) {
		return "missing command: " + name, false
	}
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out) + "\n" + err.Error(), false
	}
	return string(out), true
}
