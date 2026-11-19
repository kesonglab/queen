package main

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
)

// SiteProfile 描述某个站点（按域名后缀匹配）的下载策略。
// 用户可以在 ~/.config/queen/sites.json 中覆盖任意字段。
type SiteProfile struct {
	Name          string   `json:"name,omitempty"`
	Match         []string `json:"match"`                    // 域名后缀，例如 "youtube.com"
	Cookies       string   `json:"cookies,omitempty"`        // "" 继承全局 | "auto" 自动探测 | "off" 不使用 | 浏览器名
	CookiesFile   string   `json:"cookies_file,omitempty"`   // cookies.txt（优先于 cookies）
	Impersonate   string   `json:"impersonate,omitempty"`    // "" 关闭 | chrome | chrome-136 | safari ...
	ExtractorArgs string   `json:"extractor_args,omitempty"` // 透传给 --extractor-args
	Referer       string   `json:"referer,omitempty"`
	UserAgent     string   `json:"user_agent,omitempty"`
	Fragments     int      `json:"fragments,omitempty"` // 0 = 使用全局
	Embed         *bool    `json:"embed,omitempty"`     // nil = 使用全局
	Privacy       bool     `json:"privacy,omitempty"`   // 通知/日志中隐藏 URL 与标题
}

// SiteDB 内置站点档案 + 用户覆盖。
type SiteDB struct {
	builtin  []SiteProfile
	user     []SiteProfile
	warnings []string
}

// builtinProfiles 内置站点策略。原则：
//   - 需要登录/年龄验证的站点 → cookies: auto
//   - Cloudflare 常见站点 → impersonate
//   - 成人内容站点 → privacy
//   - YouTube/HLS 友好站点 → 更高分片并发
var builtinProfiles = []SiteProfile{
	{Name: "YouTube", Match: []string{"youtube.com", "youtu.be", "youtube-nocookie.com"}, Fragments: 16},
	{Name: "Bilibili", Match: []string{"bilibili.com", "b23.tv"}, Cookies: "auto", Referer: "https://www.bilibili.com", Fragments: 8},
	{Name: "X / Twitter", Match: []string{"x.com", "twitter.com", "t.co"}, Cookies: "auto"},
	{Name: "Instagram", Match: []string{"instagram.com", "instagr.am"}, Cookies: "auto"},
	{Name: "Facebook", Match: []string{"facebook.com", "fb.watch", "fb.com"}, Cookies: "auto"},
	{Name: "TikTok", Match: []string{"tiktok.com"}, Cookies: "auto", Referer: "https://www.tiktok.com/"},
	{Name: "Douyin", Match: []string{"douyin.com", "iesdouyin.com"}, Cookies: "auto", Referer: "https://www.douyin.com/"},
	{Name: "Twitch", Match: []string{"twitch.tv"}, Fragments: 8},
	{Name: "Vimeo", Match: []string{"vimeo.com"}},
	{Name: "Reddit", Match: []string{"reddit.com", "redd.it", "v.redd.it"}, Cookies: "auto"},
	{Name: "Niconico", Match: []string{"nicovideo.jp", "nico.ms"}, Cookies: "auto"},
	{Name: "SoundCloud", Match: []string{"soundcloud.com"}},
	{
		Name:        "Rule34Video",
		Match:       []string{"rule34video.com"},
		Cookies:     "auto",
		Impersonate: "chrome",
		Embed:       boolPtr(false),
		Privacy:     true,
		Fragments:   4,
	},
}

func boolPtr(b bool) *bool { return &b }

// defaultSiteDB 返回只含内置档案的 SiteDB（用于无配置场景与测试）。
func defaultSiteDB() *SiteDB {
	return &SiteDB{builtin: append([]SiteProfile(nil), builtinProfiles...)}
}

// LoadSiteDB 载入内置档案 + 用户 sites.json（可选）。
func LoadSiteDB() *SiteDB {
	db := defaultSiteDB()
	data, err := os.ReadFile(sitesPath())
	if err != nil {
		return db
	}
	profiles, err := parseSiteProfiles(data)
	if err != nil {
		db.warnings = append(db.warnings, err.Error())
		return db
	}
	db.user = profiles
	return db
}

// parseSiteProfiles 同时接受 `[...]` 与 `{"profiles":[...]}` 两种格式。
func parseSiteProfiles(data []byte) ([]SiteProfile, error) {
	var arr []SiteProfile
	if err := json.Unmarshal(data, &arr); err == nil {
		return arr, nil
	}
	var wrapper struct {
		Profiles []SiteProfile `json:"profiles"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Profiles, nil
}

// Lookup 返回与 host 匹配的合并后档案；没有匹配时返回零值。
// 规则：域名后缀匹配（"youtube.com" 匹配 "www.youtube.com"、"m.youtube.com"），
// 取最长后缀；用户档案覆盖内置档案的同名字段。
func (db *SiteDB) Lookup(host string) SiteProfile {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return SiteProfile{}
	}
	base := bestMatch(db.builtin, host)
	over := bestMatch(db.user, host)
	if over == nil {
		if base == nil {
			return SiteProfile{}
		}
		return *base
	}
	if base == nil {
		return *over
	}
	return mergeProfile(*base, *over)
}

func bestMatch(profiles []SiteProfile, host string) *SiteProfile {
	var best *SiteProfile
	bestLen := -1
	for i := range profiles {
		for _, m := range profiles[i].Match {
			m = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(m, ".")))
			if m == "" {
				continue
			}
			if !hostMatches(host, m) {
				continue
			}
			if len(m) > bestLen {
				bestLen = len(m)
				p := profiles[i]
				best = &p
			}
		}
	}
	return best
}

// hostMatches 判断 host 是否等于 suffix 或为其子域。
func hostMatches(host, suffix string) bool {
	return host == suffix || strings.HasSuffix(host, "."+suffix)
}

// mergeProfile 用 over 中的非零字段覆盖 base。
func mergeProfile(base, over SiteProfile) SiteProfile {
	out := base
	if over.Name != "" {
		out.Name = over.Name
	}
	if len(over.Match) > 0 {
		out.Match = over.Match
	}
	if over.Cookies != "" {
		out.Cookies = over.Cookies
	}
	if over.CookiesFile != "" {
		out.CookiesFile = over.CookiesFile
	}
	if over.Impersonate != "" {
		out.Impersonate = over.Impersonate
	}
	if over.ExtractorArgs != "" {
		out.ExtractorArgs = over.ExtractorArgs
	}
	if over.Referer != "" {
		out.Referer = over.Referer
	}
	if over.UserAgent != "" {
		out.UserAgent = over.UserAgent
	}
	if over.Fragments != 0 {
		out.Fragments = over.Fragments
	}
	if over.Embed != nil {
		out.Embed = over.Embed
	}
	if over.Privacy {
		out.Privacy = true
	}
	return out
}

// List 返回内置 + 用户档案，按名称排序，便于在「站点档案」页面展示。
func (db *SiteDB) List() []SiteProfile {
	out := make([]SiteProfile, 0, len(db.builtin)+len(db.user))
	out = append(out, db.builtin...)
	out = append(out, db.user...)
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// UserProfileCount 返回用户自定义档案数量。
func (db *SiteDB) UserProfileCount() int { return len(db.user) }

// Warnings 返回用户档案解析过程中的问题。
func (db *SiteDB) Warnings() []string { return db.warnings }

// EffectiveSite 是全局配置与站点档案合并后的最终参数。
type EffectiveSite struct {
	Profile        SiteProfile
	CookiesBrowser string
	CookiesFile    string
	Impersonate    string
	ExtractorArgs  string
	Referer        string
	UserAgent      string
	Fragments      int
	Embed          bool
	Privacy        bool
}

// Effective 合并全局配置与该 URL 对应的站点档案。
func (c *Config) Effective(db *SiteDB, url string) EffectiveSite {
	prof := db.Lookup(hostOf(url))
	eff := EffectiveSite{
		Profile:        prof,
		CookiesBrowser: c.CookieBrowser,
		CookiesFile:    c.CookiesFile,
		Impersonate:    c.Impersonate,
		ExtractorArgs:  c.ExtractorArgs,
		Referer:        c.Referer,
		UserAgent:      c.UserAgent,
		Fragments:      c.Fragments,
		Embed:          c.Embed,
		Privacy:        c.PrivacyMode,
	}
	// cookie 策略
	switch strings.ToLower(strings.TrimSpace(prof.Cookies)) {
	case "off":
		eff.CookiesBrowser = ""
		eff.CookiesFile = ""
	case "auto":
		if c.CookieBrowser != "" {
			eff.CookiesBrowser = c.CookieBrowser
		} else {
			eff.CookiesBrowser = detectBrowser()
		}
	case "":
		// 继承全局
	default:
		eff.CookiesBrowser = prof.Cookies
	}
	if prof.CookiesFile != "" {
		eff.CookiesFile = expandHome(prof.CookiesFile)
		eff.CookiesBrowser = "" // 文件优先，避免同时读浏览器
	}
	if eff.CookiesFile != "" {
		eff.CookiesBrowser = ""
	}
	if prof.Impersonate != "" {
		eff.Impersonate = prof.Impersonate
	}
	if prof.ExtractorArgs != "" {
		eff.ExtractorArgs = prof.ExtractorArgs
	}
	if prof.Referer != "" {
		eff.Referer = prof.Referer
	}
	if prof.UserAgent != "" {
		eff.UserAgent = prof.UserAgent
	}
	if prof.Fragments > 0 {
		eff.Fragments = prof.Fragments
	}
	if prof.Embed != nil {
		eff.Embed = *prof.Embed
	}
	if prof.Privacy {
		eff.Privacy = true
	}
	return eff
}
