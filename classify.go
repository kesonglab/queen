package main

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// ErrorKind 是 yt-dlp 失败的粗分类，用于 UI 展示与 i18n。
// 字符串值即翻译 key 的后缀（见 I18nKey），不可随意改动。
type ErrorKind string

const (
	KindUnknown     ErrorKind = "unknown"
	KindAuth        ErrorKind = "auth"
	KindMembership  ErrorKind = "membership"
	KindDRM         ErrorKind = "drm"
	KindGeo         ErrorKind = "geo"
	KindCloudflare  ErrorKind = "cloudflare"
	KindRateLimit   ErrorKind = "rate_limit"
	KindUnsupported ErrorKind = "unsupported"
	KindNetwork     ErrorKind = "network"
	KindNotFound    ErrorKind = "not_found"
	KindStorage     ErrorKind = "storage"
	KindExtractor   ErrorKind = "extractor"
)

// Diagnosis 是对一次 yt-dlp 失败的诊断结果。
type Diagnosis struct {
	Kind                 ErrorKind // 粗分类，供 UI / i18n 使用
	Detail               string    // 从 yt-dlp 输出提取的一行人类可读详情（已去 ANSI，截断到 300 个 rune）
	RetryWithCookies     bool      // 是否值得带上浏览器 cookies 自动重试一次
	RetryWithImpersonate bool      // 是否值得加上 --impersonate 自动重试一次
	RetryLater           bool      // 是否属于临时性错误，稍后重试同一 URL 可能成功
}

const (
	// classifyMaxDetailRunes 限制 Detail 中来自 yt-dlp 文本的长度（截断时另加 "..."）。
	classifyMaxDetailRunes = 300
	// classifyEllipsis 是截断标记。
	classifyEllipsis = "..."
)

// classifyErrorLineRe 匹配 "error:" 开头的错误行并捕获其后的内容（大小写不敏感）。
var classifyErrorLineRe = regexp.MustCompile(`(?i)error:\s*(.+)`)

// classifyKindMatcher 把一个分类与它的全部小写子串特征绑定。
type classifyKindMatcher struct {
	kind     ErrorKind
	patterns []string
}

// classifyKindMatchers 按优先级从上到下排列：第一个命中任意 pattern 的分类胜出。
// 顺序非常关键（每一档都写了原因），调整前请先看 classify_test.go 里的优先级用例。
var classifyKindMatchers = []classifyKindMatcher{
	// 1. DRM 最优先：DRM 报错里往往同时出现账号、地区、cookies 等字样。
	{kind: KindDRM, patterns: []string{
		"drm", "widevine", "fairplay", "playready", "protected by drm", "drm-protected",
	}},
	// 2. 本地存储/文件系统错误先于账号类，避免把 "Permission denied"（如 cookies 文件不可写）
	//    误判成 cookie/登录问题。
	{kind: KindStorage, patterns: []string{
		"no space left", "disk full", "read-only file system", "permission denied",
		"file name too long", "errno 28", "cannot write file",
	}},
	// 3. 会员内容先于 auth：members-only 报错通常也会提示登录。
	{kind: KindMembership, patterns: []string{
		"members-only", "members only", "join this channel", "subscriber", "premium",
		"paid", "requires payment", "available to patrons", "only available to",
	}},
	// 4. 账号/登录/年龄/cookies 类。注意 "account has been terminated" 必须在此命中，
	//    否则会被 not_found 的 "terminated" 抢走。
	{kind: KindAuth, patterns: []string{
		"sign in", "sign-in", "login required", "log in", "authentication",
		"requires authentication", "cookies", "cookie", "private video",
		"this video is private", "age-restricted", "confirm your age",
		"account has been terminated", "login", "credentials",
	}},
	// 5. 地区限制。
	{kind: KindGeo, patterns: []string{
		"not available in your country", "not available from your location",
		"geo-restrict", "geo restrict", "region-locked", "region locked",
		"blocked in your country", "geo bypass",
	}},
	// 6. Cloudflare / 反爬 403。先于限流与 extractor：403 需要 --impersonate 而不是重试。
	{kind: KindCloudflare, patterns: []string{
		"cloudflare", "checking your browser", "just a moment", "cf-chl",
		"http error 403", "403 forbidden", "access denied", "blocked by",
	}},
	// 7. 限流，属于 RetryLater。
	{kind: KindRateLimit, patterns: []string{
		"http error 429", "429", "too many requests", "rate limit", "rate-limit",
	}},
	// 8. 不支持的 URL / 无可用格式。
	{kind: KindUnsupported, patterns: []string{
		"unsupported url", "no suitable extractor", "is not a valid url",
		"no video formats found", "unsupported",
	}},
	// 9. extractor 自身失败先于 network / not_found：
	//    "Unable to extract initial data" 是需要升级 yt-dlp 的 extractor bug，不是网络抖动。
	{kind: KindExtractor, patterns: []string{
		"unable to extract", "extractor", "unable to download webpage",
		"unable to download video data", "failed to parse", "keyerror",
		"traceback", "unable to extract initial data",
	}},
	// 10. 网络类（含 5xx），属于 RetryLater。
	{kind: KindNetwork, patterns: []string{
		"timed out", "timeout", "connection reset", "connection refused",
		"temporary failure", "name or service not known", "network is unreachable",
		"ssl", "certificate", "incomplete read", "http error 5", "502", "503", "504",
		"broken pipe", "connection aborted", "remote end closed", "no route to host",
	}},
	// 11. 内容不存在/已删除，属于永久失败。
	{kind: KindNotFound, patterns: []string{
		"video unavailable", "has been removed", "does not exist", "not found",
		"404", "no longer available", "has been deleted", "deleted", "unavailable",
		"terminated",
	}},
}

// ClassifyError 根据 yt-dlp 的完整输出（stderr+stdout）给出诊断。
// hadCookies 表示本次命令是否已经带了浏览器 cookies。
func ClassifyError(text string, hadCookies bool) Diagnosis {
	plain := classifyStripANSI(text)
	lower := strings.ToLower(plain)

	kind := KindUnknown
	for _, m := range classifyKindMatchers {
		if classifyContainsAny(lower, m.patterns) {
			kind = m.kind
			break
		}
	}

	return Diagnosis{
		Kind:   kind,
		Detail: classifyDetail(plain),
		// 只有账号/会员类错误才值得在没带 cookies 的情况下降级重试一次。
		RetryWithCookies: !hadCookies && (kind == KindAuth || kind == KindMembership),
		// 反爬 403 通常只能靠 --impersonate 更换 TLS 指纹绕过。
		RetryWithImpersonate: kind == KindCloudflare,
		// 只有限流与网络错误标记为稍后重试。extractor 失败大多要靠升级 yt-dlp 修复，
		// 偶发的 IMDb/API 抖动不值得为此自动重试，故不计入。
		RetryLater: kind == KindRateLimit || kind == KindNetwork,
	}
}

// I18nKey 返回该分类的翻译 key，例如 "errkind_auth"。
func (k ErrorKind) I18nKey() string {
	if k == "" {
		return "errkind_" + string(KindUnknown)
	}
	return "errkind_" + string(k)
}

// classifyDetail 提取一行人类可读摘要：优先取第一处 "error:" 之后的内容；
// 没有 error 行时退回到最后一个非空行。结果会折叠空白并按 rune 截断。
func classifyDetail(plain string) string {
	// 按 "\n" 切行即可：\r 会被后续的空白折叠去掉。
	lines := strings.Split(plain, "\n")

	// yt-dlp 的致命错误行以 "ERROR:" 开头，但也可能出现在 WARNING 等前缀之后。
	for _, line := range lines {
		m := classifyErrorLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if detail := classifyNormalize(m[1]); detail != "" {
			return detail
		}
	}

	// 没有 error 标记时取最后一个非空行，通常是真正的失败原因。
	for i := len(lines) - 1; i >= 0; i-- {
		if detail := classifyNormalize(lines[i]); detail != "" {
			return detail
		}
	}
	return ""
}

// classifyNormalize 去掉首尾空白、把连续空白折叠为单个空格，并按 rune 数截断。
func classifyNormalize(s string) string {
	oneLine := strings.Join(strings.Fields(s), " ")
	return classifyTruncateRunes(oneLine, classifyMaxDetailRunes)
}

// classifyTruncateRunes 在 UTF-8 rune 边界上把 s 截断到最多 limit 个 rune，
// 超长时在末尾追加 "..."。输入含非法 UTF-8 时按字节边界退化为单字节 rune，不会切坏合法字符。
func classifyTruncateRunes(s string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(s) <= limit {
		return s
	}
	n := 0
	for i := range s {
		if n == limit {
			return s[:i] + classifyEllipsis
		}
		n++
	}
	return s
}

// classifyContainsAny 报告已转小写的 lower 中是否包含任意一个小写 pattern。
func classifyContainsAny(lower string, patterns []string) bool {
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// classifyStripANSI 移除 ANSI/VT100 转义序列（CSI 颜色、OSC 超链接等）。
// yt-dlp 在 TTY 下会给错误文本着色，若不清理会污染 Detail 与关键字匹配。
func classifyStripANSI(s string) string {
	if !strings.Contains(s, "\x1b") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != 0x1b {
			b.WriteByte(c)
			continue
		}
		// ESC 序列：CSI (ESC [) / OSC (ESC ]) / 字符集指定 (ESC ( B 等) / 其他两字节。
		j := i + 1
		if j >= len(s) {
			break
		}
		switch s[j] {
		case '[': // 控制序列：ESC [ 参数 终止字节(0x40-0x7e)
			j++
			for j < len(s) && !(s[j] >= 0x40 && s[j] <= 0x7e) {
				j++
			}
			i = j
		case ']': // 操作系统命令：ESC ] ... BEL 或 ST(ESC \)
			j++
			for j < len(s) && s[j] != 0x07 && !(s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\') {
				j++
			}
			if j < len(s) && s[j] == 0x1b {
				j++
			}
			i = j
		case '(', ')', '#', '%': // 字符集指定序列，多带一个字节
			j++
			i = j
		default: // 其他两字节序列
			i = j
		}
	}
	return b.String()
}
