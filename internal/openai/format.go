package openai

import (
	"fmt"

	"github.com/charmbracelet/mods/internal/proto"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/shared/constant"
)

// fromMCPTools 将 MCP 工具映射转换为 OpenAI 聊天补全工具参数列表。
// 参数 mcps: MCP 工具映射，键为服务器名称，值为该服务器的工具列表
// 返回值: OpenAI 聊天补全工具参数切片
func fromMCPTools(mcps map[string][]mcp.Tool) []openai.ChatCompletionToolParam {
	var tools []openai.ChatCompletionToolParam
	for name, serverTools := range mcps {
		for _, tool := range serverTools {
			// 构建工具参数结构
			params := map[string]any{
				"type":       "object",                        // 参数类型为对象
				"properties": tool.InputSchema.Properties,     // 参数属性定义
			}
			// 添加必需参数列表
			if len(tool.InputSchema.Required) > 0 {
				params["required"] = tool.InputSchema.Required
			}

			// 创建 OpenAI 工具参数，工具名称格式为 "服务器名_工具名"
			tools = append(tools, openai.ChatCompletionToolParam{
				Type: constant.Function("function"),
				Function: openai.FunctionDefinitionParam{
					Name:        fmt.Sprintf("%s_%s", name, tool.Name), // 组合工具名称
					Description: openai.String(tool.Description),        // 工具描述
					Parameters:  params,                                 // 工具参数定义
				},
			})
		}
	}
	return tools
}

// fromProtoMessages 将协议消息列表转换为 OpenAI 聊天补全消息参数列表。
// 参数 input: 协议消息切片
// 返回值: OpenAI 聊天补全消息参数联合类型切片
func fromProtoMessages(input []proto.Message) []openai.ChatCompletionMessageParamUnion {
	var messages []openai.ChatCompletionMessageParamUnion
	for _, msg := range input {
		switch msg.Role {
		case proto.RoleSystem:
			// 系统消息
			messages = append(messages, openai.SystemMessage(msg.Content))
		case proto.RoleTool:
			// 工具消息，需要关联工具调用 ID
			for _, call := range msg.ToolCalls {
				messages = append(messages, openai.ToolMessage(msg.Content, call.ID))
				break
			}
		case proto.RoleUser:
			// 用户消息
			messages = append(messages, openai.UserMessage(msg.Content))
		case proto.RoleAssistant:
			// 助手消息，可能包含工具调用
			m := openai.AssistantMessage(msg.Content)
			for _, tool := range msg.ToolCalls {
				m.OfAssistant.ToolCalls = append(m.OfAssistant.ToolCalls, openai.ChatCompletionMessageToolCallParam{
					ID: tool.ID,
					Function: openai.ChatCompletionMessageToolCallFunctionParam{
						Arguments: string(tool.Function.Arguments), // 函数参数 JSON 字符串
						Name:      tool.Function.Name,              // 函数名称
					},
				})
			}
			messages = append(messages, m)
		}
	}
	return messages
}

// toProtoMessage 将 OpenAI 聊天补全消息参数转换为协议消息。
// 参数 in: OpenAI 聊天补全消息参数联合类型
// 返回值: 协议消息结构体
func toProtoMessage(in openai.ChatCompletionMessageParamUnion) proto.Message {
	msg := proto.Message{
		Role: msgRole(in), // 获取消息角色
	}
	// 处理消息内容
	switch content := in.GetContent().AsAny().(type) {
	case *string:
		// 字符串类型内容
		if content == nil || *content == "" {
			break
		}
		msg.Content = *content
	case *[]openai.ChatCompletionContentPartTextParam:
		// 文本部分数组类型内容
		if content == nil || len(*content) == 0 {
			break
		}
		for _, c := range *content {
			msg.Content += c.Text
		}
	}
	// 如果是助手消息，提取工具调用信息
	if msg.Role == proto.RoleAssistant {
		for _, call := range in.OfAssistant.ToolCalls {
			msg.ToolCalls = append(msg.ToolCalls, proto.ToolCall{
				ID: call.ID,
				Function: proto.Function{
					Name:      call.Function.Name,              // 函数名称
					Arguments: []byte(call.Function.Arguments), // 函数参数字节数组
				},
			})
		}
	}
	return msg
}

// msgRole 从 OpenAI 消息参数中提取消息角色。
// 参数 in: OpenAI 聊天补全消息参数联合类型
// 返回值: 消息角色字符串
func msgRole(in openai.ChatCompletionMessageParamUnion) string {
	if in.OfSystem != nil {
		return proto.RoleSystem // 系统角色
	}
	if in.OfAssistant != nil {
		return proto.RoleAssistant // 助手角色
	}
	if in.OfUser != nil {
		return proto.RoleUser // 用户角色
	}
	if in.OfTool != nil {
		return proto.RoleTool // 工具角色
	}
	return "" // 未知角色
}
