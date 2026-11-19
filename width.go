package main

// lipgloss 的薄封装：util.go 不想直接依赖 lipgloss 的具体 API 细节，
// 集中在这里以便测试时替换/统一行为。

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

func lipglossWidth(s string) int { return lipgloss.Width(s) }

func lipglossRuneWidth(r rune) int { return runewidth.RuneWidth(r) }
