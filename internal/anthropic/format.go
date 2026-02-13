package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/charmbracelet/mods/internal/proto"
	"github.com/mark3labs/mcp-go/mcp"
)

// fromMCPTools 将 MCP（Model Context Protocol）工具映射转换为 Anthropic 工具参数格式。
// 参数：
//   - mcps: MCP 工具映射，键为服务器名称，值为该服务器提供的工具列表
// 返回：
//   - []anthropic.ToolUnionParam: Anthropic 格式的工具参数列表
func fromMCPTools(mcps map[string][]mcp.Tool) []anthropic.ToolUnionParam {
	var tools []anthropic.ToolUnionParam
	
	// 遍历所有服务器的工具
	for name, serverTools := range mcps {
		for _, tool := range serverTools {
			// 将每个工具转换为 Anthropic 格式，工具名称格式为 "服务器名_工具名"
			tools = append(tools, anthropic.ToolUnionParam{
				OfTool: &anthropic.ToolParam{
					InputSchema: anthropic.ToolInputSchemaParam{
						Properties: tool.InputSchema.Properties,
					},
					Name:        fmt.Sprintf("%s_%s", name, tool.Name),
					Description: anthropic.String(tool.Description),
				},
			})
		}
	}
	return tools
}

// fromProtoMessages 将协议消息列表转换为 Anthropic 格式的系统消息和用户消息。
// 参数：
//   - input: 协议格式的消息列表
// 返回：
//   - system: 系统消息块列表（Anthropic 中系统消息不作为角色存在，需单独设置）
//   - messages: Anthropic 格式的消息参数列表
func fromProtoMessages(input []proto.Message) (system []anthropic.TextBlockParam, messages []anthropic.MessageParam) {
	for _, msg := range input {
		switch msg.Role {
		case proto.RoleSystem:
			// 在 Anthropic API 中，系统消息不作为角色存在，必须设置为请求的系统部分
			system = append(system, *anthropic.NewTextBlock(msg.Content).OfText)
		case proto.RoleTool:
			// 处理工具响应消息
			for _, call := range msg.ToolCalls {
				block := newToolResultBlock(call.ID, msg.Content, call.IsError)
				// 在 Anthropic API 中，工具消息不作为角色存在，必须作为用户消息
				messages = append(messages, anthropic.NewUserMessage(block))
				break
			}
		case proto.RoleUser:
			// 用户消息：创建文本块并添加到消息列表
			block := anthropic.NewTextBlock(msg.Content)
			messages = append(messages, anthropic.NewUserMessage(block))
		case proto.RoleAssistant:
			// 助手消息：创建文本块和工具使用块
			blocks := []anthropic.ContentBlockParamUnion{
				anthropic.NewTextBlock(msg.Content),
			}
			
			// 添加工具调用块
			for _, tool := range msg.ToolCalls {
				block := anthropic.ContentBlockParamUnion{
					OfToolUse: &anthropic.ToolUseBlockParam{
						ID:    tool.ID,
						Name:  tool.Function.Name,
						Input: json.RawMessage(tool.Function.Arguments),
					},
				}
				blocks = append(blocks, block)
			}
			messages = append(messages, anthropic.NewAssistantMessage(blocks...))
		}
	}
	return system, messages
}

// toProtoMessage 将 Anthropic 消息参数转换为协议消息格式。
// 参数：
//   - in: Anthropic 格式的消息参数
// 返回：
//   - proto.Message: 协议格式的消息对象
func toProtoMessage(in anthropic.MessageParam) proto.Message {
	msg := proto.Message{
		Role: string(in.Role),
	}

	// 遍历消息内容块，提取文本和工具调用信息
	for _, block := range in.Content {
		// 提取文本内容
		if txt := block.OfText; txt != nil {
			msg.Content += txt.Text
		}

		// 提取工具结果块
		if call := block.OfToolResult; call != nil {
			msg.ToolCalls = append(msg.ToolCalls, proto.ToolCall{
				ID:      call.ToolUseID,
				IsError: call.IsError.Value,
			})
		}

		// 提取工具使用块
		if call := block.OfToolUse; call != nil {
			msg.ToolCalls = append(msg.ToolCalls, proto.ToolCall{
				ID: call.ID,
				Function: proto.Function{
					Name:      call.Name,
					Arguments: call.Input.(json.RawMessage),
				},
			})
		}
	}

	return msg
}

// newToolResultBlock 创建工具结果块。
// Anthropic v1.5 版本移除了此方法，此处将其复制回来以避免大量重构。
// 参数：
//   - toolUseID: 工具使用 ID，用于关联工具调用和结果
//   - content: 工具执行结果内容
//   - isError: 是否为错误结果
// 返回：
//   - anthropic.ContentBlockParamUnion: 内容块参数联合类型
func newToolResultBlock(toolUseID string, content string, isError bool) anthropic.ContentBlockParamUnion {
	toolBlock := anthropic.ToolResultBlockParam{
		ToolUseID: toolUseID,
		Content: []anthropic.ToolResultBlockParamContentUnion{
			{OfText: &anthropic.TextBlockParam{Text: content}},
		},
		IsError: anthropic.Bool(isError),
	}
	return anthropic.ContentBlockParamUnion{OfToolResult: &toolBlock}
}
