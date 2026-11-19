package main

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// maxContentWidth 限制内容最大宽度：宽屏终端下内容居中，避免一行横跨 200 列。
const maxContentWidth = 100

// Theme 是语义化颜色令牌 + 预构建样式的集合。
// 支持深色 / 浅色 / 自适应三种模式，全部样式通过令牌派生，避免散落的硬编码色值。
type Theme struct {
	Name string // auto | dark | light
	Dark bool

	Accent  lipgloss.TerminalColor
	Success lipgloss.TerminalColor
	Warn    lipgloss.TerminalColor
	Danger  lipgloss.TerminalColor
	Info    lipgloss.TerminalColor
	Muted   lipgloss.TerminalColor
	Text    lipgloss.TerminalColor
	Border  lipgloss.TerminalColor
	Surface lipgloss.TerminalColor

	AppName    lipgloss.Style
	Title      lipgloss.Style
	Subtitle   lipgloss.Style
	Separator  lipgloss.Style
	Dim        lipgloss.Style
	DimBold    lipgloss.Style
	Key        lipgloss.Style
	Selected   lipgloss.Style
	Cursor     lipgloss.Style
	Panel      lipgloss.Style
	PanelTitle lipgloss.Style

	SuccessText lipgloss.Style
	WarnText    lipgloss.Style
	DangerText  lipgloss.Style
	InfoText    lipgloss.Style
	AccentText  lipgloss.Style
	TextStyle   lipgloss.Style

	BarFill  lipgloss.Style
	BarEmpty lipgloss.Style
	BarWarn  lipgloss.Style

	StatusBar lipgloss.Style
	StatusKey lipgloss.Style
	Toast     lipgloss.Style
	ToastErr  lipgloss.Style
	TableHead lipgloss.Style
	BadgeOn   lipgloss.Style
	BadgeOff  lipgloss.Style
}

// NewTheme 根据 auto|dark|light 构造主题。
func NewTheme(name string) *Theme {
	if name != "dark" && name != "light" {
		name = "auto"
	}
	t := &Theme{Name: name}
	t.Dark = name == "dark" || (name == "auto" && lipgloss.HasDarkBackground())

	// 深色 / 浅色两套调色板
	t.Accent = t.color("#22d3ee", "#0369a1")
	t.Success = t.color("#34d399", "#047857")
	t.Warn = t.color("#fbbf24", "#b45309")
	t.Danger = t.color("#f87171", "#b91c1c")
	t.Info = t.color("#60a5fa", "#1d4ed8")
	t.Muted = t.color("#94a3b8", "#64748b")
	t.Text = t.color("#e5e7eb", "#0f172a")
	t.Border = t.color("#475569", "#cbd5e1")
	t.Surface = t.color("#1e293b", "#e2e8f0")

	t.AppName = lipgloss.NewStyle().Bold(true).Foreground(t.Accent)
	t.Title = lipgloss.NewStyle().Bold(true).Foreground(t.Text)
	t.Subtitle = lipgloss.NewStyle().Foreground(t.Muted)
	t.Separator = lipgloss.NewStyle().Foreground(t.Border)
	t.Dim = lipgloss.NewStyle().Foreground(t.Muted)
	t.DimBold = lipgloss.NewStyle().Foreground(t.Muted).Bold(true)
	t.Key = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	t.Selected = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	t.Cursor = lipgloss.NewStyle().Foreground(t.Accent)
	t.Panel = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Border).
		Padding(0, 1)
	t.PanelTitle = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)

	t.SuccessText = lipgloss.NewStyle().Foreground(t.Success)
	t.WarnText = lipgloss.NewStyle().Foreground(t.Warn)
	t.DangerText = lipgloss.NewStyle().Foreground(t.Danger)
	t.InfoText = lipgloss.NewStyle().Foreground(t.Info)
	t.AccentText = lipgloss.NewStyle().Foreground(t.Accent)
	t.TextStyle = lipgloss.NewStyle().Foreground(t.Text)

	t.BarFill = lipgloss.NewStyle().Foreground(t.Accent)
	t.BarEmpty = lipgloss.NewStyle().Foreground(t.Border)
	t.BarWarn = lipgloss.NewStyle().Foreground(t.Warn)

	t.StatusBar = lipgloss.NewStyle().Foreground(t.Muted)
	t.StatusKey = lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	t.Toast = lipgloss.NewStyle().Foreground(t.Warn).Bold(true)
	t.ToastErr = lipgloss.NewStyle().Foreground(t.Danger).Bold(true)
	t.TableHead = lipgloss.NewStyle().Foreground(t.Muted).Bold(true)
	t.BadgeOn = lipgloss.NewStyle().Foreground(t.Success)
	t.BadgeOff = lipgloss.NewStyle().Foreground(t.Muted)

	return t
}

// color 返回当前主题模式下应使用的颜色。
func (t *Theme) color(darkHex, lightHex string) lipgloss.TerminalColor {
	switch t.Name {
	case "dark":
		return lipgloss.Color(darkHex)
	case "light":
		return lipgloss.Color(lightHex)
	default:
		return lipgloss.AdaptiveColor{Dark: darkHex, Light: lightHex}
	}
}

// nextThemeName 在 auto → dark → light 之间循环。
func nextThemeName(cur string) string {
	switch cur {
	case "dark":
		return "light"
	case "light":
		return "auto"
	default:
		return "dark"
	}
}

// noColor 表示用户显式要求无色输出。
func noColor() bool { return os.Getenv("NO_COLOR") != "" }

// mdStyles 生成更新日志 Markdown 渲染所需的样式函数。
func (t *Theme) mdStyles(wrap int) MDStyles {
	return MDStyles{
		Heading1: func(s string) string { return t.Title.Underline(true).Render(s) },
		Heading2: func(s string) string { return t.AccentText.Bold(true).Render(s) },
		Heading3: func(s string) string { return t.InfoText.Bold(true).Render(s) },
		Bold:     func(s string) string { return t.TextStyle.Bold(true).Render(s) },
		Italic:   func(s string) string { return t.TextStyle.Italic(true).Render(s) },
		Code:     func(s string) string { return t.WarnText.Render(s) },
		Link:     func(s string) string { return t.InfoText.Underline(true).Render(s) },
		Bullet:   func(s string) string { return t.AccentText.Render(s) },
		Quote:    func(s string) string { return t.Dim.Render(s) },
		Dim:      func(s string) string { return t.Dim.Render(s) },
		Wrap:     wrap,
	}
}

// rule 生成一条分隔线。
func (t *Theme) rule(width int) string {
	if width < 1 {
		width = 1
	}
	return t.Separator.Render(strings.Repeat("─", width))
}
