package main

import (
	"math/rand"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
	"github.com/muesli/termenv"
)

const (
	charCyclingFPS  = time.Second / 22 // 字符循环帧率
	colorCycleFPS   = time.Second / 5  // 颜色循环帧率
	maxCyclingChars = 120              // 最大循环字符数
)

var charRunes = []rune("0123456789abcdefABCDEF~!@#$£€%^&*()+=_")

type charState int

const (
	charInitialState   charState = iota // 字符初始状态
	charCyclingState                    // 字符循环状态
	charEndOfLifeState                  // 字符生命周期结束状态
)

// cyclingChar 表示单个动画字符。
type cyclingChar struct {
	finalValue   rune          // 最终值，如果小于 0 则永久循环
	currentValue rune          // 当前值
	initialDelay time.Duration // 初始延迟时间
	lifetime     time.Duration // 生命周期时长
}

// randomRune 返回一个随机字符
func (c cyclingChar) randomRune() rune {
	return (charRunes)[rand.Intn(len(charRunes))] //nolint:gosec
}

// state 返回字符的当前状态
func (c cyclingChar) state(start time.Time) charState {
	now := time.Now()
	if now.Before(start.Add(c.initialDelay)) {
		return charInitialState
	}
	if c.finalValue > 0 && now.After(start.Add(c.initialDelay)) {
		return charEndOfLifeState
	}
	return charCyclingState
}

type stepCharsMsg struct{}

// stepChars 返回字符步进命令
func stepChars() tea.Cmd {
	return tea.Tick(charCyclingFPS, func(time.Time) tea.Msg {
		return stepCharsMsg{}
	})
}

type colorCycleMsg struct{}

// cycleColors 返回颜色循环命令
func cycleColors() tea.Cmd {
	return tea.Tick(colorCycleFPS, func(time.Time) tea.Msg {
		return colorCycleMsg{}
	})
}

// anim 是管理动画的模型，在生成输出时显示动画效果。
type anim struct {
	start           time.Time        // 动画开始时间
	cyclingChars    []cyclingChar    // 循环字符列表
	labelChars      []cyclingChar    // 标签字符列表
	ramp            []lipgloss.Style // 颜色渐变样式
	label           []rune           // 标签文本
	ellipsis        spinner.Model    // 省略号旋转器模型
	ellipsisStarted bool             // 省略号是否已启动
	styles          styles           // 样式配置
}

// newAnim 创建一个新的动画实例
// cyclingCharsSize: 循环字符数量
// label: 标签文本
// r: lipgloss 渲染器
// s: 样式配置
func newAnim(cyclingCharsSize uint, label string, r *lipgloss.Renderer, s styles) anim {
	// #nosec G115
	n := int(cyclingCharsSize)
	if n > maxCyclingChars {
		n = maxCyclingChars
	}

	gap := " "
	if n == 0 {
		gap = ""
	}

	c := anim{
		start:    time.Now(),
		label:    []rune(gap + label),
		ellipsis: spinner.New(spinner.WithSpinner(spinner.Ellipsis)),
		styles:   s,
	}

	// 如果处于真彩色模式（并且有足够的循环字符）
	// 使用渐变色彩条为循环字符着色
	const minRampSize = 3
	if n >= minRampSize && r.ColorProfile() == termenv.TrueColor {
		// 注意：为颜色循环预留双倍容量，因为我们需要反转并
		// 追加色彩条以实现无缝过渡
		c.ramp = make([]lipgloss.Style, n, n*2) //nolint:mnd
		ramp := makeGradientRamp(n)
		for i, color := range ramp {
			c.ramp[i] = r.NewStyle().Foreground(color)
		}
		c.ramp = append(c.ramp, reverse(c.ramp)...) // 反转并追加以实现颜色循环
	}

	makeDelay := func(a int32, b time.Duration) time.Duration {
		return time.Duration(rand.Int31n(a)) * (time.Millisecond * b) //nolint:gosec
	}

	makeInitialDelay := func() time.Duration {
		return makeDelay(8, 60) //nolint:mnd
	}

	// 永久循环的初始字符
	c.cyclingChars = make([]cyclingChar, n)

	for i := 0; i < n; i++ {
		c.cyclingChars[i] = cyclingChar{
			finalValue:   -1, // 永久循环
			initialDelay: makeInitialDelay(),
		}
	}

	// 仅循环一小段时间的标签文本
	c.labelChars = make([]cyclingChar, len(c.label))

	for i, r := range c.label {
		c.labelChars[i] = cyclingChar{
			finalValue:   r,
			initialDelay: makeInitialDelay(),
			lifetime:     makeDelay(5, 180), //nolint:mnd
		}
	}

	return c
}

// Init 初始化动画
func (anim) Init() tea.Cmd {
	return tea.Batch(stepChars(), cycleColors())
}

// Update 处理消息
func (a anim) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg.(type) {
	case stepCharsMsg:
		a.updateChars(&a.cyclingChars)
		a.updateChars(&a.labelChars)

		if !a.ellipsisStarted {
			var eol int
			for _, c := range a.labelChars {
				if c.state(a.start) == charEndOfLifeState {
					eol++
				}
			}
			if eol == len(a.label) {
				// 如果整个标签都已到达生命周期终点，在短暂暂停后
				// 启动省略号"旋转器"
				a.ellipsisStarted = true
				cmd = tea.Tick(time.Millisecond*220, func(time.Time) tea.Msg { //nolint:mnd
					return a.ellipsis.Tick()
				})
			}
		}

		return a, tea.Batch(stepChars(), cmd)
	case colorCycleMsg:
		const minColorCycleSize = 2
		if len(a.ramp) < minColorCycleSize {
			return a, nil
		}
		a.ramp = append(a.ramp[1:], a.ramp[0])
		return a, cycleColors()
	case spinner.TickMsg:
		var cmd tea.Cmd
		a.ellipsis, cmd = a.ellipsis.Update(msg)
		return a, cmd
	default:
		return a, nil
	}
}

// updateChars 更新字符状态
func (a *anim) updateChars(chars *[]cyclingChar) {
	for i, c := range *chars {
		switch c.state(a.start) {
		case charInitialState:
			(*chars)[i].currentValue = '.'
		case charCyclingState:
			(*chars)[i].currentValue = c.randomRune()
		case charEndOfLifeState:
			(*chars)[i].currentValue = c.finalValue
		}
	}
}

// View 渲染动画视图
func (a anim) View() string {
	var b strings.Builder

	for i, c := range a.cyclingChars {
		if len(a.ramp) > i {
			b.WriteString(a.ramp[i].Render(string(c.currentValue)))
			continue
		}
		b.WriteRune(c.currentValue)
	}

	for _, c := range a.labelChars {
		b.WriteRune(c.currentValue)
	}

	return b.String() + a.ellipsis.View()
}

// makeGradientRamp 创建渐变色彩条
// length: 色彩条长度
// 返回：lipgloss 颜色数组
func makeGradientRamp(length int) []lipgloss.Color {
	const startColor = "#F967DC" // 起始颜色（粉红色）
	const endColor = "#6B50FF"   // 结束颜色（紫色）
	var (
		c        = make([]lipgloss.Color, length)
		start, _ = colorful.Hex(startColor)
		end, _   = colorful.Hex(endColor)
	)
	for i := 0; i < length; i++ {
		step := start.BlendLuv(end, float64(i)/float64(length))
		c[i] = lipgloss.Color(step.Hex())
	}
	return c
}

// makeGradientText 创建渐变文本
// baseStyle: 基础样式
// str: 要渲染的字符串
// 返回：带渐变效果的字符串
func makeGradientText(baseStyle lipgloss.Style, str string) string {
	const minSize = 3
	if len(str) < minSize {
		return str
	}
	b := strings.Builder{}
	runes := []rune(str)
	for i, c := range makeGradientRamp(len(str)) {
		b.WriteString(baseStyle.Foreground(c).Render(string(runes[i])))
	}
	return b.String()
}

// reverse 反转切片
// in: 输入切片
// 返回：反转后的切片
func reverse[T any](in []T) []T {
	out := make([]T, len(in))
	copy(out, in[:])
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}
