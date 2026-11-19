package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// classifyTestCase 描述一条完整的分类用例：输入、cookies 状态与全部预期输出。
type classifyTestCase struct {
	name                 string
	text                 string
	hadCookies           bool
	wantKind             ErrorKind
	wantDetail           string
	wantRetryCookies     bool
	wantRetryImpersonate bool
	wantRetryLater       bool
}

// TestClassifyErrorKinds 用真实 yt-dlp 报错样本覆盖每一种 ErrorKind，
// 同时校验 Detail、三个重试标志以及 hadCookies 的影响。
func TestClassifyErrorKinds(t *testing.T) {
	const signInText = `ERROR: [youtube] dQw4w9WgXcQ: Sign in to confirm you're not a bot. Use --cookies-from-browser or --cookies for the authentication.`
	const signInDetail = `[youtube] dQw4w9WgXcQ: Sign in to confirm you're not a bot. Use --cookies-from-browser or --cookies for the authentication.`
	const membersText = `ERROR: [bilibili] BV1xx411c7mD: This video is only available to members`
	const membersDetail = `[bilibili] BV1xx411c7mD: This video is only available to members`

	cases := []classifyTestCase{
		{
			name:       "unknown",
			text:       "some totally unexpected failure happened",
			wantKind:   KindUnknown,
			wantDetail: "some totally unexpected failure happened",
		},
		{
			name:       "unknown-empty",
			text:       "",
			wantKind:   KindUnknown,
			wantDetail: "",
		},
		{
			name:       "storage-no-space-left",
			text:       "ERROR: [Errno 28] No space left on device",
			wantKind:   KindStorage,
			wantDetail: "[Errno 28] No space left on device",
		},
		{
			// storage 优先于 auth：cookies 文件写不进去应按本地磁盘问题处理。
			name:       "storage-permission-denied-beats-auth",
			text:       `ERROR: [Errno 13] Permission denied: '/home/u/.cache/yt-dlp/cookies.txt'`,
			wantKind:   KindStorage,
			wantDetail: `[Errno 13] Permission denied: '/home/u/.cache/yt-dlp/cookies.txt'`,
		},
		{
			name:             "auth-sign-in-cookies",
			text:             signInText,
			wantKind:         KindAuth,
			wantDetail:       signInDetail,
			wantRetryCookies: true,
		},
		{
			// 已经带 cookies 时不再自动用 cookies 重试。
			name:             "auth-sign-in-had-cookies",
			text:             signInText,
			hadCookies:       true,
			wantKind:         KindAuth,
			wantDetail:       signInDetail,
			wantRetryCookies: false,
		},
		{
			name:             "membership-members-only",
			text:             membersText,
			wantKind:         KindMembership,
			wantDetail:       membersDetail,
			wantRetryCookies: true,
		},
		{
			name:             "membership-had-cookies",
			text:             membersText,
			hadCookies:       true,
			wantKind:         KindMembership,
			wantDetail:       membersDetail,
			wantRetryCookies: false,
		},
		{
			name:       "drm-protected",
			text:       `ERROR: [vimeo] 12345: This video is DRM protected`,
			wantKind:   KindDRM,
			wantDetail: `[vimeo] 12345: This video is DRM protected`,
		},
		{
			name:       "geo-not-available-in-country",
			text:       `ERROR: [vimeo] 12345: This video is not available in your country`,
			wantKind:   KindGeo,
			wantDetail: `[vimeo] 12345: This video is not available in your country`,
		},
		{
			name:                 "cloudflare-http-403",
			text:                 `ERROR: unable to download video data: HTTP Error 403: Forbidden`,
			wantKind:             KindCloudflare,
			wantDetail:           `unable to download video data: HTTP Error 403: Forbidden`,
			wantRetryImpersonate: true,
		},
		{
			name:                 "cloudflare-browser-check",
			text:                 `ERROR: [youtube] abc: 403 Forbidden - checking your browser before accessing`,
			wantKind:             KindCloudflare,
			wantDetail:           `[youtube] abc: 403 Forbidden - checking your browser before accessing`,
			wantRetryImpersonate: true,
		},
		{
			name:           "rate-limit-http-429",
			text:           `ERROR: [download] Got error: HTTP Error 429: Too Many Requests`,
			wantKind:       KindRateLimit,
			wantDetail:     `[download] Got error: HTTP Error 429: Too Many Requests`,
			wantRetryLater: true,
		},
		{
			name:       "unsupported-url",
			text:       `ERROR: Unsupported URL: https://example.com/foo`,
			wantKind:   KindUnsupported,
			wantDetail: `Unsupported URL: https://example.com/foo`,
		},
		{
			name:       "extractor-initial-data",
			text:       `ERROR: [generic] Unable to extract initial data; please report this issue on https://github.com/yt-dlp/yt-dlp/issues?q= and confirm you are on the latest version.`,
			wantKind:   KindExtractor,
			wantDetail: `[generic] Unable to extract initial data; please report this issue on https://github.com/yt-dlp/yt-dlp/issues?q= and confirm you are on the latest version.`,
		},
		{
			name:           "network-http-503",
			text:           `ERROR: [youtube] dQw4w9WgXcQ: HTTP Error 503: Service Unavailable`,
			wantKind:       KindNetwork,
			wantDetail:     `[youtube] dQw4w9WgXcQ: HTTP Error 503: Service Unavailable`,
			wantRetryLater: true,
		},
		{
			name:           "network-connection-reset",
			text:           `ERROR: [download] Got error: Connection reset by peer (104)`,
			wantKind:       KindNetwork,
			wantDetail:     `[download] Got error: Connection reset by peer (104)`,
			wantRetryLater: true,
		},
		{
			name:       "not-found-video-unavailable",
			text:       `ERROR: [youtube] abc: Video unavailable`,
			wantKind:   KindNotFound,
			wantDetail: `[youtube] abc: Video unavailable`,
		},
		{
			name:       "not-found-http-404",
			text:       `ERROR: [youtube] abc: HTTP Error 404: Not Found`,
			wantKind:   KindNotFound,
			wantDetail: `[youtube] abc: HTTP Error 404: Not Found`,
		},
		{
			// auth 先于 not_found："terminated" 不应被当成内容被删除。
			name:             "precedence-auth-over-not-found",
			text:             `ERROR: [youtube] abc: This account has been terminated`,
			wantKind:         KindAuth,
			wantDetail:       `[youtube] abc: This account has been terminated`,
			wantRetryCookies: true,
		},
		{
			// membership 先于 auth：members-only 报错通常也提示登录。
			name:             "precedence-membership-over-auth",
			text:             `ERROR: [youtube] abc: Join this channel to get access to members-only content. Sign in to confirm your age.`,
			wantKind:         KindMembership,
			wantDetail:       `[youtube] abc: Join this channel to get access to members-only content. Sign in to confirm your age.`,
			wantRetryCookies: true,
		},
		{
			// extractor 先于 network："Unable to extract" 是需要升级 yt-dlp 的 bug。
			name:       "precedence-extractor-over-network",
			text:       `ERROR: unable to extract initial data: HTTP Error 503: Service Unavailable`,
			wantKind:   KindExtractor,
			wantDetail: `unable to extract initial data: HTTP Error 503: Service Unavailable`,
		},
		{
			// 多行输出里 WARNING 先出现 "extractor"，但真正的 ERROR 是登录问题，auth 优先。
			name: "multiline-warning-extractor-but-auth-error",
			text: `[youtube] Extracting URL: https://www.youtube.com/watch?v=dQw4w9WgXcQ
WARNING: [youtube] Falling back on generic information extractor
[debug] Command-line config: ['-f', 'bv*+ba']
ERROR: [youtube] dQw4w9WgXcQ: Sign in to confirm you're not a bot.`,
			wantKind:         KindAuth,
			wantDetail:       `[youtube] dQw4w9WgXcQ: Sign in to confirm you're not a bot.`,
			wantRetryCookies: true,
		},
	}

	covered := make(map[ErrorKind]bool, len(cases))
	for _, tc := range cases {
		covered[tc.wantKind] = true
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyError(tc.text, tc.hadCookies)

			if got.Kind != tc.wantKind {
				t.Errorf("Kind = %q, want %q (detail=%q)", got.Kind, tc.wantKind, got.Detail)
			}
			if got.Detail != tc.wantDetail {
				t.Errorf("Detail = %q, want %q", got.Detail, tc.wantDetail)
			}
			if got.RetryWithCookies != tc.wantRetryCookies {
				t.Errorf("RetryWithCookies = %v, want %v", got.RetryWithCookies, tc.wantRetryCookies)
			}
			if got.RetryWithImpersonate != tc.wantRetryImpersonate {
				t.Errorf("RetryWithImpersonate = %v, want %v", got.RetryWithImpersonate, tc.wantRetryImpersonate)
			}
			if got.RetryLater != tc.wantRetryLater {
				t.Errorf("RetryLater = %v, want %v", got.RetryLater, tc.wantRetryLater)
			}
		})
	}

	// 保证每种 ErrorKind 都至少有一条真实样本，防止后续新增分类时漏测。
	allKinds := []ErrorKind{
		KindUnknown, KindAuth, KindMembership, KindDRM, KindGeo, KindCloudflare,
		KindRateLimit, KindUnsupported, KindNetwork, KindNotFound, KindStorage, KindExtractor,
	}
	for _, kind := range allKinds {
		if !covered[kind] {
			t.Errorf("no test case covers kind %q", kind)
		}
	}
}

// TestClassifyErrorDetail 覆盖 Detail 的提取规则：
// 去除 ANSI、优先第一处 "error:"、否则取最后一个非空行、折叠空白。
func TestClassifyErrorDetail(t *testing.T) {
	cases := []struct {
		name       string
		text       string
		wantKind   ErrorKind
		wantDetail string
	}{
		{
			name:       "strips-ansi-csi",
			text:       "\x1b[31mERROR:\x1b[0m \x1b[1mboom\x1b[0m happened\x1b[K",
			wantKind:   KindUnknown,
			wantDetail: "boom happened",
		},
		{
			name:       "strips-ansi-osc-hyperlink",
			text:       "\x1b]8;;https://example.com/foo\x07link\x1b]8;;\x07 ERROR: video unavailable",
			wantKind:   KindNotFound,
			wantDetail: "video unavailable",
		},
		{
			name:       "ansi-does-not-hide-keywords",
			text:       "\x1b[33mERROR: \x1b[31mHTTP Error 403: Forbidden\x1b[0m",
			wantKind:   KindCloudflare,
			wantDetail: "HTTP Error 403: Forbidden",
		},
		{
			name:       "last-non-empty-line-fallback",
			text:       "line one\n  line two   with   spaces  \n\n",
			wantKind:   KindUnknown,
			wantDetail: "line two with spaces",
		},
		{
			name:       "whitespace-only",
			text:       "   \n\t\n  ",
			wantKind:   KindUnknown,
			wantDetail: "",
		},
		{
			name:       "crlf-line-endings",
			text:       "ERROR: boom\r\nWARNING: more\r\n",
			wantKind:   KindUnknown,
			wantDetail: "boom",
		},
		{
			name:       "first-error-line-wins",
			text:       "ERROR: first failure\nERROR: second failure",
			wantKind:   KindUnknown,
			wantDetail: "first failure",
		},
		{
			// "error:" 出现在行中间也算（yt-dlp 有时把真正的错误包在 WARNING 里）。
			name:       "error-marker-mid-line",
			text:       "WARNING: request error: connection reset by peer\nERROR: giving up after retries",
			wantKind:   KindNetwork,
			wantDetail: "connection reset by peer",
		},
		{
			name:       "collapses-tabs",
			text:       "ERROR:\tfoo\t\tbar  baz",
			wantKind:   KindUnknown,
			wantDetail: "foo bar baz",
		},
		{
			name:       "empty-error-capture-is-skipped",
			text:       "ERROR:   \nERROR: the real one",
			wantKind:   KindUnknown,
			wantDetail: "the real one",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyError(tc.text, false)
			if got.Kind != tc.wantKind {
				t.Errorf("Kind = %q, want %q", got.Kind, tc.wantKind)
			}
			if got.Detail != tc.wantDetail {
				t.Errorf("Detail = %q, want %q", got.Detail, tc.wantDetail)
			}
			if n := utf8.RuneCountInString(got.Detail); n > 303 {
				t.Errorf("Detail has %d runes, want <= 303", n)
			}
		})
	}
}

// TestClassifyErrorDetailTruncation 覆盖 300 rune 上限与 UTF-8 边界。
func TestClassifyErrorDetailTruncation(t *testing.T) {
	t.Run("ascii-over-limit", func(t *testing.T) {
		got := ClassifyError("ERROR: "+strings.Repeat("a", 400), false)
		want := strings.Repeat("a", 300) + "..."
		if got.Detail != want {
			t.Errorf("Detail = %q, want 300 'a' + %q", got.Detail, "...")
		}
		if n := utf8.RuneCountInString(got.Detail); n != 303 {
			t.Errorf("Detail has %d runes, want 303", n)
		}
	})

	t.Run("exactly-at-limit-not-truncated", func(t *testing.T) {
		got := ClassifyError("ERROR: "+strings.Repeat("b", 300), false)
		if got.Detail != strings.Repeat("b", 300) {
			t.Errorf("Detail = %q, want exactly 300 'b' without ellipsis", got.Detail)
		}
	})

	t.Run("one-over-limit-truncated", func(t *testing.T) {
		got := ClassifyError("ERROR: "+strings.Repeat("c", 301), false)
		want := strings.Repeat("c", 300) + "..."
		if got.Detail != want {
			t.Errorf("Detail = %q, want 300 'c' + %q", got.Detail, "...")
		}
	})

	t.Run("multibyte-boundary-error-line", func(t *testing.T) {
		got := ClassifyError("ERROR: "+strings.Repeat("汉", 400), false)
		want := strings.Repeat("汉", 300) + "..."
		if got.Detail != want {
			t.Errorf("Detail = %q, want 300 汉 + %q", got.Detail, "...")
		}
		if !utf8.ValidString(got.Detail) {
			t.Errorf("Detail is not valid UTF-8: %q", got.Detail)
		}
		if strings.ContainsRune(got.Detail, utf8.RuneError) {
			t.Errorf("Detail contains U+FFFD replacement char: %q", got.Detail)
		}
	})

	t.Run("multibyte-boundary-fallback-line", func(t *testing.T) {
		got := ClassifyError(strings.Repeat("蓝", 400), false)
		want := strings.Repeat("蓝", 300) + "..."
		if got.Detail != want {
			t.Errorf("Detail = %q, want 300 蓝 + %q", got.Detail, "...")
		}
		if !utf8.ValidString(got.Detail) {
			t.Errorf("Detail is not valid UTF-8: %q", got.Detail)
		}
	})
}

// TestClassifyErrorI18nKey 覆盖 ErrorKind.I18nKey 的映射规则。
func TestClassifyErrorI18nKey(t *testing.T) {
	kinds := []ErrorKind{
		KindUnknown, KindAuth, KindMembership, KindDRM, KindGeo, KindCloudflare,
		KindRateLimit, KindUnsupported, KindNetwork, KindNotFound, KindStorage, KindExtractor,
	}
	for _, kind := range kinds {
		want := "errkind_" + string(kind)
		if got := kind.I18nKey(); got != want {
			t.Errorf("%q.I18nKey() = %q, want %q", kind, got, want)
		}
	}

	// 零值 / 未知分类统一回退到 unknown，避免 UI 显示空 key。
	if got := ErrorKind("").I18nKey(); got != "errkind_unknown" {
		t.Errorf(`ErrorKind("").I18nKey() = %q, want "errkind_unknown"`, got)
	}
	if got := ErrorKind("weird").I18nKey(); got != "errkind_weird" {
		t.Errorf(`ErrorKind("weird").I18nKey() = %q, want "errkind_weird"`, got)
	}
}

// TestClassifyErrorCookiesRetryOnlyForAccountKinds 明确 cookies 重试只发生在
// 账号/会员类错误上，且只在尚未使用 cookies 时。
func TestClassifyErrorCookiesRetryOnlyForAccountKinds(t *testing.T) {
	cases := []struct {
		name       string
		text       string
		hadCookies bool
		want       bool
	}{
		{"auth-no-cookies", `ERROR: [youtube] abc: Sign in to confirm your age`, false, true},
		{"auth-had-cookies", `ERROR: [youtube] abc: Sign in to confirm your age`, true, false},
		{"membership-no-cookies", `ERROR: [youtube] abc: Join this channel to get access`, false, true},
		{"membership-had-cookies", `ERROR: [youtube] abc: Join this channel to get access`, true, false},
		{"drm-no-cookies", `ERROR: [vimeo] 1: This video is DRM protected`, false, false},
		{"geo-no-cookies", `ERROR: [vimeo] 1: not available in your country`, false, false},
		{"cloudflare-no-cookies", `ERROR: HTTP Error 403: Forbidden`, false, false},
		{"rate-limit-no-cookies", `ERROR: HTTP Error 429: Too Many Requests`, false, false},
		{"network-no-cookies", `ERROR: HTTP Error 503: Service Unavailable`, false, false},
		{"not-found-no-cookies", `ERROR: [youtube] abc: Video unavailable`, false, false},
		{"unknown-no-cookies", "weird failure", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyError(tc.text, tc.hadCookies).RetryWithCookies; got != tc.want {
				t.Errorf("RetryWithCookies = %v, want %v", got, tc.want)
			}
		})
	}
}
