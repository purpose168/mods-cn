package main

import (
	"strings"

	"github.com/charmbracelet/mods/internal/proto"
)

// lastPrompt 获取最后的用户提示
// messages: 消息列表
// 返回：最后的用户提示内容
func lastPrompt(messages []proto.Message) string {
	var result string
	for _, msg := range messages {
		if msg.Role != proto.RoleUser {
			continue
		}
		if msg.Content == "" {
			continue
		}
		result = msg.Content
	}
	return result
}

// firstLine 获取字符串的第一行
// s: 输入字符串
// 返回：第一行内容
func firstLine(s string) string {
	first, _, _ := strings.Cut(s, "\n")
	return first
}
