package cache

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"io"

	"github.com/charmbracelet/mods/internal/proto"
)

// Conversations 是对话缓存结构。
type Conversations struct {
	cache *Cache[[]proto.Message] // 底层缓存实例
}

// NewConversations 创建一个新的对话缓存实例。
func NewConversations(dir string) (*Conversations, error) {
	cache, err := New[[]proto.Message](dir, ConversationCache)
	if err != nil {
		return nil, err
	}
	return &Conversations{
		cache: cache,
	}, nil
}

// Read 通过指定的标识符读取对话消息列表。
func (c *Conversations) Read(id string, messages *[]proto.Message) error {
	return c.cache.Read(id, func(r io.Reader) error {
		return decode(r, messages)
	})
}

// Write 通过指定的标识符写入对话消息列表。
func (c *Conversations) Write(id string, messages *[]proto.Message) error {
	return c.cache.Write(id, func(w io.Writer) error {
		return encode(w, messages)
	})
}

// Delete 删除指定标识符的对话缓存。
func (c *Conversations) Delete(id string) error {
	return c.cache.Delete(id)
}

func init() {
	gob.Register(errors.New(""))
}

// encode 使用 gob 编码将消息列表编码到写入器中。
func encode(w io.Writer, messages *[]proto.Message) error {
	if err := gob.NewEncoder(w).Encode(messages); err != nil {
		return fmt.Errorf("编码: %w", err)
	}
	return nil
}

// decode 使用 gob 解码从读取器中解码消息列表。
// 我们使用 TeeReader 以防用户尝试读取旧格式（MCP 之前）的消息，
// 如果是这种情况，则在类型之间进行转换以避免编码错误。
func decode(r io.Reader, messages *[]proto.Message) error {
	var tr bytes.Buffer
	if err1 := gob.NewDecoder(io.TeeReader(r, &tr)).Decode(messages); err1 != nil {
		var noCalls []noCallMessage
		if err2 := gob.NewDecoder(&tr).Decode(&noCalls); err2 != nil {
			return fmt.Errorf("解码: %w", err1)
		}
		for _, msg := range noCalls {
			*messages = append(*messages, proto.Message{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}
	return nil
}

// noCallMessage 用于兼容没有工具调用（tool calls）的消息格式。
type noCallMessage struct {
	Content string // 消息内容
	Role    string // 角色类型
}
