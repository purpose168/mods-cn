// Package proto 共享协议包。
// 该包定义了对话系统中使用的核心数据结构和协议类型。
package proto

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// 角色常量定义。
// 定义了对话中可能出现的各种角色类型。
const (
	RoleSystem    = "system"    // 系统角色：用于设置对话的上下文和行为规则
	RoleUser      = "user"      // 用户角色：表示用户的输入消息
	RoleAssistant = "assistant" // 助手角色：表示AI助手的回复消息
	RoleTool      = "tool"      // 工具角色：表示工具调用的结果
)

// Chunk 表示流式文本的数据块。
// 用于在流式传输过程中逐步传递文本内容。
type Chunk struct {
	Content string // 文本块的内容
}

// ToolCallStatus 表示工具调用的状态信息。
// 记录工具调用的名称以及可能的错误信息。
type ToolCallStatus struct {
	Name string // 工具名称
	Err  error  // 错误信息，如果调用成功则为nil
}

// String 方法将工具调用状态格式化为可读的字符串。
// 输出格式包括工具名称和错误信息（如果存在）。
func (c ToolCallStatus) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n> 运行工具: `%s`\n", c.Name))
	if c.Err != nil {
		sb.WriteString(">\n> *失败*:\n> ```\n")
		for line := range strings.SplitSeq(c.Err.Error(), "\n") {
			sb.WriteString("> " + line)
		}
		sb.WriteString("\n> ```\n")
	}
	sb.WriteByte('\n')
	return sb.String()
}

// Message 表示对话中的一条消息。
// 包含消息的角色、内容以及可能的工具调用信息。
type Message struct {
	Role      string    // 消息角色（system/user/assistant/tool）
	Content   string    // 消息内容
	ToolCalls []ToolCall // 工具调用列表（仅在角色为tool时使用）
}

// ToolCall 表示消息中的工具调用。
// 记录工具调用的ID、函数信息和执行状态。
type ToolCall struct {
	ID       string   // 工具调用的唯一标识符
	Function Function // 被调用的函数信息
	IsError  bool     // 标识工具调用是否失败
}

// Function 表示工具调用的函数签名。
// 包含函数名称和调用参数。
type Function struct {
	Name      string // 函数名称
	Arguments []byte // 函数参数（JSON格式的字节数组）
}

// Request 表示聊天请求。
// 包含完整的对话上下文和模型配置参数。
type Request struct {
	Messages       []Message                   // 对话消息列表
	API            string                      // API端点地址
	Model          string                      // 模型名称
	User           string                      // 用户标识
	Tools          map[string][]mcp.Tool       // 可用工具映射（按类别分组）
	Temperature    *float64                    // 温度参数，控制输出的随机性
	TopP           *float64                    // Top-P采样参数（核采样）
	TopK           *int64                      // Top-K采样参数
	Stop           []string                    // 停止词列表
	MaxTokens      *int64                      // 最大生成令牌数
	ResponseFormat *string                     // 响应格式（如json、text等）
	ToolCaller     func(name string, data []byte) (string, error) // 工具调用函数
}

// Conversation 表示一个完整的对话。
// 是Message切片的类型别名，提供了格式化输出的方法。
type Conversation []Message

// String 方法将对话格式化为可读的字符串。
// 按照角色类型对消息进行分类显示，支持系统、用户、助手和工具消息。
func (cc Conversation) String() string {
	var sb strings.Builder
	for _, msg := range cc {
		if msg.Content == "" {
			continue
		}
		switch msg.Role {
		case RoleSystem:
			sb.WriteString("**系统**: ")
		case RoleUser:
			sb.WriteString("**用户**: ")
		case RoleTool:
			// 处理工具调用结果
			for _, tool := range msg.ToolCalls {
				s := ToolCallStatus{
					Name: tool.Function.Name,
				}
				if tool.IsError {
					s.Err = errors.New(msg.Content)
				}
				sb.WriteString(s.String())
			}
			continue
		case RoleAssistant:
			sb.WriteString("**助手**: ")
		}
		sb.WriteString(msg.Content)
		sb.WriteString("\n\n")
	}
	return sb.String()
}
