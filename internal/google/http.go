package google

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// httpHeader 是 http.Header 的类型别名，用于嵌入到其他结构体中
type httpHeader http.Header

// ErrTooManyEmptyStreamMessages 表示流发送了过多空消息的错误。
// 当流式响应中连续出现超过限制数量的空消息时，会返回此错误。
var ErrTooManyEmptyStreamMessages = errors.New("流发送了过多空消息")

// Marshaller 是将值序列化为字节数组的接口。
// 该接口定义了数据序列化的标准方法。
type Marshaller interface {
	// Marshal 将任意值序列化为字节数组
	// 参数：
	//   - value: 要序列化的值
	// 返回：
	//   - []byte: 序列化后的字节数组
	//   - error: 序列化过程中的错误
	Marshal(value any) ([]byte, error)
}

// JSONMarshaller 是将值序列化为 JSON 格式的序列化器。
// 该结构体实现了 Marshaller 接口。
type JSONMarshaller struct{}

// Marshal 将值序列化为 JSON 格式的字节数组。
// 参数：
//   - value: 要序列化的值
// 返回：
//   - []byte: JSON 格式的字节数组
//   - error: 序列化过程中的错误
func (jm *JSONMarshaller) Marshal(value any) ([]byte, error) {
	result, err := json.Marshal(value)
	if err != nil {
		return result, fmt.Errorf("JSONMarshaller.Marshal: %w", err)
	}
	return result, nil
}

// HTTPRequestBuilder 是构建 HTTP 请求的实现。
// 该结构体实现了 RequestBuilder 接口，用于构建发送给 API 的 HTTP 请求。
type HTTPRequestBuilder struct {
	// marshaller 用于序列化请求体
	marshaller Marshaller
}

// Build 构建一个 HTTP 请求。
// 该方法支持多种类型的请求体，包括 io.Reader 和可序列化的对象。
// 参数：
//   - ctx: 上下文，用于控制请求的生命周期
//   - method: HTTP 方法（如 GET、POST、PUT 等）
//   - url: 请求的目标 URL
//   - body: 请求体，可以是 io.Reader 或可序列化的对象
//   - header: HTTP 请求头
// 返回：
//   - req: 构建的 HTTP 请求对象
//   - err: 构建过程中的错误
func (b *HTTPRequestBuilder) Build(
	ctx context.Context,
	method string,
	url string,
	body any,
	header http.Header,
) (req *http.Request, err error) {
	var bodyReader io.Reader
	// 处理请求体
	if body != nil {
		// 如果请求体已经是 io.Reader 类型，直接使用
		if v, ok := body.(io.Reader); ok {
			bodyReader = v
		} else {
			// 否则，将请求体序列化为 JSON
			var reqBytes []byte
			reqBytes, err = b.marshaller.Marshal(body)
			if err != nil {
				return
			}
			bodyReader = bytes.NewBuffer(reqBytes)
		}
	}
	// 创建 HTTP 请求
	req, err = http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return
	}
	// 设置请求头
	if header != nil {
		req.Header = header
	}
	return
}

// requestOptions 包含构建请求的选项
type requestOptions struct {
	// body 是请求体内容
	body MessageCompletionRequest
	// header 是 HTTP 请求头
	header http.Header
}

// requestOption 是用于设置 requestOptions 的函数类型
type requestOption func(*requestOptions)

// withBody 返回一个设置请求体的选项函数。
// 参数：
//   - body: 要设置的请求体
// 返回：
//   - requestOption: 选项函数
func withBody(body MessageCompletionRequest) requestOption {
	return func(args *requestOptions) {
		args.body = body
	}
}

// ErrorAccumulator 是累积错误的接口。
// 该接口用于收集和处理错误信息。
type ErrorAccumulator interface {
	// Write 写入字节数据
	Write(p []byte) error
	// Bytes 返回累积的字节数据
	Bytes() []byte
}

// Unmarshaler 是反序列化字节数组的接口。
// 该接口定义了数据反序列化的标准方法。
type Unmarshaler interface {
	// Unmarshal 将字节数组反序列化为指定的值
	// 参数：
	//   - data: 要反序列化的字节数组
	//   - v: 反序列化结果的目标对象
	// 返回：
	//   - error: 反序列化过程中的错误
	Unmarshal(data []byte, v any) error
}

// isFailureStatusCode 检查 HTTP 状态码是否表示失败。
// 状态码小于 200 或大于等于 400 被视为失败。
// 参数：
//   - resp: HTTP 响应对象
// 返回：
//   - bool: 如果状态码表示失败返回 true，否则返回 false
func isFailureStatusCode(resp *http.Response) bool {
	return resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest
}

// JSONUnmarshaler 是反序列化 JSON 数据的反序列化器。
// 该结构体实现了 Unmarshaler 接口。
type JSONUnmarshaler struct{}

// Unmarshal 将 JSON 格式的字节数组反序列化为指定的值。
// 参数：
//   - data: JSON 格式的字节数组
//   - v: 反序列化结果的目标对象
// 返回：
//   - error: 反序列化过程中的错误
func (jm *JSONUnmarshaler) Unmarshal(data []byte, v any) error {
	err := json.Unmarshal(data, v)
	if err != nil {
		return fmt.Errorf("JSONUnmarshaler.Unmarshal: %w", err)
	}
	return nil
}
