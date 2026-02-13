// Package stream 提供流式对话的接口。
package stream

import (
	"context"
	"errors"

	"github.com/charmbracelet/mods/internal/proto"
)

// ErrNoContent 当客户端返回无内容时发生的错误。
var ErrNoContent = errors.New("no content")

// Client 是一个流式客户端接口。
type Client interface {
	Request(context.Context, proto.Request) Stream
}

// Stream 是一个进行中的流接口。
type Stream interface {
	// 当没有更多消息时返回 false，调用者应该在此时执行 [Stream.CallTools()]，
	// 然后再次检查此方法
	Next() bool

	// 返回当前的数据块
	// 实现应该将数据块累积成一条消息，并保持其内部对话状态
	Current() (proto.Chunk, error)

	// 关闭底层流
	Close() error

	// 返回流式处理过程中的错误
	Err() error

	// 返回完整的对话消息列表
	Messages() []proto.Message

	// 处理所有待执行的工具调用
	CallTools() []proto.ToolCallStatus
}

// CallTool 使用提供的数据和调用器调用工具，并返回结果 [proto.Message] 和 [proto.ToolCallStatus]。
func CallTool(
	id, name string,
	data []byte,
	caller func(name string, data []byte) (string, error),
) (proto.Message, proto.ToolCallStatus) {
	// 调用工具并获取内容和错误
	content, err := caller(name, data)
	// 如果内容为空且存在错误，则将错误信息作为内容
	if content == "" && err != nil {
		content = err.Error()
	}
	// 返回工具调用消息和工具调用状态
	return proto.Message{
			Role:    proto.RoleTool,
			Content: content,
			ToolCalls: []proto.ToolCall{
				{
					ID:      id,
					IsError: err != nil,
					Function: proto.Function{
						Name:      name,
						Arguments: data,
					},
				},
			},
		},
		proto.ToolCallStatus{
			Name: name,
			Err:  err,
		}
}
