package main

import (
	"os"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/muesli/termenv"
)

// isInputTTY 检查标准输入是否为终端
var isInputTTY = sync.OnceValue(func() bool {
	return isatty.IsTerminal(os.Stdin.Fd())
})

// isOutputTTY 检查标准输出是否为终端
var isOutputTTY = sync.OnceValue(func() bool {
	return isatty.IsTerminal(os.Stdout.Fd())
})

// stdoutRenderer 标准输出渲染器
var stdoutRenderer = sync.OnceValue(func() *lipgloss.Renderer {
	return lipgloss.DefaultRenderer()
})

// stdoutStyles 标准输出样式
var stdoutStyles = sync.OnceValue(func() styles {
	return makeStyles(stdoutRenderer())
})

// stderrRenderer 标准错误渲染器
var stderrRenderer = sync.OnceValue(func() *lipgloss.Renderer {
	return lipgloss.NewRenderer(os.Stderr, termenv.WithColorCache(true))
})

// stderrStyles 标准错误样式
var stderrStyles = sync.OnceValue(func() styles {
	return makeStyles(stderrRenderer())
})
