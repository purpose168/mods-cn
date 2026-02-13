package main

import (
	"testing"

	"github.com/charmbracelet/mods/internal/proto"
	"github.com/stretchr/testify/require"
)

// TestLastPrompt 测试 lastPrompt 函数
func TestLastPrompt(t *testing.T) {
	// 测试用例：无提示词
	t.Run("no prompt", func(t *testing.T) {
		require.Equal(t, "", lastPrompt(nil))
	})

	// 测试用例：单个提示词
	t.Run("single prompt", func(t *testing.T) {
		require.Equal(t, "single", lastPrompt([]proto.Message{
			{
				Role:    proto.RoleUser,
				Content: "single",
			},
		}))
	})

	// 测试用例：多个提示词
	t.Run("multiple prompts", func(t *testing.T) {
		require.Equal(t, "last", lastPrompt([]proto.Message{
			{
				Role:    proto.RoleUser,
				Content: "first",
			},
			{
				Role:    proto.RoleAssistant,
				Content: "hallo",
			},
			{
				Role:    proto.RoleUser,
				Content: "middle 1",
			},
			{
				Role:    proto.RoleUser,
				Content: "middle 2",
			},
			{
				Role:    proto.RoleUser,
				Content: "last",
			},
		}))
	})
}

// TestFirstLine 测试 firstLine 函数
func TestFirstLine(t *testing.T) {
	// 测试用例：单行文本
	t.Run("single line", func(t *testing.T) {
		require.Equal(t, "line", firstLine("line"))
	})
	// 测试用例：以换行符结尾的单行文本
	t.Run("single line ending with \n", func(t *testing.T) {
		require.Equal(t, "line", firstLine("line\n"))
	})
	// 测试用例：多行文本
	t.Run("multiple lines", func(t *testing.T) {
		require.Equal(t, "line", firstLine("line\nsomething else\nline3\nfoo\nends with a double \n\n"))
	})
}
