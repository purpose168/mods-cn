package main

import (
	"math/rand"
	"regexp"
)

// examples 示例命令映射
var examples = map[string]string{
	"为 README 编写新章节": `cat README.md | mods "为此 README 编写一个新章节，记录 PDF 分享功能"`,
	"编辑视频文件":         `ls ~/vids | mods -f "总结这些标题，按年代分组" | glow`,
	"让 GPT 选择观看内容":   `ls ~/vids | mods "从此列表中挑选 5 部 80 年代的动作片" | gum choose | xargs vlc`,
}

// randomExample 返回随机示例
func randomExample() string {
	keys := make([]string, 0, len(examples))
	for k := range examples {
		keys = append(keys, k)
	}
	desc := keys[rand.Intn(len(keys))] //nolint:gosec
	return desc
}

// cheapHighlighting 简单的语法高亮
// s: 样式配置
// code: 代码字符串
// 返回：高亮后的代码字符串
func cheapHighlighting(s styles, code string) string {
	// 高亮引号字符串
	code = regexp.
		MustCompile(`"([^"\\]|\\.)*"`).
		ReplaceAllStringFunc(code, func(x string) string {
			return s.Quote.Render(x)
		})
	// 高亮管道符
	code = regexp.
		MustCompile(`\|`).
		ReplaceAllStringFunc(code, func(x string) string {
			return s.Pipe.Render(x)
		})
	return code
}
