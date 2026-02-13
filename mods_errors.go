package main

import (
	"errors"
	"fmt"
	"net/http"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/openai/openai-go"
)

// handleRequestError 处理请求错误
func (m *Mods) handleRequestError(err error, mod Model, content string) tea.Msg {
	ae := &openai.Error{}
	if errors.As(err, &ae) {
		return m.handleAPIError(ae, mod, content)
	}
	return modsError{err, fmt.Sprintf(
		"%s API 请求出现问题。",
		mod.API,
	)}
}

// handleAPIError 处理 API 错误
func (m *Mods) handleAPIError(err *openai.Error, mod Model, content string) tea.Msg {
	cfg := m.Config
	switch err.StatusCode {
	case http.StatusNotFound:
		// 如果配置了回退模型，尝试使用回退模型
		if mod.Fallback != "" {
			m.Config.Model = mod.Fallback
			return m.retry(content, modsError{
				err:    err,
				reason: fmt.Sprintf("%s API 服务器错误。", mod.API),
			})
		}
		return modsError{err: err, reason: fmt.Sprintf(
			"API '%s' 缺少模型 '%s'。",
			cfg.API,
			cfg.Model,
		)}
	case http.StatusBadRequest:
		// 处理上下文长度超出错误
		if err.Code == "context_length_exceeded" {
			pe := modsError{err: err, reason: "超出最大提示词大小。"}
			if cfg.NoLimit {
				return pe
			}

			return m.retry(cutPrompt(err.Message, content), pe)
		}
		// 错误请求（不重试）
		return modsError{err: err, reason: fmt.Sprintf("%s API 请求错误。", mod.API)}
	case http.StatusUnauthorized:
		// 无效的认证或密钥（不重试）
		return modsError{err: err, reason: fmt.Sprintf("无效的 %s API 密钥。", mod.API)}
	case http.StatusTooManyRequests:
		// 速率限制或引擎过载（等待并重试）
		return m.retry(content, modsError{
			err: err, reason: fmt.Sprintf("您已达到 %s API 速率限制。", mod.API),
		})
	case http.StatusInternalServerError:
		if mod.API == "openai" {
			return m.retry(content, modsError{err: err, reason: "OpenAI API 服务器错误。"})
		}
		return modsError{err: err, reason: fmt.Sprintf(
			"API '%s' 加载模型 '%s' 出错。",
			mod.API,
			mod.Name,
		)}
	default:
		return m.retry(content, modsError{err: err, reason: "未知的 API 错误。"})
	}
}
