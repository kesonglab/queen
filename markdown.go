package main

// A small, dependency-free Markdown renderer used by the changelog view.
//
// RenderMarkdown turns Markdown-ish source into lines that are already styled
// by the caller-supplied MDStyles functions (lipgloss etc.).  The renderer
// itself never emits ANSI escape sequences.  Wrapping happens on the visible
// text before styling, so escape sequences never influence the layout.

import (
	"regexp"
	"strings"
	"unicode"
)

// MDStyles carries the style functions for each Markdown element. A nil func
// means "no styling" (identity).
type MDStyles struct {
	Heading1 func(string) string // "# "    heading level 1
	Heading2 func(string) string // "## "   heading level 2
	Heading3 func(string) string // "### "  heading level 3
	Bold     func(string) string
	Italic   func(string) string
	Code     func(string) string // inline code and fenced-code lines
	Link     func(string) string
	Bullet   func(string) string // the bullet marker itself, e.g. "• "
	Quote    func(string) string // the quote marker "│ "
	Dim      func(string) string // separators, fenced-code text, secondary info
	Wrap     int                 // wrap width in display columns; <=0 disables wrapping
}

const (
	mdBulletMarker = "• "
	mdQuoteMarker  = "│ "
	mdRuleWidth    = 40
	mdCodeIndent   = "    "
)

var (
	mdHeadingRE = regexp.MustCompile(`^(#{1,3})\s+(.*)$`)
	mdQuoteRE   = regexp.MustCompile(`^>\s?(.*)$`)
	mdBulletRE  = regexp.MustCompile(`^(\s*)[-*+]\s+(.*)$`)
	mdNumberRE  = regexp.MustCompile(`^(\s*)(\d+)[.)]\s+(.*)$`)
)

// RenderMarkdown converts Markdown source into rendered lines.
// mdJoinContinuations 把「缩进的续行」并入上一行（Markdown 惰性续行），
// 避免仓库里的硬折行 changelog 在渲染时丢失列表缩进。
func mdJoinContinuations(src string) string {
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		isContinuation := len(line) >= 2 && (line[0] == ' ' || line[0] == '\t') &&
			trimmed != "" &&
			!strings.HasPrefix(trimmed, "- ") && !strings.HasPrefix(trimmed, "* ") &&
			!strings.HasPrefix(trimmed, "+ ") && !strings.HasPrefix(trimmed, "#") &&
			!strings.HasPrefix(trimmed, ">") && !strings.HasPrefix(trimmed, "```") &&
			!mdNumberRE.MatchString(trimmed)
		if isContinuation && len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
			// 中文硬折行拼接时不要插入空格（英文需要空格）
			sep := " "
			prev := strings.TrimRight(out[len(out)-1], " ")
			// 两侧都是 CJK，或前一行以中文标点结尾时，不加空格；
			// 中英文交界处保留空格（"NO_COLOR 输出" 这种排版更好看）。
			if mdCJKPunctTail(prev) || (mdCJKTail(prev) && mdCJKHead(trimmed)) {
				sep = ""
			}
			out[len(out)-1] = prev + sep + trimmed
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// mdCJK 判断 rune 是否属于中文/日文/全角标点区间。
func mdCJK(r rune) bool {
	switch {
	case r >= 0x2E80 && r <= 0x9FFF, // CJK radicals, kana, unified ideographs
		r >= 0xF900 && r <= 0xFAFF,
		r >= 0xFE30 && r <= 0xFE4F,
		r >= 0xFF00 && r <= 0xFFEF, // fullwidth forms
		r >= 0x20000 && r <= 0x3FFFF:
		return true
	}
	return false
}

// mdCJKPunctTail 判断字符串结尾是否为中文标点（合并硬折行时不留空格）。
func mdCJKPunctTail(s string) bool {
	runes := []rune(s)
	if len(runes) == 0 {
		return false
	}
	switch runes[len(runes)-1] {
	case '、', '。', '，', '．', '；', '：', '！', '？', '「', '」', '『', '』',
		'（', '）', '【', '】', '《', '》', '…', '—', '～', '·', '］', '｝':
		return true
	}
	return false
}

// mdCJKTail 判断字符串结尾是否为 CJK 字符。
func mdCJKTail(s string) bool {
	runes := []rune(s)
	if len(runes) == 0 {
		return false
	}
	return mdCJK(runes[len(runes)-1])
}

// mdCJKHead 判断字符串开头是否为 CJK 字符。
func mdCJKHead(s string) bool {
	for _, r := range s {
		return mdCJK(r)
	}
	return false
}

func RenderMarkdown(src string, st MDStyles) []string {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	src = mdJoinContinuations(src)
	lines := strings.Split(src, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}

	out := make([]string, 0, len(lines))
	inCode := false
	for _, line := range lines {
		// A fence line toggles code mode and is never rendered itself; this is
		// checked first even while inside a block so the closing fence works.
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inCode = !inCode
			continue
		}
		if inCode {
			out = append(out, mdRenderCodeLine(line, st)...)
			continue
		}

		if m := mdHeadingRE.FindStringSubmatch(line); m != nil {
			out = append(out, mdRenderHeading(m[2], len(m[1]), st)...)
			continue
		}
		if mdIsHorizontalRule(line) {
			n := mdRuleWidth
			if st.Wrap > 0 && st.Wrap < n {
				n = st.Wrap
			}
			out = append(out, mdStyle(st.Dim, strings.Repeat("─", n)))
			continue
		}
		if m := mdQuoteRE.FindStringSubmatch(line); m != nil {
			out = append(out, mdRenderQuote(m[1], st)...)
			continue
		}
		if m := mdBulletRE.FindStringSubmatch(line); m != nil {
			out = append(out, mdRenderBullet(m[1], m[2], st)...)
			continue
		}
		if m := mdNumberRE.FindStringSubmatch(line); m != nil {
			out = append(out, mdRenderNumbered(m[1], m[2], m[3], st)...)
			continue
		}
		out = append(out, mdRenderParagraph(line, st)...)
	}

	if len(out) == 0 {
		return []string{""}
	}
	return out
}

// ---------- block renderers ----------

func mdRenderHeading(text string, level int, st MDStyles) []string {
	text = strings.TrimRight(text, " \t#")
	var style func(string) string
	switch level {
	case 1:
		style = st.Heading1
	case 2:
		style = st.Heading2
	default:
		style = st.Heading3
	}
	segs := []mdSegment{{text: text}}
	return mdAssemble(mdWrapLines(segs, st.Wrap, st.Wrap, st.Wrap), "", "", style)
}

func mdRenderCodeLine(line string, st MDStyles) []string {
	segs := []mdSegment{{text: line}}
	lines := mdWrapLines(segs, st.Wrap, st.Wrap-len(mdCodeIndent), st.Wrap-len(mdCodeIndent))
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = mdStyle(st.Dim, mdCodeIndent+mdRenderSegments(l))
	}
	return out
}

func mdRenderQuote(text string, st MDStyles) []string {
	segs := mdParseInline(text, st)
	markerW := mdDisplayWidth(mdQuoteMarker)
	lines := mdWrapLines(segs, st.Wrap, st.Wrap-markerW, st.Wrap-markerW)
	return mdAssemble(lines, mdStyle(st.Quote, mdQuoteMarker), strings.Repeat(" ", markerW), st.Dim)
}

func mdRenderBullet(indent, text string, st MDStyles) []string {
	segs := mdParseInline(text, st)
	markerW := mdDisplayWidth(mdBulletMarker)
	indentW := mdDisplayWidth(indent)
	budget := st.Wrap - indentW - markerW
	lines := mdWrapLines(segs, st.Wrap, budget, budget)
	first := indent + mdStyle(st.Bullet, mdBulletMarker)
	cont := indent + strings.Repeat(" ", markerW)
	return mdAssemble(lines, first, cont, nil)
}

func mdRenderNumbered(indent, num, text string, st MDStyles) []string {
	segs := mdParseInline(text, st)
	marker := num + ". "
	markerW := mdDisplayWidth(marker)
	indentW := mdDisplayWidth(indent)
	budget := st.Wrap - indentW - markerW
	lines := mdWrapLines(segs, st.Wrap, budget, budget)
	first := indent + marker
	cont := indent + strings.Repeat(" ", markerW)
	return mdAssemble(lines, first, cont, nil)
}

func mdRenderParagraph(line string, st MDStyles) []string {
	segs := mdParseInline(line, st)
	return mdAssemble(mdWrapLines(segs, st.Wrap, st.Wrap, st.Wrap), "", "", nil)
}

// mdIsHorizontalRule reports whether line is a Markdown horizontal rule, i.e.
// its trimmed form consists solely of at least three '-', '*' or '_' runes.
func mdIsHorizontalRule(line string) bool {
	t := strings.TrimSpace(line)
	if len(t) < 3 {
		return false
	}
	for _, r := range t {
		if r != '-' && r != '*' && r != '_' {
			return false
		}
	}
	return true
}

// mdAssemble attaches the (already styled) line prefixes to wrapped content and
// optionally wraps every content line in inner (e.g. Dim for quotes).
func mdAssemble(lines [][]mdSegment, firstPrefix, contPrefix string, inner func(string) string) []string {
	out := make([]string, len(lines))
	for i, segs := range lines {
		prefix := contPrefix
		if i == 0 {
			prefix = firstPrefix
		}
		text := mdRenderSegments(segs)
		if inner != nil {
			text = inner(text)
		}
		out[i] = prefix + text
	}
	return out
}

// ---------- wrapping ----------

// mdSegment is a piece of visible text plus the style funcs applied to it.
// funcs[0] is the outermost style (rendered last).
type mdSegment struct {
	text  string
	funcs []func(string) string
}

func (s mdSegment) render() string {
	out := s.text
	for i := len(s.funcs) - 1; i >= 0; i-- {
		out = mdStyle(s.funcs[i], out)
	}
	return out
}

type mdToken struct {
	seg   mdSegment
	space bool
}

// mdWrapLines greedily wraps segs into lines of at most the given display
// width. firstBudget/contBudget are the display columns available for content
// on the first and on continuation lines (they exclude the line prefix).
func mdWrapLines(segs []mdSegment, wrap, firstBudget, contBudget int) [][]mdSegment {
	if wrap <= 0 {
		return [][]mdSegment{segs}
	}
	if firstBudget < 1 {
		firstBudget = 1
	}
	if contBudget < 1 {
		contBudget = 1
	}
	budget := func(lines [][]mdSegment) int {
		if len(lines) == 0 {
			return firstBudget
		}
		return contBudget
	}

	var (
		lines    [][]mdSegment
		cur      []mdSegment
		curW     int
		pending  []mdSegment
		pendingW int
	)
	flush := func() {
		lines = append(lines, cur)
		cur, curW = nil, 0
		pending, pendingW = nil, 0
	}

	for _, tok := range mdTokenize(segs) {
		w := mdDisplayWidth(tok.seg.text)
		if tok.space {
			pending = append(pending, tok.seg)
			pendingW += w
			continue
		}
		b := budget(lines)
		if curW > 0 && curW+pendingW+w > b {
			flush()
			b = budget(lines)
		}
		// Spaces that still fit stay on the current line; a break drops them
		// and leading whitespace never starts a wrapped line.
		if curW > 0 {
			cur = append(cur, pending...)
			curW += pendingW
		}
		pending, pendingW = nil, 0
		if w > b {
			// A single token wider than the line: hard-break it by runes.
			for ci, chunk := range mdHardBreak(tok.seg, b) {
				if ci > 0 {
					flush()
				}
				cur = append(cur, chunk)
				curW += mdDisplayWidth(chunk.text)
			}
			continue
		}
		cur = append(cur, tok.seg)
		curW += w
	}
	flush()
	return lines
}

// mdTokenize splits segments into word and whitespace tokens.
func mdTokenize(segs []mdSegment) []mdToken {
	var toks []mdToken
	for _, seg := range segs {
		start := -1
		var curSpace bool
		for i, r := range seg.text {
			sp := unicode.IsSpace(r)
			if start < 0 {
				start, curSpace = i, sp
				continue
			}
			if sp != curSpace {
				toks = append(toks, mdToken{
					seg:   mdSegment{text: seg.text[start:i], funcs: seg.funcs},
					space: curSpace,
				})
				start, curSpace = i, sp
			}
		}
		if start >= 0 {
			toks = append(toks, mdToken{
				seg:   mdSegment{text: seg.text[start:], funcs: seg.funcs},
				space: curSpace,
			})
		}
	}
	return toks
}

// mdHardBreak splits a single segment into chunks no wider than max display
// columns. It never returns an empty slice for non-empty text.
func mdHardBreak(seg mdSegment, max int) []mdSegment {
	if max < 1 {
		max = 1
	}
	var chunks []mdSegment
	var b strings.Builder
	w := 0
	for _, r := range seg.text {
		rw := mdRuneWidth(r)
		if w > 0 && w+rw > max {
			chunks = append(chunks, mdSegment{text: b.String(), funcs: seg.funcs})
			b.Reset()
			w = 0
		}
		b.WriteRune(r)
		w += rw
	}
	if b.Len() > 0 {
		chunks = append(chunks, mdSegment{text: b.String(), funcs: seg.funcs})
	}
	return chunks
}

func mdRenderSegments(segs []mdSegment) string {
	var b strings.Builder
	for _, seg := range segs {
		b.WriteString(seg.render())
	}
	return b.String()
}

// ---------- inline parsing ----------

// mdParseInline converts one logical line of inline Markdown into styled
// segments. Markers without a matching closer are kept literally.
func mdParseInline(s string, st MDStyles) []mdSegment {
	var (
		segs []mdSegment
		lit  strings.Builder
	)
	flush := func() {
		if lit.Len() > 0 {
			segs = append(segs, mdSegment{text: lit.String()})
			lit.Reset()
		}
	}

	for i := 0; i < len(s); {
		switch s[i] {
		case '`':
			if j := strings.IndexByte(s[i+1:], '`'); j >= 0 {
				flush()
				segs = append(segs, mdSegment{text: s[i+1 : i+1+j], funcs: mdWrapFunc(st.Code)})
				i += j + 2
				continue
			}
		case '*', '_':
			// **bold** / __bold__
			if i+1 < len(s) && s[i+1] == s[i] {
				if j := strings.Index(s[i+2:], s[i:i+2]); j >= 0 && j > 0 {
					inner := s[i+2 : i+2+j]
					flush()
					segs = append(segs, mdAddStyle(mdParseInline(inner, st), st.Bold)...)
					i += j + 4
					continue
				}
			}
			// *italic* / _italic_
			if j := strings.IndexByte(s[i+1:], s[i]); j > 0 {
				inner := s[i+1 : i+1+j]
				flush()
				segs = append(segs, mdAddStyle(mdParseInline(inner, st), st.Italic)...)
				i += j + 2
				continue
			}
		case '[':
			if text, next, ok := mdParseLink(s, i); ok {
				flush()
				segs = append(segs, mdSegment{text: text, funcs: mdWrapFunc(st.Link)})
				i = next
				continue
			}
		case 'h':
			if strings.HasPrefix(s[i:], "http://") || strings.HasPrefix(s[i:], "https://") {
				j := i
				for j < len(s) && !unicode.IsSpace(rune(s[j])) {
					j++
				}
				flush()
				segs = append(segs, mdSegment{text: s[i:j], funcs: mdWrapFunc(st.Link)})
				i = j
				continue
			}
		}
		lit.WriteByte(s[i])
		i++
	}
	flush()
	return segs
}

// mdParseLink parses "[text](url)" starting at i and returns the link text,
// the index just past the closing ')' and whether the span was a valid link.
// The URL itself is intentionally dropped.
func mdParseLink(s string, i int) (string, int, bool) {
	closeText := strings.IndexByte(s[i+1:], ']')
	if closeText < 0 {
		return "", i, false
	}
	closeText += i + 1
	if closeText+1 >= len(s) || s[closeText+1] != '(' {
		return "", i, false
	}
	closeURL := strings.IndexByte(s[closeText+2:], ')')
	if closeURL < 0 {
		return "", i, false
	}
	next := closeText + 2 + closeURL + 1
	return s[i+1 : closeText], next, true
}

// mdAddStyle puts fn around every segment (fn becomes the outermost style).
func mdAddStyle(segs []mdSegment, fn func(string) string) []mdSegment {
	if fn == nil {
		return segs
	}
	for i := range segs {
		funcs := make([]func(string) string, 0, len(segs[i].funcs)+1)
		funcs = append(funcs, fn)
		segs[i].funcs = append(funcs, segs[i].funcs...)
	}
	return segs
}

func mdWrapFunc(fn func(string) string) []func(string) string {
	if fn == nil {
		return nil
	}
	return []func(string) string{fn}
}

// ---------- display width ----------

func mdStyle(fn func(string) string, s string) string {
	if fn == nil {
		return s
	}
	return fn(s)
}

func mdDisplayWidth(s string) int {
	w := 0
	for _, r := range s {
		w += mdRuneWidth(r)
	}
	return w
}

// mdRuneWidth returns the number of terminal columns a rune occupies:
// combining marks and control/format characters are zero-width, East Asian
// Wide/Fullwidth characters are two columns and everything else is one.
func mdRuneWidth(r rune) int {
	if r == 0 {
		return 0
	}
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Cf, r) {
		return 0
	}
	if unicode.Is(unicode.Cc, r) {
		return 0
	}
	if mdIsWideRune(r) {
		return 2
	}
	return 1
}

func mdIsWideRune(r rune) bool {
	// Halfwidth forms share the CJK scripts but occupy a single column.
	if (r >= 0xFF61 && r <= 0xFFDC) || (r >= 0xFFE8 && r <= 0xFFEE) {
		return false
	}
	if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul, unicode.Bopomofo, unicode.Yi) {
		return true
	}
	switch {
	case r >= 0x1100 && r <= 0x115F: // Hangul Jamo initial consonants
		return true
	case r == 0x2329 || r == 0x232A: // angle brackets
		return true
	case r >= 0x2E80 && r <= 0x303E: // CJK radicals and symbols/punctuation
		return true
	case r >= 0x3041 && r <= 0x33FF: // kana, Bopomofo, CJK compatibility
		return true
	case r >= 0x3400 && r <= 0x4DBF: // CJK unified ideographs extension A
		return true
	case r >= 0x4E00 && r <= 0x9FFF: // CJK unified ideographs
		return true
	case r >= 0xA000 && r <= 0xA4CF: // Yi syllables and radicals
		return true
	case r >= 0xA960 && r <= 0xA97F: // Hangul Jamo extended-A
		return true
	case r >= 0xAC00 && r <= 0xD7A3: // Hangul syllables
		return true
	case r >= 0xF900 && r <= 0xFAFF: // CJK compatibility ideographs
		return true
	case r >= 0xFE10 && r <= 0xFE19: // vertical forms
		return true
	case r >= 0xFE30 && r <= 0xFE6F: // CJK compatibility forms
		return true
	case r >= 0xFF00 && r <= 0xFF60: // fullwidth forms
		return true
	case r >= 0xFFE0 && r <= 0xFFE6: // fullwidth signs
		return true
	case r >= 0x1B000 && r <= 0x1B16F: // kana supplement/extended
		return true
	case r >= 0x1F300 && r <= 0x1F64F: // emoji
		return true
	case r >= 0x1F900 && r <= 0x1F9FF: // supplemental symbols and pictographs
		return true
	case r >= 0x20000 && r <= 0x3FFFD: // CJK extensions B+
		return true
	}
	return false
}
