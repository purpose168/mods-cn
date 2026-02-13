package ollama

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/charmbracelet/mods/internal/proto"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/ollama/ollama/api"
)

// fromMCPTools 将 MCP 工具映射转换为 Ollama API 工具格式。
// 该函数遍历所有 MCP 服务器提供的工具，并将其转换为 Ollama 可以理解的工具定义格式。
// 参数:
//   - mcps: MCP 工具映射，键为服务器名称，值为该服务器提供的工具列表
// 返回:
//   - []api.Tool: 转换后的 Ollama 工具列表
func fromMCPTools(mcps map[string][]mcp.Tool) []api.Tool {
	var tools []api.Tool

	// 遍历所有 MCP 服务器的工具
	for name, serverTools := range mcps {
		for _, tool := range serverTools {
			// 构建工具定义，使用 "服务器名_工具名" 的格式命名
			t := api.Tool{
				Type:  "function", // 工具类型为函数
				Items: nil,
				Function: api.ToolFunction{
					Name:        fmt.Sprintf("%s_%s", name, tool.Name), // 组合名称确保唯一性
					Description: tool.Description,                       // 工具描述
				},
			}
			// 解析工具的输入参数模式（Input Schema）
			_ = json.Unmarshal(tool.RawInputSchema, &t.Function.Parameters)
			tools = append(tools, t)
		}
	}
	return tools
}

// fromProtoMessages 将 proto.Message 列表转换为 Ollama API 消息格式。
// 该函数批量转换消息列表，保持消息顺序不变。
// 参数:
//   - input: proto 格式的消息列表
// 返回:
//   - []api.Message: Ollama API 格式的消息列表
func fromProtoMessages(input []proto.Message) []api.Message {
	messages := make([]api.Message, 0, len(input))
	for _, msg := range input {
		messages = append(messages, fromProtoMessage(msg))
	}
	return messages
}

// fromProtoMessage 将单个 proto.Message 转换为 Ollama API 消息格式。
// 该函数转换消息的基本属性（角色、内容）以及工具调用信息。
// 参数:
//   - input: proto 格式的消息
// 返回:
//   - api.Message: Ollama API 格式的消息
func fromProtoMessage(input proto.Message) api.Message {
	m := api.Message{
		Content: input.Content, // 消息内容
		Role:    input.Role,    // 消息角色（user/assistant/system）
	}

	// 转换工具调用信息
	for _, call := range input.ToolCalls {
		var args api.ToolCallFunctionArguments
		// 解析工具调用的参数
		_ = json.Unmarshal(call.Function.Arguments, &args)

		// 将工具调用 ID（字符串）转换为索引（整数）
		idx, _ := strconv.Atoi(call.ID)

		m.ToolCalls = append(m.ToolCalls, api.ToolCall{
			Function: api.ToolCallFunction{
				Index:     idx,       // 工具调用索引
				Name:      call.Function.Name,      // 工具名称
				Arguments: args,                    // 工具参数
			},
		})
	}
	return m
}

// toProtoMessage 将 Ollama API 消息转换为 proto.Message 格式。
// 该函数执行与 fromProtoMessage 相反的转换，用于将 Ollama 的响应转换回内部格式。
// 参数:
//   - in: Ollama API 格式的消息
// 返回:
//   - proto.Message: proto 格式的消息
func toProtoMessage(in api.Message) proto.Message {
	msg := proto.Message{
		Role:    in.Role,    // 消息角色
		Content: in.Content, // 消息内容
	}

	// 转换工具调用信息
	for _, call := range in.ToolCalls {
		msg.ToolCalls = append(msg.ToolCalls, proto.ToolCall{
			ID: strconv.Itoa(call.Function.Index), // 将索引转换为字符串 ID
			Function: proto.Function{
				Arguments: []byte(call.Function.Arguments.String()), // 序列化参数
				Name:      call.Function.Name,                       // 工具名称
			},
		})
	}
	return msg
}
