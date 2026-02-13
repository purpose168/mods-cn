package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLoad 测试加载功能
func TestLoad(t *testing.T) {
	const content = "just text"
	// 测试普通消息
	t.Run("普通消息", func(t *testing.T) {
		msg, err := loadMsg(content)
		require.NoError(t, err)
		require.Equal(t, content, msg)
	})

	// 测试文件
	t.Run("文件", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "foo.txt")
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

		msg, err := loadMsg("file://" + path)
		require.NoError(t, err)
		require.Equal(t, content, msg)
	})

	// 测试 HTTP URL
	t.Run("HTTP URL", func(t *testing.T) {
		msg, err := loadMsg("http://raw.githubusercontent.com/charmbracelet/mods/main/LICENSE")
		require.NoError(t, err)
		require.Contains(t, msg, "MIT License")
	})

	// 测试 HTTPS URL
	t.Run("HTTPS URL", func(t *testing.T) {
		msg, err := loadMsg("https://raw.githubusercontent.com/charmbracelet/mods/main/LICENSE")
		require.NoError(t, err)
		require.Contains(t, msg, "MIT License")
	})
}
