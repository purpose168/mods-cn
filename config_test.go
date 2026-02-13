package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestConfig 测试配置解析
func TestConfig(t *testing.T) {
	// 测试旧格式文本
	t.Run("旧格式文本", func(t *testing.T) {
		var cfg Config
		require.NoError(t, yaml.Unmarshal([]byte("format-text: as markdown"), &cfg))
		require.Equal(t, FormatText(map[string]string{
			"markdown": "as markdown",
		}), cfg.FormatText)
	})
	// 测试新格式文本
	t.Run("新格式文本", func(t *testing.T) {
		var cfg Config
		require.NoError(t, yaml.Unmarshal([]byte("format-text:\n  markdown: as markdown\n  json: as json"), &cfg))
		require.Equal(t, FormatText(map[string]string{
			"markdown": "as markdown",
			"json":     "as json",
		}), cfg.FormatText)
	})
}
