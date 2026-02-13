package cohere

import (
	"github.com/charmbracelet/mods/internal/proto"
	cohere "github.com/cohere-ai/cohere-go/v2"
)

// fromProtoMessages 将协议消息转换为 Cohere 格式的消息历史和当前消息。
// 返回历史记录和当前用户消息。
func fromProtoMessages(input []proto.Message) (history []*cohere.Message, message string) {
	var messages []*cohere.Message //nolint:prealloc
	// 遍历所有输入消息并转换为 Cohere 格式
	for _, msg := range input {
		messages = append(messages, &cohere.Message{
			Role: fromProtoRole(msg.Role),
			Chatbot: &cohere.ChatMessage{
				Message: msg.Content,
			},
		})
	}
	// 如果有多条消息，则除最后一条外的所有消息作为历史记录
	if len(messages) > 1 {
		history = messages[:len(messages)-1]
	}
	// 最后一条消息作为当前用户消息
	message = messages[len(messages)-1].User.Message
	return history, message
}

// toProtoMessages 将 Cohere 格式的消息转换为协议消息格式。
func toProtoMessages(input []*cohere.Message) []proto.Message {
	var messages []proto.Message
	// 遍历所有 Cohere 消息并根据角色类型转换
	for _, in := range input {
		switch in.Role {
		case "USER":
			// 用户角色消息
			messages = append(messages, proto.Message{
				Role:    proto.RoleUser,
				Content: in.User.Message,
			})
		case "SYSTEM":
			// 系统角色消息
			messages = append(messages, proto.Message{
				Role:    proto.RoleSystem,
				Content: in.System.Message,
			})
		case "CHATBOT":
			// 助手（聊天机器人）角色消息
			messages = append(messages, proto.Message{
				Role:    proto.RoleAssistant,
				Content: in.Chatbot.Message,
			})
		case "TOOL":
			// 工具角色消息 - 当前尚未支持
		}
	}
	return messages
}

// fromProtoRole 将协议角色转换为 Cohere 角色格式。
func fromProtoRole(role string) string {
	switch role {
	case proto.RoleSystem:
		return "SYSTEM"      // 系统角色
	case proto.RoleAssistant:
		return "CHATBOT"     // 助手角色
	default:
		return "USER"        // 默认为用户角色
	}
}
