// Package google 实现了 Google API 的 [stream.Stream] 接口。
// 该包提供了与 Google Generative AI API 交互的客户端实现，
// 支持流式响应处理和消息补全功能。
package google

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/charmbracelet/mods/internal/proto"
	"github.com/charmbracelet/mods/internal/stream"
	"github.com/openai/openai-go"
)

// 确保 Client 实现了 stream.Client 接口
var _ stream.Client = &Client{}

// emptyMessagesLimit 定义了流中允许的空消息数量上限
const emptyMessagesLimit uint = 300

var (
	// googleHeaderData 是 Google API 流式响应的数据前缀
	googleHeaderData = []byte("data: ")
	// errorPrefix 是错误事件的前缀标识
	errorPrefix = []byte(`event: error`)
)

// Config 表示 Google API 客户端的配置信息。
// 该结构体包含了连接 Google API 所需的所有配置参数。
type Config struct {
	// BaseURL 是 Google API 的基础 URL 地址
	BaseURL string
	// HTTPClient 是用于发送 HTTP 请求的客户端实例
	HTTPClient *http.Client
	// ThinkingBudget 设置模型的思考预算（thinking budget），
	// 用于控制模型在生成响应时的思考深度
	ThinkingBudget int
}

// DefaultConfig 返回 Google API 客户端的默认配置。
// 参数：
//   - model: 要使用的模型名称
//   - authToken: API 认证令牌
// 返回：
//   - Config: 包含默认设置的配置对象
func DefaultConfig(model, authToken string) Config {
	return Config{
		BaseURL:    fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:streamGenerateContent?alt=sse&key=%s", model, authToken),
		HTTPClient: &http.Client{},
	}
}

// Part 是包含媒体内容的数据类型，作为多部分 Content 消息的一部分。
// 每个 Part 代表消息内容中的一个独立片段。
type Part struct {
	// Text 包含文本内容
	Text string `json:"text,omitempty"`
}

// Content 是包含多部分消息内容的基础结构化数据类型。
// 它是 Google API 中消息的基本单位。
type Content struct {
	// Parts 包含消息的各个部分内容
	Parts []Part `json:"parts,omitempty"`
	// Role 表示消息的角色（如 "user" 或 "model"）
	Role string `json:"role,omitempty"`
}

// ThinkingConfig 配置模型的思考模式参数。
// 更多详情请参阅：https://ai.google.dev/gemini-api/docs/thinking#rest
type ThinkingConfig struct {
	// ThinkingBudget 设置思考预算值
	ThinkingBudget int `json:"thinkingBudget,omitempty"`
}

// GenerationConfig 包含模型生成和输出的配置选项。
// 注意：并非所有参数都适用于每个模型。
type GenerationConfig struct {
	// StopSequences 定义停止生成的序列字符串列表
	StopSequences []string `json:"stopSequences,omitempty"`
	// ResponseMimeType 指定响应的 MIME 类型
	ResponseMimeType string `json:"responseMimeType,omitempty"`
	// CandidateCount 指定要生成的候选响应数量
	CandidateCount uint `json:"candidateCount,omitempty"`
	// MaxOutputTokens 设置输出令牌的最大数量
	MaxOutputTokens uint `json:"maxOutputTokens,omitempty"`
	// Temperature 控制生成内容的随机性（0.0-1.0）
	Temperature float64 `json:"temperature,omitempty"`
	// TopP 设置核采样（nucleus sampling）的概率阈值
	TopP float64 `json:"topP,omitempty"`
	// TopK 设置从概率最高的 K 个令牌中采样的数量
	TopK int64 `json:"topK,omitempty"`
	// ThinkingConfig 配置思考模式的参数
	ThinkingConfig *ThinkingConfig `json:"thinkingConfig,omitempty"`
}

// MessageCompletionRequest 表示消息补全请求的有效参数和值选项。
// 该结构体封装了发送给 Google API 的完整请求体。
type MessageCompletionRequest struct {
	// Contents 包含对话历史消息列表
	Contents []Content `json:"contents,omitempty"`
	// GenerationConfig 包含生成配置选项
	GenerationConfig GenerationConfig `json:"generationConfig,omitempty"`
}

// RequestBuilder 是构建 Google API HTTP 请求的接口。
// 该接口定义了构建请求的标准方法。
type RequestBuilder interface {
	// Build 构建一个 HTTP 请求
	// 参数：
	//   - ctx: 上下文
	//   - method: HTTP 方法（如 GET、POST）
	//   - url: 请求 URL
	//   - body: 请求体
	//   - header: HTTP 头部
	// 返回：
	//   - *http.Request: 构建的请求对象
	//   - error: 错误信息
	Build(ctx context.Context, method, url string, body any, header http.Header) (*http.Request, error)
}

// NewRequestBuilder 创建一个新的 HTTPRequestBuilder 实例。
// 返回：
//   - *HTTPRequestBuilder: 新的请求构建器实例
func NewRequestBuilder() *HTTPRequestBuilder {
	return &HTTPRequestBuilder{
		marshaller: &JSONMarshaller{},
	}
}

// Client 是 Google API 的客户端实现。
// 该客户端负责与 Google Generative AI API 进行通信。
type Client struct {
	// config 存储客户端配置
	config Config
	// requestBuilder 用于构建 HTTP 请求
	requestBuilder RequestBuilder
}

// Request 实现 stream.Client 接口，发送请求到 Google API。
// 该方法构建请求体并发送流式请求。
// 参数：
//   - ctx: 上下文，用于控制请求的生命周期
//   - request: 协议层的请求对象，包含消息和配置
// 返回：
//   - stream.Stream: 流式响应对象
func (c *Client) Request(ctx context.Context, request proto.Request) stream.Stream {
	// 创建新的流对象
	stream := new(Stream)
	// 构建请求体
	body := MessageCompletionRequest{
		Contents: fromProtoMessages(request.Messages),
		GenerationConfig: GenerationConfig{
			ResponseMimeType: "",
			CandidateCount:   1,
			StopSequences:    request.Stop,
			MaxOutputTokens:  4096,
		},
	}

	// 设置温度参数（如果提供）
	if request.Temperature != nil {
		body.GenerationConfig.Temperature = *request.Temperature
	}
	// 设置 TopP 参数（如果提供）
	if request.TopP != nil {
		body.GenerationConfig.TopP = *request.TopP
	}
	// 设置 TopK 参数（如果提供）
	if request.TopK != nil {
		body.GenerationConfig.TopK = *request.TopK
	}

	// 设置最大输出令牌数（如果提供）
	if request.MaxTokens != nil {
		body.GenerationConfig.MaxOutputTokens = uint(*request.MaxTokens) //nolint:gosec
	}

	// 设置思考预算配置（如果提供）
	if c.config.ThinkingBudget != 0 {
		body.GenerationConfig.ThinkingConfig = &ThinkingConfig{
			ThinkingBudget: c.config.ThinkingBudget,
		}
	}

	// 构建新的 HTTP 请求
	req, err := c.newRequest(ctx, http.MethodPost, c.config.BaseURL, withBody(body))
	if err != nil {
		stream.err = err
		return stream
	}

	// 发送流式请求
	stream, err = googleSendRequestStream(c, req)
	if err != nil {
		stream.err = err
	}
	return stream
}

// New 使用给定的配置创建一个新的 Client 实例。
// 参数：
//   - config: 客户端配置对象
// 返回：
//   - *Client: 新的客户端实例
func New(config Config) *Client {
	return &Client{
		config:         config,
		requestBuilder: NewRequestBuilder(),
	}
}

// newRequest 创建一个新的 HTTP 请求。
// 该方法支持通过选项模式配置请求参数。
// 参数：
//   - ctx: 上下文
//   - method: HTTP 方法
//   - url: 请求 URL
//   - setters: 请求选项函数列表
// 返回：
//   - *http.Request: 构建的请求对象
//   - error: 错误信息
func (c *Client) newRequest(ctx context.Context, method, url string, setters ...requestOption) (*http.Request, error) {
	// 初始化默认选项
	args := &requestOptions{
		body:   MessageCompletionRequest{},
		header: make(http.Header),
	}
	// 应用所有选项
	for _, setter := range setters {
		setter(args)
	}
	// 构建请求
	req, err := c.requestBuilder.Build(ctx, method, url, args.body, args.header)
	if err != nil {
		return new(http.Request), err
	}
	return req, nil
}

// handleErrorResp 处理错误响应。
// 该方法解析 HTTP 错误响应并返回相应的错误对象。
// 参数：
//   - resp: HTTP 响应对象
// 返回：
//   - error: 解析后的错误对象
func (c *Client) handleErrorResp(resp *http.Response) error {
	// 解析响应体中的错误信息
	var errRes openai.Error
	if err := json.NewDecoder(resp.Body).Decode(&errRes); err != nil {
		return &openai.Error{
			StatusCode: resp.StatusCode,
			Message:    err.Error(),
		}
	}
	errRes.StatusCode = resp.StatusCode
	return &errRes
}

// Candidate 表示模型生成的响应候选。
// 每个候选包含生成的内容和相关的元数据。
type Candidate struct {
	// Content 包含候选的内容
	Content Content `json:"content,omitempty"`
	// FinishReason 表示生成结束的原因
	FinishReason string `json:"finishReason,omitempty"`
	// TokenCount 表示令牌数量
	TokenCount uint `json:"tokenCount,omitempty"`
	// Index 表示候选在候选列表中的索引
	Index uint `json:"index,omitempty"`
}

// CompletionMessageResponse 表示 Google 补全消息的响应。
// 该结构体封装了 API 返回的完整响应数据。
type CompletionMessageResponse struct {
	// Candidates 包含生成的候选响应列表
	Candidates []Candidate `json:"candidates,omitempty"`
}

// Stream 表示来自 Google API 的消息流。
// 该结构体实现了流式读取 API 响应的功能。
type Stream struct {
	// isFinished 标记流是否已结束
	isFinished bool
	// reader 用于读取流数据的缓冲读取器
	reader *bufio.Reader
	// response HTTP 响应对象
	response *http.Response
	// err 存储流处理过程中的错误
	err error
	// unmarshaler 用于反序列化 JSON 数据
	unmarshaler Unmarshaler

	// httpHeader 嵌入的 HTTP 头部
	httpHeader
}

// CallTools 实现 stream.Stream 接口。
// 返回工具调用状态列表。
// 注意：Gemini/Google API 目前尚不支持工具调用。
// 返回：
//   - []proto.ToolCallStatus: 工具调用状态列表（当前为 nil）
func (s *Stream) CallTools() []proto.ToolCallStatus {
	// Gemini/Google API 目前尚不支持工具调用
	return nil
}

// Err 实现 stream.Stream 接口。
// 返回流处理过程中发生的错误。
// 返回：
//   - error: 错误对象
func (s *Stream) Err() error { return s.err }

// Messages 实现 stream.Stream 接口。
// 返回流式消息列表。
// 注意：Gemini 不支持在事后返回流式消息。
// 返回：
//   - []proto.Message: 消息列表（当前为 nil）
func (s *Stream) Messages() []proto.Message {
	// Gemini 不支持在事后返回流式消息
	return nil
}

// Next 实现 stream.Stream 接口。
// 检查流是否还有更多数据可读。
// 返回：
//   - bool: 如果流未结束返回 true，否则返回 false
func (s *Stream) Next() bool {
	return !s.isFinished
}

// Close 关闭流并释放相关资源。
// 返回：
//   - error: 关闭过程中发生的错误
func (s *Stream) Close() error {
	return s.response.Body.Close() //nolint:wrapcheck
}

// Current 实现 stream.Stream 接口。
// 读取并返回流中的当前数据块。
// 该方法处理流式响应的解析和错误处理。
// 返回：
//   - proto.Chunk: 数据块
//   - error: 错误信息
//
//nolint:gocognit
func (s *Stream) Current() (proto.Chunk, error) {
	var (
		emptyMessagesCount uint // 空消息计数器
		hasError           bool // 错误标志
	)

	for {
		// 读取一行数据
		rawLine, readErr := s.reader.ReadBytes('\n')
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				s.isFinished = true
				return proto.Chunk{}, stream.ErrNoContent // 表示流结束，不是真正的错误
			}
			return proto.Chunk{}, fmt.Errorf("googleStreamReader.processLines: %w", readErr)
		}

		// 去除首尾空白字符
		noSpaceLine := bytes.TrimSpace(rawLine)

		// 检查是否为错误事件
		if bytes.HasPrefix(noSpaceLine, errorPrefix) {
			hasError = true
			// 注意：继续到下一个事件以获取错误数据
			continue
		}

		// 处理非数据行或错误情况
		if !bytes.HasPrefix(noSpaceLine, googleHeaderData) || hasError {
			if hasError {
				noSpaceLine = bytes.TrimPrefix(noSpaceLine, googleHeaderData)
				return proto.Chunk{}, fmt.Errorf("googleStreamReader.processLines: %s", noSpaceLine)
			}
			emptyMessagesCount++
			if emptyMessagesCount > emptyMessagesLimit {
				return proto.Chunk{}, ErrTooManyEmptyStreamMessages
			}
			continue
		}

		// 移除数据前缀
		noPrefixLine := bytes.TrimPrefix(noSpaceLine, googleHeaderData)

		// 反序列化 JSON 数据
		var chunk CompletionMessageResponse
		unmarshalErr := s.unmarshaler.Unmarshal(noPrefixLine, &chunk)
		if unmarshalErr != nil {
			return proto.Chunk{}, fmt.Errorf("googleStreamReader.processLines: %w", unmarshalErr)
		}
		// 检查是否有候选响应
		if len(chunk.Candidates) == 0 {
			return proto.Chunk{}, stream.ErrNoContent
		}
		// 检查候选响应是否有内容部分
		parts := chunk.Candidates[0].Content.Parts
		if len(parts) == 0 {
			return proto.Chunk{}, stream.ErrNoContent
		}

		// 返回第一个候选的第一个部分的文本内容
		return proto.Chunk{
			Content: chunk.Candidates[0].Content.Parts[0].Text,
		}, nil
	}
}

// googleSendRequestStream 发送流式请求到 Google API。
// 该方法设置请求头并发送 HTTP 请求，返回流式响应对象。
// 参数：
//   - client: Google API 客户端
//   - req: HTTP 请求对象
// 返回：
//   - *Stream: 流式响应对象
//   - error: 错误信息
func googleSendRequestStream(client *Client, req *http.Request) (*Stream, error) {
	// 设置请求内容类型为 JSON
	req.Header.Set("content-type", "application/json")

	// 发送 HTTP 请求
	resp, err := client.config.HTTPClient.Do(req) //nolint:bodyclose // body 在 stream.Close() 中关闭
	if err != nil {
		return new(Stream), err
	}
	// 检查响应状态码
	if isFailureStatusCode(resp) {
		return new(Stream), client.handleErrorResp(resp)
	}
	// 返回流对象
	return &Stream{
		reader:      bufio.NewReader(resp.Body),
		response:    resp,
		unmarshaler: &JSONUnmarshaler{},
		httpHeader:  httpHeader(resp.Header),
	}, nil
}
