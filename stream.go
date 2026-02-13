package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/mods/internal/proto"
)

// setupStreamContext 设置流上下文
func (m *Mods) setupStreamContext(content string, mod Model) error {
	cfg := m.Config
	m.messages = []proto.Message{}
	// 如果配置了格式化文本，添加系统消息
	if txt := cfg.FormatText[cfg.FormatAs]; cfg.Format && txt != "" {
		m.messages = append(m.messages, proto.Message{
			Role:    proto.RoleSystem,
			Content: txt,
		})
	}

	// 如果配置了角色，加载角色设置
	if cfg.Role != "" {
		roleSetup, ok := cfg.Roles[cfg.Role]
		if !ok {
			return modsError{
				err:    fmt.Errorf("角色 %q 不存在", cfg.Role),
				reason: "无法使用角色",
			}
		}
		for _, msg := range roleSetup {
			content, err := loadMsg(msg)
			if err != nil {
				return modsError{
					err:    err,
					reason: "无法使用角色",
				}
			}
			m.messages = append(m.messages, proto.Message{
				Role:    proto.RoleSystem,
				Content: content,
			})
		}
	}

	// 如果配置了前缀，添加到内容
	if prefix := cfg.Prefix; prefix != "" {
		content = strings.TrimSpace(prefix + "\n\n" + content)
	}

	// 如果未配置无限制且内容超过最大字符数，截断内容
	if !cfg.NoLimit && int64(len(content)) > mod.MaxChars {
		content = content[:mod.MaxChars]
	}

	// 如果未配置无缓存且配置了读取缓存 ID，从缓存读取
	if !cfg.NoCache && cfg.cacheReadFromID != "" {
		if err := m.cache.Read(cfg.cacheReadFromID, &m.messages); err != nil {
			return modsError{
				err: err,
				reason: fmt.Sprintf(
					"读取缓存时出现问题。使用 %s / %s 禁用它。",
					m.Styles.InlineCode.Render("--no-cache"),
					m.Styles.InlineCode.Render("NO_CACHE"),
				),
			}
		}
	}

	// 添加用户消息
	m.messages = append(m.messages, proto.Message{
		Role:    proto.RoleUser,
		Content: content,
	})

	return nil
}
