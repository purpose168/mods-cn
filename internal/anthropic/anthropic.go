// Package anthropic 为 Anthropic API 实现 [stream.Stream] 接口。
package anthropic

import (
	"context"
	"net/http"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/charmbracelet/mods/internal/proto"
	"github.com/charmbracelet/mods/internal/stream"
)

var _ stream.Client = &Client{}

// Client 是 Anthropic API 的客户端结构体。
type Client struct {
	*anthropic.Client
}

// Request 实现 stream.Client 接口，创建并返回一个流式请求。
// 参数：
//   - ctx: 上下文，用于控制请求的生命周期
//   - request: 协议请求对象，包含消息、模型配置等信息
// 返回：
//   - stream.Stream: 流式响应对象
func (c *Client) Request(ctx context.Context, request proto.Request) stream.Stream {
	// 将协议消息转换为 Anthropic 格式的系统消息和用户消息
	system, messages := fromProtoMessages(request.Messages)
	
	// 构建消息请求参数
	body := anthropic.MessageNewParams{
		Model:         anthropic.Model(request.Model),
		Messages:      messages,
		System:        system,
		Tools:         fromMCPTools(request.Tools),
		StopSequences: request.Stop,
	}

	// 设置最大令牌数，如果未指定则使用默认值 4096
	if request.MaxTokens != nil {
		body.MaxTokens = *request.MaxTokens
	} else {
		body.MaxTokens = 4096
	}

	// 设置温度参数（Temperature），控制输出的随机性
	if request.Temperature != nil {
		body.Temperature = anthropic.Float(*request.Temperature)
	}

	// 设置 Top-P 参数，控制核采样概率
	if request.TopP != nil {
		body.TopP = anthropic.Float(*request.TopP)
	}

	// 创建流式响应对象
	s := &Stream{
		stream:   c.Messages.NewStreaming(ctx, body),
		request:  body,
		toolCall: request.ToolCaller,
		messages: request.Messages,
	}

	// 设置流工厂函数，用于重新创建流
	s.factory = func() *ssestream.Stream[anthropic.MessageStreamEventUnion] {
		return c.Messages.NewStreaming(ctx, s.request)
	}
	return s
}

// Config 表示 Anthropic API 客户端的配置信息。
type Config struct {
	AuthToken          string        // 认证令牌，用于 API 身份验证
	BaseURL            string        // API 基础 URL 地址
	HTTPClient         *http.Client  // HTTP 客户端，用于发送请求
	EmptyMessagesLimit uint          // 空消息限制数量
}

// DefaultConfig 返回 Anthropic API 客户端的默认配置。
// 参数：
//   - authToken: 认证令牌
// 返回：
//   - Config: 包含默认设置的配置对象
func DefaultConfig(authToken string) Config {
	return Config{
		AuthToken:  authToken,
		HTTPClient: &http.Client{},
	}
}

// New 使用给定的配置创建新的 Anthropic 客户端。
// 参数：
//   - config: 客户端配置对象
// 返回：
//   - *Client: 初始化后的客户端实例
func New(config Config) *Client {
	// 构建请求选项列表
	opts := []option.RequestOption{
		option.WithAPIKey(config.AuthToken),
		option.WithHTTPClient(config.HTTPClient),
	}
	
	// 如果配置了自定义基础 URL，则添加到选项中
	// 移除 URL 末尾的 "/v1" 后缀以避免重复
	if config.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(strings.TrimSuffix(config.BaseURL, "/v1")))
	}
	
	// 创建 Anthropic 客户端并返回
	client := anthropic.NewClient(opts...)
	return &Client{
		Client: &client,
	}
}

// Stream 表示用于聊天补全的流式响应结构。
type Stream struct {
	done     bool                                                        // 流式传输是否完成的标志
	stream   *ssestream.Stream[anthropic.MessageStreamEventUnion]       // SSE 事件流
	request  anthropic.MessageNewParams                                  // 请求参数
	factory  func() *ssestream.Stream[anthropic.MessageStreamEventUnion] // 流工厂函数，用于重新创建流
	message  anthropic.Message                                           // 当前累积的消息
	toolCall func(name string, data []byte) (string, error)             // 工具调用处理函数
	messages []proto.Message                                             // 消息历史记录
}

// CallTools 实现 stream.Stream 接口，执行工具调用并返回调用状态。
// 遍历消息内容中的工具使用块，调用相应工具并构建响应消息。
// 返回：
//   - []proto.ToolCallStatus: 工具调用状态列表
func (s *Stream) CallTools() []proto.ToolCallStatus {
	var statuses []proto.ToolCallStatus
	
	// 遍历消息内容中的所有块
	for _, block := range s.message.Content {
		switch call := block.AsAny().(type) {
		case anthropic.ToolUseBlock:
			// 调用工具并获取结果
			msg, status := stream.CallTool(
				call.ID,
				call.Name,
				[]byte(call.JSON.Input.Raw()),
				s.toolCall,
			)
			
			// 构建工具结果消息块
			resp := anthropic.NewUserMessage(
				newToolResultBlock(
					call.ID,
					msg.Content,
					status.Err != nil,
				),
			)
			
			// 将工具结果添加到请求消息和消息历史中
			s.request.Messages = append(s.request.Messages, resp)
			s.messages = append(s.messages, msg)
			statuses = append(statuses, status)
		}
	}
	return statuses
}

// Close 实现 stream.Stream 接口，关闭流式连接。
// 返回：
//   - error: 关闭过程中可能发生的错误
func (s *Stream) Close() error { return s.stream.Close() } //nolint:wrapcheck

// Current 实现 stream.Stream 接口，获取当前流事件的内容块。
// 处理内容块增量事件，提取文本内容并返回。
// 返回：
//   - proto.Chunk: 内容块，包含文本内容
//   - error: 处理过程中可能发生的错误
func (s *Stream) Current() (proto.Chunk, error) {
	// 获取当前流事件
	event := s.stream.Current()
	
	// 累积事件到消息中
	if err := s.message.Accumulate(event); err != nil {
		return proto.Chunk{}, err //nolint:wrapcheck
	}
	
	// 根据事件类型处理内容
	switch eventVariant := event.AsAny().(type) {
	case anthropic.ContentBlockDeltaEvent:
		// 处理内容块增量事件
		switch deltaVariant := eventVariant.Delta.AsAny().(type) {
		case anthropic.TextDelta:
			// 返回文本增量内容
			return proto.Chunk{
				Content: deltaVariant.Text,
			}, nil
		}
	}
	
	// 无内容可返回
	return proto.Chunk{}, stream.ErrNoContent
}

// Err 实现 stream.Stream 接口，返回流式传输过程中的错误。
// 返回：
//   - error: 流式传输错误
func (s *Stream) Err() error { return s.stream.Err() } //nolint:wrapcheck

// Messages 实现 stream.Stream 接口，返回消息历史记录。
// 返回：
//   - []proto.Message: 消息列表
func (s *Stream) Messages() []proto.Message { return s.messages }

// Next 实现 stream.Stream 接口，推进到下一个流事件。
// 如果流已完成，则重置流并重新开始；否则推进到下一个事件。
// 返回：
//   - bool: 是否还有下一个事件
func (s *Stream) Next() bool {
	// 如果流已完成，重置状态并重新创建流
	if s.done {
		s.done = false
		s.stream = s.factory()
		s.message = anthropic.Message{}
	}

	// 尝试推进到下一个事件
	if s.stream.Next() {
		return true
	}

	// 流已结束，标记为完成并保存消息
	s.done = true
	s.request.Messages = append(s.request.Messages, s.message.ToParam())
	s.messages = append(s.messages, toProtoMessage(s.message.ToParam()))

	return false
}
