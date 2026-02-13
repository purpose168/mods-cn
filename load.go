package main

import (
	"io"
	"net/http"
	"os"
	"strings"
)

// loadMsg 加载消息内容
// msg: 消息字符串，可以是普通文本、URL 或文件路径
// 返回：消息内容和错误信息
func loadMsg(msg string) (string, error) {
	// 处理 HTTP/HTTPS URL
	if strings.HasPrefix(msg, "https://") || strings.HasPrefix(msg, "http://") {
		resp, err := http.Get(msg) //nolint:gosec,noctx
		if err != nil {
			return "", err //nolint:wrapcheck
		}
		defer func() { _ = resp.Body.Close() }()
		bts, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err //nolint:wrapcheck
		}
		return string(bts), nil
	}

	// 处理文件路径
	if strings.HasPrefix(msg, "file://") {
		bts, err := os.ReadFile(strings.TrimPrefix(msg, "file://"))
		if err != nil {
			return "", err //nolint:wrapcheck
		}
		return string(bts), nil
	}

	// 返回原始消息
	return msg, nil
}
