// Package cohere 为 Cohere 实现 [stream.Stream] 接口。
package cohere

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/charmbracelet/mods/internal/proto"
	"github.com/charmbracelet/mods/internal/stream"
	cohere "github.com/cohere-ai/cohere-go/v2"
	"github.com/cohere-ai/cohere-go/v2/client"
	"github.com/cohere-ai/cohere-go/v2/core"
	"github.com/cohere-ai/cohere-go/v2/option"
)

var _ stream.Client = &Client{}

// Config 表示 Cohere API 客户端的配置。
type Config struct {
	AuthToken  string       // 认证令牌
	BaseURL    string       // 基础 URL
	HTTPClient *http.Client // HTTP 客户端
}

// DefaultConfig 返回 Cohere API 客户端的默认配置。
func DefaultConfig(authToken string) Config {
	return Config{
		AuthToken:  authToken,
		BaseURL:    "",
		HTTPClient: &http.Client{},
	}
}

// Client Cohere 客户端。
type Client struct {
	*client.Client // 嵌入的 Cohere SDK 客户端
}

// New 使用给定的 [Config] 创建一个新的 [Client]。
func New(config Config) *Client {
	// 初始化请求选项，包括认证令牌和 HTTP 客户端
	opts := []option.RequestOption{
		client.WithToken(config.AuthToken),
		client.WithHTTPClient(config.HTTPClient),
	}

	// 如果配置了自定义基础 URL，则添加到选项中
	if config.BaseURL != "" {
		opts = append(opts, client.WithBaseURL(config.BaseURL))
	}

	// 返回新创建的客户端实例
	return &Client{
		Client: client.NewClient(opts...),
	}
}

// Request 实现 stream.Client 接口。
// 发送聊天请求并返回流式响应。
func (c *Client) Request(ctx context.Context, request proto.Request) stream.Stream {
	s := &Stream{}
	// 将协议消息转换为 Cohere 格式的历史记录和当前消息
	history, message := fromProtoMessages(request.Messages)

	// 构建聊天流请求
	body := &cohere.ChatStreamRequest{
		Model:         cohere.String(request.Model), // 模型名称
		Message:       message,                      // 当前用户消息
		ChatHistory:   history,                      // 聊天历史记录
		Temperature:   request.Temperature,          // 温度参数，控制响应的随机性
		P:             request.TopP,                 // Top-P 采样参数
		StopSequences: request.Stop,                 // 停止序列
	}

	// 如果设置了最大令牌数，则添加到请求中
	if request.MaxTokens != nil {
		body.MaxTokens = cohere.Int(int(*request.MaxTokens))
	}

	// 初始化流对象
	s.request = body
	s.done = false
	s.message = &cohere.Message{
		Role:    "CHATBOT",
		Chatbot: &cohere.ChatMessage{},
	}
	// 发起流式聊天请求
	s.stream, s.err = c.ChatStream(ctx, s.request)
	return s
}

// Stream 是一个 Cohere 流，用于处理流式聊天响应。
type Stream struct {
	stream  *core.Stream[cohere.StreamedChatResponse] // 底层流对象
	request *cohere.ChatStreamRequest                 // 原始请求
	err     error                                     // 错误信息
	done    bool                                      // 流是否完成
	message *cohere.Message                           // 累积的消息内容
}

// CallTools 实现 stream.Stream 接口。
// 当前不支持工具调用功能。
func (s *Stream) CallTools() []proto.ToolCallStatus { return nil }

// Close 实现 stream.Stream 接口。
// 关闭流并标记为已完成。
func (s *Stream) Close() error {
	s.done = true
	return s.stream.Close() //nolint:wrapcheck
}

// Current 实现 stream.Stream 接口。
// 获取当前流中的下一个内容块。
func (s *Stream) Current() (proto.Chunk, error) {
	// 接收流中的下一个响应
	resp, err := s.stream.Recv()
	if errors.Is(err, io.EOF) {
		// 流已结束，返回无内容错误
		return proto.Chunk{}, stream.ErrNoContent
	}
	if err != nil {
		return proto.Chunk{}, fmt.Errorf("cohere: %w", err)
	}

	// 根据事件类型处理响应
	switch resp.EventType {
	case "text-generation":
		// 文本生成事件，累积消息内容并返回文本块
		s.message.Chatbot.Message += resp.TextGeneration.Text
		return proto.Chunk{
			Content: resp.TextGeneration.Text,
		}, nil
	}
	// 其他事件类型返回无内容错误
	return proto.Chunk{}, stream.ErrNoContent
}

// Err 实现 stream.Stream 接口。
// 返回流处理过程中发生的错误。
func (s *Stream) Err() error { return s.err }

// Messages 实现 stream.Stream 接口。
// 返回完整的消息列表，包括历史记录、用户消息和助手响应。
func (s *Stream) Messages() []proto.Message {
	return toProtoMessages(append(s.request.ChatHistory, &cohere.Message{
		Role: "USER",
		User: &cohere.ChatMessage{
			Message: s.request.Message,
		},
	}, s.message))
}

// Next 实现 stream.Stream 接口。
// 检查流是否还有更多内容可读取。
func (s *Stream) Next() bool {
	// 如果有错误，则停止迭代
	if s.err != nil {
		return false
	}
	// 返回流是否未完成
	return !s.done
}
