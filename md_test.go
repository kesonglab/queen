package main

import (
	"strings"
	"testing"
)

// ---------- test helpers ----------

// mdTestIdentityStyles returns MDStyles with no styling at all.
func mdTestIdentityStyles(wrap int) MDStyles {
	return MDStyles{Wrap: wrap}
}

// mdTestTagStyles returns styles that wrap each element in an XML-ish tag so
// tests can assert the structure without any terminal dependency.
func mdTestTagStyles(wrap int) MDStyles {
	tag := func(name string) func(string) string {
		return func(s string) string { return "<" + name + ">" + s + "</" + name + ">" }
	}
	return MDStyles{
		Heading1: tag("h1"),
		Heading2: tag("h2"),
		Heading3: tag("h3"),
		Bold:     tag("b"),
		Italic:   tag("i"),
		Code:     tag("c"),
		Link:     tag("l"),
		Bullet:   tag("u"),
		Quote:    tag("q"),
		Dim:      tag("d"),
		Wrap:     wrap,
	}
}

// mdTestWidth computes display width for the character repertoire used in the
// tests (ASCII, CJK ideographs, CJK punctuation and fullwidth forms).
func mdTestWidth(s string) int {
	w := 0
	for _, r := range s {
		switch {
		case r >= 0x4E00 && r <= 0x9FFF,
			r >= 0x3040 && r <= 0x30FF,
			r >= 0x3000 && r <= 0x303F,
			r >= 0xFE30 && r <= 0xFE4F,
			r >= 0xFF00 && r <= 0xFF60:
			w += 2
		default:
			w++
		}
	}
	return w
}

// mdTestStripTags removes the tags produced by mdTestTagStyles.
func mdTestStripTags(s string) string {
	for _, name := range []string{"h1", "h2", "h3", "b", "i", "c", "l", "u", "q", "d"} {
		s = strings.ReplaceAll(s, "<"+name+">", "")
		s = strings.ReplaceAll(s, "</"+name+">", "")
	}
	return s
}

func mdAssertLines(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("line count = %d, want %d\ngot:  %q\nwant: %q", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func mdAssertMaxWidth(t *testing.T, lines []string, wrap int) {
	t.Helper()
	for i, line := range lines {
		line = mdTestStripTags(line)
		if w := mdTestWidth(line); w > wrap {
			t.Errorf("line %d = %q has display width %d > wrap %d", i, line, w, wrap)
		}
	}
}

// ---------- headings ----------

func TestRenderMarkdownHeadings(t *testing.T) {
	got := RenderMarkdown("# Title\n## Sub ##\n### Deep ###\n# 中文标题", mdTestTagStyles(0))
	want := []string{
		"<h1>Title</h1>",
		"<h2>Sub</h2>",
		"<h3>Deep</h3>",
		"<h1>中文标题</h1>",
	}
	mdAssertLines(t, got, want)

	// Four or more '#' (or a '#' without a following space) is not a heading.
	got = RenderMarkdown("#### not a heading\n#nospace", mdTestIdentityStyles(0))
	mdAssertLines(t, got, []string{"#### not a heading", "#nospace"})

	// Headings still wrap when they are wider than Wrap.
	wide := strings.Repeat("中", 12) // 24 columns
	got = RenderMarkdown("## "+wide, mdTestIdentityStyles(10))
	mdAssertLines(t, got, []string{
		strings.Repeat("中", 5),
		strings.Repeat("中", 5),
		strings.Repeat("中", 2),
	})
	mdAssertMaxWidth(t, got, 10)
}

// ---------- lists ----------

func TestRenderMarkdownBulletAndNumberedLists(t *testing.T) {
	src := "- one\n* two\n+ three\n  - nested\n1. first\n2) second\n   3. third"
	got := RenderMarkdown(src, mdTestIdentityStyles(0))
	want := []string{
		"• one",
		"• two",
		"• three",
		"  • nested",
		"1. first",
		"2. second",
		"   3. third",
	}
	mdAssertLines(t, got, want)

	// The bullet marker itself is styled; numbered markers are plain and the
	// ')' delimiter is normalised to '.'.
	got = RenderMarkdown("- item\n- **bold** `code`\n1) numbered", mdTestTagStyles(0))
	want = []string{
		"<u>• </u>item",
		"<u>• </u><b>bold</b> <c>code</c>",
		"1. numbered",
	}
	mdAssertLines(t, got, want)

	// A rule-like line is not a bullet.
	got = RenderMarkdown("- - -", mdTestIdentityStyles(0))
	mdAssertLines(t, got, []string{"• - -"})
}

// ---------- block quotes ----------

func TestRenderMarkdownBlockquote(t *testing.T) {
	got := RenderMarkdown("> quoted text\n>\n> second line", mdTestIdentityStyles(0))
	mdAssertLines(t, got, []string{"│ quoted text", "│ ", "│ second line"})

	// The quote marker is styled and the quoted text is dimmed.
	got = RenderMarkdown("> **bold** quote", mdTestTagStyles(0))
	mdAssertLines(t, got, []string{"<q>│ </q><d><b>bold</b> quote</d>"})
}

// ---------- horizontal rules ----------

func TestRenderMarkdownHorizontalRule(t *testing.T) {
	got := RenderMarkdown("---\n***\n___", mdTestIdentityStyles(0))
	rule := strings.Repeat("─", 40)
	mdAssertLines(t, got, []string{rule, rule, rule})

	// The rule is shortened to Wrap when Wrap is smaller than 40.
	got = RenderMarkdown("---", mdTestIdentityStyles(12))
	mdAssertLines(t, got, []string{strings.Repeat("─", 12)})

	// ...and dimmed when a Dim style is supplied.
	got = RenderMarkdown("---", mdTestTagStyles(0))
	mdAssertLines(t, got, []string{"<d>" + rule + "</d>"})
}

// ---------- fenced code ----------

func TestRenderMarkdownFencedCode(t *testing.T) {
	src := "before\n```go\n**not bold** *not italic*\n`also not code`\n\n```\nafter"
	got := RenderMarkdown(src, mdTestIdentityStyles(0))
	want := []string{
		"before",
		"    **not bold** *not italic*",
		"    `also not code`",
		"    ",
		"after",
	}
	mdAssertLines(t, got, want)

	// Fence lines themselves are never emitted, and code lines are dimmed.
	got = RenderMarkdown("```\ncode\n```", mdTestTagStyles(0))
	mdAssertLines(t, got, []string{"<d>    code</d>"})

	// Long code lines are hard-wrapped but keep the four-space indent.
	got = RenderMarkdown("```\nabcdefghijkl\n```", mdTestIdentityStyles(10))
	mdAssertLines(t, got, []string{"    abcdef", "    ghijkl"})
	mdAssertMaxWidth(t, got, 10)
}

// ---------- inline styles ----------

func TestRenderMarkdownInlineStyles(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"bold", "**bold**", "<b>bold</b>"},
		{"bold underscore", "__bold__", "<b>bold</b>"},
		{"italic", "*italic*", "<i>italic</i>"},
		{"italic underscore", "_italic_", "<i>italic</i>"},
		{"code", "`code`", "<c>code</c>"},
		{"link", "[text](https://example.com/page)", "<l>text</l>"},
		{"bare https url", "see https://example.com/x now", "see <l>https://example.com/x</l> now"},
		{"bare http url", "http://example.com", "<l>http://example.com</l>"},
		{"adjacent markers", "**a** *b* `c` [d](https://e.f)", "<b>a</b> <i>b</i> <c>c</c> <l>d</l>"},
		{"adjacent no space", "**a**`b`[c](d)", "<b>a</b><c>b</c><l>c</l>"},
		{"nested bold italic", "**outer *inner* end**", "<b>outer </b><b><i>inner</i></b><b> end</b>"},
		{"code inside bold", "**bold `code`**", "<b>bold </b><b><c>code</c></b>"},
		{"unmatched bold", "a ** b", "a ** b"},
		{"unmatched italic", "a *b", "a *b"},
		{"unmatched underscore", "a __b", "a __b"},
		{"unmatched bracket", "[not a link", "[not a link"},
		{"markdown inside code span", "`**not bold** *not italic*`", "<c>**not bold** *not italic*</c>"},
		{"link inside code span", "`[x](https://y.z)`", "<c>[x](https://y.z)</c>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderMarkdown(tc.src, mdTestTagStyles(0))
			mdAssertLines(t, got, []string{tc.want})
		})
	}
}

// ---------- blank lines and line endings ----------

func TestRenderMarkdownBlankLinesAndCRLF(t *testing.T) {
	// Blank lines inside the document are preserved...
	got := RenderMarkdown("a\n\nb\n\n\n", mdTestIdentityStyles(0))
	mdAssertLines(t, got, []string{"a", "", "b"})

	// ...while trailing blank lines are trimmed.
	got = RenderMarkdown("a\n   \n\t\n\n", mdTestIdentityStyles(0))
	mdAssertLines(t, got, []string{"a"})

	// CRLF input is normalised.
	got = RenderMarkdown("# A\r\n\r\n**b**\r\n", mdTestTagStyles(0))
	mdAssertLines(t, got, []string{"<h1>A</h1>", "", "<b>b</b>"})

	// A blank line inside a fenced block is still part of the block.
	got = RenderMarkdown("```\n\n```", mdTestIdentityStyles(0))
	mdAssertLines(t, got, []string{"    "})
}

// ---------- wrapping ----------

func TestRenderMarkdownWrapEnglish(t *testing.T) {
	src := "the quick brown fox jumps over the lazy dog"
	want := []string{"the quick brown fox", "jumps over the lazy", "dog"}
	got := RenderMarkdown(src, mdTestIdentityStyles(20))
	mdAssertLines(t, got, want)
	mdAssertMaxWidth(t, got, 20)
	if joined := strings.Join(got, " "); joined != src {
		t.Errorf("wrapped lines lost data: %q != %q", joined, src)
	}

	// Long words that do not fit are hard-broken.
	got = RenderMarkdown("abcdefghijklmnop", mdTestIdentityStyles(6))
	mdAssertLines(t, got, []string{"abcdef", "ghijkl", "mnop"})
	mdAssertMaxWidth(t, got, 6)

	// Styling is applied after wrapping, so style funcs never affect width.
	got = RenderMarkdown("**abcdefghij klmnop**", mdTestTagStyles(10))
	mdAssertLines(t, got, []string{"<b>abcdefghij</b>", "<b>klmnop</b>"})
	mdAssertMaxWidth(t, got, 10)
}

func TestRenderMarkdownWrapCJK(t *testing.T) {
	src := strings.Repeat("数", 30) // 60 display columns
	got := RenderMarkdown(src, mdTestIdentityStyles(20))
	want := []string{
		strings.Repeat("数", 10),
		strings.Repeat("数", 10),
		strings.Repeat("数", 10),
	}
	mdAssertLines(t, got, want)
	mdAssertMaxWidth(t, got, 20)
	if joined := strings.Join(got, ""); joined != src {
		t.Errorf("wrapped lines lost data: %q != %q", joined, src)
	}

	// Mixed CJK and ASCII runes are measured per rune.
	mixed := "中a中a中a中a中a中a"
	got = RenderMarkdown(mixed, mdTestIdentityStyles(10))
	mdAssertMaxWidth(t, got, 10)
	if joined := strings.Join(got, ""); joined != mixed {
		t.Errorf("wrapped lines lost data: %q != %q", joined, mixed)
	}
}

func TestRenderMarkdownWrapListIndent(t *testing.T) {
	// Bullet continuation lines get the marker's display width as indent.
	src := "- " + strings.Repeat("中", 12)
	got := RenderMarkdown(src, mdTestIdentityStyles(10))
	want := []string{
		"• " + strings.Repeat("中", 4),
		"  " + strings.Repeat("中", 4),
		"  " + strings.Repeat("中", 4),
	}
	mdAssertLines(t, got, want)
	mdAssertMaxWidth(t, got, 10)

	// Numbered markers ("12. " is four columns wide).
	src = "12. " + strings.Repeat("中", 9)
	got = RenderMarkdown(src, mdTestIdentityStyles(10))
	want = []string{
		"12. " + strings.Repeat("中", 3),
		"    " + strings.Repeat("中", 3),
		"    " + strings.Repeat("中", 3),
	}
	mdAssertLines(t, got, want)
	mdAssertMaxWidth(t, got, 10)

	// English bullet wrapping keeps the bullet only on the first line.
	got = RenderMarkdown("- one two three four", mdTestIdentityStyles(12))
	mdAssertLines(t, got, []string{"• one two", "  three four"})
	mdAssertMaxWidth(t, got, 12)
}

func TestRenderMarkdownWrapDisabled(t *testing.T) {
	src := "the quick brown fox jumps over the lazy dog **bold**"
	for _, wrap := range []int{0, -1, -100} {
		got := RenderMarkdown(src, mdTestIdentityStyles(wrap))
		mdAssertLines(t, got, []string{"the quick brown fox jumps over the lazy dog bold"})
	}
	// No wrapping also applies to lists.
	got := RenderMarkdown("- one two three four", mdTestIdentityStyles(0))
	mdAssertLines(t, got, []string{"• one two three four"})
}

// TestRenderMarkdownWrapInvariant renders a document that mixes all block
// types and checks that no rendered line exceeds the wrap width.
func TestRenderMarkdownWrapInvariant(t *testing.T) {
	src := "# 一个很长的中文标题标题标题\n\n" +
		"## Heading two with several words\n\n" +
		"- 列表项内容很长很长很长很长很长很长\n" +
		"  - 嵌套列表项内容也很长很长很长\n" +
		"1. 编号列表内容同样很长很长很长很长\n\n" +
		"> 引用内容很长很长很长很长很长很长\n\n" +
		"---\n\n" +
		"普通段落，包含 **加粗**、*斜体*、`代码` 与 [链接](https://example.com)。\n\n" +
		"```\nfunc main() { fmt.Println(\"a fairly long code line here\") }\n```\n"
	got := RenderMarkdown(src, mdTestIdentityStyles(20))
	mdAssertMaxWidth(t, got, 20)
}

// ---------- edge cases ----------

func TestRenderMarkdownEmptyInput(t *testing.T) {
	mdAssertLines(t, RenderMarkdown("", MDStyles{}), []string{""})
	mdAssertLines(t, RenderMarkdown("", mdTestTagStyles(0)), []string{""})
	mdAssertLines(t, RenderMarkdown("\n\n\n", MDStyles{}), []string{""})
	mdAssertLines(t, RenderMarkdown("\r\n\r\n", MDStyles{}), []string{""})
	mdAssertLines(t, RenderMarkdown("   \n\t\n", MDStyles{}), []string{""})
}

// TestRenderMarkdownNilStyles checks that the zero value of MDStyles renders
// plain text for every block type.
func TestRenderMarkdownNilStyles(t *testing.T) {
	got := RenderMarkdown("# h1\n## h2\n### h3\n- bullet\n1. numbered\n> quote\n---\n**b** *i* `c` [l](https://u)", MDStyles{})
	want := []string{
		"h1",
		"h2",
		"h3",
		"• bullet",
		"1. numbered",
		"│ quote",
		strings.Repeat("─", 40),
		"b i c l",
	}
	mdAssertLines(t, got, want)
}
