// Package ollama 为 Ollama 实现 [stream.Stream] 接口。
// 该包提供了与 Ollama API 交互的客户端实现，支持流式响应处理。
package ollama

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/charmbracelet/mods/internal/proto"
	"github.com/charmbracelet/mods/internal/stream"
	"github.com/ollama/ollama/api"
)

// 确保 Client 实现了 stream.Client 接口
var _ stream.Client = &Client{}

// Config 表示 Ollama API 客户端的配置信息。
// 该结构体包含了连接 Ollama 服务所需的所有配置参数。
type Config struct {
	// BaseURL Ollama 服务的基础 URL 地址
	BaseURL string
	// HTTPClient 自定义 HTTP 客户端，用于发送请求
	HTTPClient *http.Client
	// EmptyMessagesLimit 空消息的限制数量
	EmptyMessagesLimit uint
}

// DefaultConfig 返回 Ollama API 客户端的默认配置。
// 默认配置使用本地主机的 11434 端口作为 Ollama 服务地址。
func DefaultConfig() Config {
	return Config{
		BaseURL:    "http://localhost:11434/",
		HTTPClient: &http.Client{},
	}
}

// Client 表示 Ollama 客户端，封装了 Ollama API 客户端。
// 该客户端实现了 stream.Client 接口，支持流式对话交互。
type Client struct {
	*api.Client
}

// New 使用给定的 [Config] 创建一个新的 [Client] 实例。
// 参数:
//   - config: 客户端配置信息
// 返回:
//   - *Client: 新创建的客户端实例
//   - error: 解析 URL 失败时返回的错误
func New(config Config) (*Client, error) {
	// 解析基础 URL
	u, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	// 使用解析后的 URL 创建 Ollama API 客户端
	client := api.NewClient(u, config.HTTPClient)
	return &Client{
		Client: client,
	}, nil
}

// Request 实现 stream.Client 接口，向 Ollama 发送聊天请求。
// 该方法将 proto.Request 转换为 Ollama API 格式，并启动流式响应处理。
// 参数:
//   - ctx: 上下文，用于控制请求的生命周期
//   - request: 包含模型、消息、工具等信息的请求对象
// 返回:
//   - stream.Stream: 流式响应对象，用于迭代获取响应内容
func (c *Client) Request(ctx context.Context, request proto.Request) stream.Stream {
	b := true
	s := &Stream{
		toolCall: request.ToolCaller,
	}

	// 构建 Ollama 聊天请求
	body := api.ChatRequest{
		Model:    request.Model,                   // 指定使用的模型
		Messages: fromProtoMessages(request.Messages), // 转换消息格式
		Stream:   &b,                              // 启用流式响应
		Tools:    fromMCPTools(request.Tools),     // 转换工具定义
		Options:  map[string]any{},                // 初始化选项映射
	}

	// 设置停止标记（Stop Sequence）
	if len(request.Stop) > 0 {
		body.Options["stop"] = request.Stop[0]
	}

	// 设置上下文长度（最大 token 数）
	if request.MaxTokens != nil {
		body.Options["num_ctx"] = *request.MaxTokens
	}

	// 设置温度参数，控制输出的随机性
	if request.Temperature != nil {
		body.Options["temperature"] = *request.Temperature
	}

	// 设置 Top-P 采样参数
	if request.TopP != nil {
		body.Options["top_p"] = *request.TopP
	}

	s.request = body
	s.messages = request.Messages

	// 初始化流式响应处理工厂函数
	s.factory = func() {
		s.done = false
		s.err = nil
		s.respCh = make(chan api.ChatResponse)
		// 启动 goroutine 异步处理聊天响应
		go func() {
			if err := c.Chat(ctx, &s.request, s.fn); err != nil {
				s.err = err
			}
		}()
	}

	s.factory()
	return s
}

// Stream 表示 Ollama 的流式响应，实现了 stream.Stream 接口。
// 该结构体管理流式响应的状态、消息累积和工具调用处理。
type Stream struct {
	request  api.ChatRequest                              // 聊天请求对象
	err      error                                        // 存储可能发生的错误
	done     bool                                         // 标记响应是否完成
	factory  func()                                       // 重置并重新启动流的工厂函数
	respCh   chan api.ChatResponse                        // 响应通道，用于接收流式响应
	message  api.Message                                  // 累积的消息内容
	toolCall func(name string, data []byte) (string, error) // 工具调用处理函数
	messages []proto.Message                              // 消息历史记录
}

// fn 是响应回调函数，将响应发送到通道中。
// 参数:
//   - resp: Ollama API 返回的聊天响应
// 返回:
//   - error: 总是返回 nil
func (s *Stream) fn(resp api.ChatResponse) error {
	s.respCh <- resp
	return nil
}

// CallTools 实现 stream.Stream 接口，执行消息中的所有工具调用。
// 该方法遍历消息中的工具调用，执行每个工具并更新请求和消息历史。
// 返回:
//   - []proto.ToolCallStatus: 工具调用的执行状态列表
func (s *Stream) CallTools() []proto.ToolCallStatus {
	statuses := make([]proto.ToolCallStatus, 0, len(s.message.ToolCalls))

	// 遍历所有工具调用并执行
	for _, call := range s.message.ToolCalls {
		// 调用工具并获取结果和状态
		msg, status := stream.CallTool(
			strconv.Itoa(call.Function.Index),        // 工具调用索引
			call.Function.Name,                       // 工具名称
			[]byte(call.Function.Arguments.String()), // 工具参数
			s.toolCall,                               // 工具调用处理函数
		)

		// 将工具响应添加到请求消息中
		s.request.Messages = append(s.request.Messages, fromProtoMessage(msg))
		s.messages = append(s.messages, msg)
		statuses = append(statuses, status)
	}
	return statuses
}

// Close 实现 stream.Stream 接口，关闭流式响应。
// 该方法关闭响应通道并标记流已完成。
// 返回:
//   - error: 总是返回 nil
func (s *Stream) Close() error {
	close(s.respCh)
	s.done = true
	return nil
}

// Current 实现 stream.Stream 接口，获取当前的响应块。
// 该方法从响应通道中读取最新的响应内容，并累积到消息中。
// 返回:
//   - proto.Chunk: 当前响应的内容块
//   - error: 没有内容时返回 stream.ErrNoContent
func (s *Stream) Current() (proto.Chunk, error) {
	select {
	case resp := <-s.respCh:
		// 构建响应块
		chunk := proto.Chunk{
			Content: resp.Message.Content,
		}
		// 累积消息内容
		s.message.Content += resp.Message.Content
		// 累积工具调用
		s.message.ToolCalls = append(s.message.ToolCalls, resp.Message.ToolCalls...)

		// 检查响应是否完成
		if resp.Done {
			s.done = true
		}
		return chunk, nil
	default:
		// 没有可用内容时返回错误
		return proto.Chunk{}, stream.ErrNoContent
	}
}

// Err 实现 stream.Stream 接口，返回流处理过程中发生的错误。
// 返回:
//   - error: 流处理过程中的错误，如果没有错误则返回 nil
func (s *Stream) Err() error { return s.err }

// Messages 实现 stream.Stream 接口，返回消息历史记录。
// 返回:
//   - []proto.Message: 消息历史列表
func (s *Stream) Messages() []proto.Message { return s.messages }

// Next 实现 stream.Stream 接口，准备下一次迭代。
// 该方法检查是否有错误或流已完成，并在需要时重置流状态。
// 返回:
//   - bool: 是否可以继续迭代
func (s *Stream) Next() bool {
	// 如果有错误，停止迭代
	if s.err != nil {
		return false
	}

	// 如果流已完成，重置状态并准备新一轮对话
	if s.done {
		s.done = false
		s.factory()
		// 将累积的消息添加到历史记录中
		s.messages = append(s.messages, toProtoMessage(s.message))
		s.request.Messages = append(s.request.Messages, s.message)
		// 重置消息对象
		s.message = api.Message{}
	}
	return true
}
