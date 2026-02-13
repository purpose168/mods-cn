package proto

import (
	"testing"

	"github.com/charmbracelet/x/exp/golden"
)

// TestStringer 测试对话消息的字符串格式化功能。
// 该测试创建了一个包含多种角色消息的对话序列，并验证其字符串输出格式。
func TestStringer(t *testing.T) {
	// 构建测试消息序列
	messages := []Message{
		{
			Role:    RoleSystem,    // 系统消息：设置AI的行为角色
			Content: "you are a medieval king", // 内容：你是一个中世纪国王
		},
		{
			Role:    RoleUser,      // 用户消息：用户提问
			Content: "first 4 natural numbers", // 内容：前4个自然数
		},
		{
			Role:    RoleAssistant, // 助手消息：AI回复
			Content: "1, 2, 3, 4",  // 内容：1, 2, 3, 4
		},
		{
			Role:    RoleTool,      // 工具消息：工具调用结果
			Content: `{"the":"result"}`, // 内容：JSON格式的结果
			ToolCalls: []ToolCall{
				{
					ID: "aaa", // 工具调用ID
					Function: Function{
						Name:      "myfunc",           // 函数名称
						Arguments: []byte(`{"a":"b"}`), // 函数参数
					},
				},
			},
		},
		{
			Role:    RoleUser,          // 用户消息：后续请求
			Content: "as a json array", // 内容：以JSON数组格式
		},
		{
			Role:    RoleAssistant,     // 助手消息：AI回复
			Content: "[ 1, 2, 3, 4 ]",  // 内容：JSON数组格式
		},
		{
			Role:    RoleAssistant,            // 助手消息：额外的AI回复
			Content: "something from an assistant", // 内容：来自助手的一些内容
		},
	}

	// 使用golden测试验证输出格式
	golden.RequireEqual(t, []byte(Conversation(messages).String()))
}
