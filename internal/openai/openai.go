// Package openai 为 OpenAI 实现 [stream.Stream] 接口。
package openai

import (
	"context"
	"net/http"
	"strings"

	"github.com/charmbracelet/mods/internal/proto"
	"github.com/charmbracelet/mods/internal/stream"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/azure"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/ssestream"
	"github.com/openai/openai-go/shared"
)

var _ stream.Client = &Client{}

// Client 是 OpenAI 客户端。
type Client struct {
	*openai.Client
}

// Config 表示 OpenAI API 客户端的配置。
type Config struct {
	AuthToken  string // 认证令牌
	BaseURL    string // 基础 URL
	HTTPClient interface {
		Do(*http.Request) (*http.Response, error)
	} // HTTP 客户端接口
	APIType string // API 类型
}

// DefaultConfig 返回 OpenAI API 客户端的默认配置。
func DefaultConfig(authToken string) Config {
	return Config{
		AuthToken: authToken,
	}
}

// New 使用给定的 [Config] 创建新的 [Client]。
func New(config Config) *Client {
	opts := []option.RequestOption{}

	// 如果提供了自定义 HTTP 客户端，则添加到选项中
	if config.HTTPClient != nil {
		opts = append(opts, option.WithHTTPClient(config.HTTPClient))
	}

	// 根据 API 类型配置不同的认证方式
	if config.APIType == "azure-ad" {
		// Azure AD 认证配置
		opts = append(opts, azure.WithAPIKey(config.AuthToken))
		if config.BaseURL != "" {
			opts = append(opts, azure.WithEndpoint(config.BaseURL, "v1"))
		}
	} else {
		// 标准 OpenAI 认证配置
		opts = append(opts, option.WithAPIKey(config.AuthToken))
		if config.BaseURL != "" {
			opts = append(opts, option.WithBaseURL(config.BaseURL))
		}
	}
	client := openai.NewClient(opts...)
	return &Client{
		Client: &client,
	}
}

// Request 发起新请求并返回流。
func (c *Client) Request(ctx context.Context, request proto.Request) stream.Stream {
	// 构建聊天补全请求参数
	body := openai.ChatCompletionNewParams{
		Model:    request.Model,                       // 模型名称
		User:     openai.String(request.User),         // 用户标识
		Messages: fromProtoMessages(request.Messages), // 消息列表
		Tools:    fromMCPTools(request.Tools),         // 工具列表
	}

	// 对于非 Perplexity 在线模型，设置额外的参数
	if request.API != "perplexity" || !strings.Contains(request.Model, "online") {
		// 设置温度参数（控制输出随机性）
		if request.Temperature != nil {
			body.Temperature = openai.Float(*request.Temperature)
		}
		// 设置 Top-P 参数（核采样参数）
		if request.TopP != nil {
			body.TopP = openai.Float(*request.TopP)
		}
		// 设置停止序列
		body.Stop = openai.ChatCompletionNewParamsStopUnion{
			OfStringArray: request.Stop,
		}
		// 设置最大令牌数
		if request.MaxTokens != nil {
			body.MaxTokens = openai.Int(*request.MaxTokens)
		}
		// 为 OpenAI API 设置 JSON 响应格式
		if request.API == "openai" && request.ResponseFormat != nil && *request.ResponseFormat == "json" {
			body.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
				OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
			}
		}
	}

	// 创建流对象
	s := &Stream{
		stream:   c.Chat.Completions.NewStreaming(ctx, body),
		request:  body,
		toolCall: request.ToolCaller,
		messages: request.Messages,
	}
	// 设置流工厂函数，用于重新创建流
	s.factory = func() *ssestream.Stream[openai.ChatCompletionChunk] {
		return c.Chat.Completions.NewStreaming(ctx, s.request)
	}
	return s
}

// Stream OpenAI 流结构体。
type Stream struct {
	done     bool                                                 // 流是否完成的标志
	request  openai.ChatCompletionNewParams                       // 请求参数
	stream   *ssestream.Stream[openai.ChatCompletionChunk]        // 底层流
	factory  func() *ssestream.Stream[openai.ChatCompletionChunk] // 流工厂函数
	message  openai.ChatCompletionAccumulator                     // 消息累加器
	messages []proto.Message                                      // 消息列表
	toolCall func(name string, data []byte) (string, error)       // 工具调用函数
}

// CallTools 实现 stream.Stream 接口。
// 调用工具并返回工具调用状态列表。
func (s *Stream) CallTools() []proto.ToolCallStatus {
	calls := s.message.Choices[0].Message.ToolCalls
	statuses := make([]proto.ToolCallStatus, 0, len(calls))
	for _, call := range calls {
		// 执行工具调用
		msg, status := stream.CallTool(
			call.ID,
			call.Function.Name,
			[]byte(call.Function.Arguments),
			s.toolCall,
		)
		// 创建工具响应消息
		resp := openai.ToolMessage(
			msg.Content,
			call.ID,
		)
		// 将工具响应添加到请求消息列表
		s.request.Messages = append(s.request.Messages, resp)
		s.messages = append(s.messages, msg)
		statuses = append(statuses, status)
	}
	return statuses
}

// Close 实现 stream.Stream 接口。
// 关闭流并释放资源。
func (s *Stream) Close() error { return s.stream.Close() } //nolint:wrapcheck

// Current 实现 stream.Stream 接口。
// 返回当前数据块。
func (s *Stream) Current() (proto.Chunk, error) {
	event := s.stream.Current()
	s.message.AddChunk(event)
	if len(event.Choices) > 0 {
		return proto.Chunk{
			Content: event.Choices[0].Delta.Content,
		}, nil
	}
	return proto.Chunk{}, stream.ErrNoContent
}

// Err 实现 stream.Stream 接口。
// 返回流中的错误。
func (s *Stream) Err() error { return s.stream.Err() } //nolint:wrapcheck

// Messages 实现 stream.Stream 接口。
// 返回消息列表。
func (s *Stream) Messages() []proto.Message { return s.messages }

// Next 实现 stream.Stream 接口。
// 推进到下一个数据块，返回是否还有更多数据。
func (s *Stream) Next() bool {
	// 如果流已完成，重置并创建新流
	if s.done {
		s.done = false
		s.stream = s.factory()
		s.message = openai.ChatCompletionAccumulator{}
	}

	if s.stream.Next() {
		return true
	}

	// 流结束，保存最终消息
	s.done = true
	if len(s.message.Choices) > 0 {
		msg := s.message.Choices[0].Message.ToParam()
		s.request.Messages = append(s.request.Messages, msg)
		s.messages = append(s.messages, toProtoMessage(msg))
	}

	return false
}
