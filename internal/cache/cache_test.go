package cache

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/charmbracelet/mods/internal/proto"
	"github.com/stretchr/testify/require"
)

// TestCache 测试缓存的基本功能
func TestCache(t *testing.T) {
	// 测试读取不存在的缓存项
	t.Run("读取不存在的缓存", func(t *testing.T) {
		cache, err := NewConversations(t.TempDir())
		require.NoError(t, err)
		err = cache.Read("super-fake", &[]proto.Message{})
		require.ErrorIs(t, err, os.ErrNotExist)
	})

	// 测试写入缓存
	t.Run("写入", func(t *testing.T) {
		cache, err := NewConversations(t.TempDir())
		require.NoError(t, err)
		messages := []proto.Message{
			{
				Role:    proto.RoleUser,
				Content: "前4个自然数",
			},
			{
				Role:    proto.RoleAssistant,
				Content: "1, 2, 3, 4",
			},
		}
		require.NoError(t, cache.Write("fake", &messages))

		result := []proto.Message{}
		require.NoError(t, cache.Read("fake", &result))

		require.ElementsMatch(t, messages, result)
	})

	// 测试删除缓存
	t.Run("删除", func(t *testing.T) {
		cache, err := NewConversations(t.TempDir())
		require.NoError(t, err)
		cache.Write("fake", &[]proto.Message{})
		require.NoError(t, cache.Delete("fake"))
		require.ErrorIs(t, cache.Read("fake", nil), os.ErrNotExist)
	})

	// 测试无效标识符
	t.Run("无效标识符", func(t *testing.T) {
		// 测试写入时使用无效标识符
		t.Run("写入", func(t *testing.T) {
			cache, err := NewConversations(t.TempDir())
			require.NoError(t, err)
			require.ErrorIs(t, cache.Write("", nil), errInvalidID)
		})
		// 测试删除时使用无效标识符
		t.Run("删除", func(t *testing.T) {
			cache, err := NewConversations(t.TempDir())
			require.NoError(t, err)
			require.ErrorIs(t, cache.Delete(""), errInvalidID)
		})
		// 测试读取时使用无效标识符
		t.Run("读取", func(t *testing.T) {
			cache, err := NewConversations(t.TempDir())
			require.NoError(t, err)
			require.ErrorIs(t, cache.Read("", nil), errInvalidID)
		})
	})
}

// TestExpiringCache 测试过期缓存的功能
func TestExpiringCache(t *testing.T) {
	// 测试写入和读取
	t.Run("写入和读取", func(t *testing.T) {
		cache, err := NewExpiring[string](t.TempDir())
		require.NoError(t, err)

		// 写入一个带过期时间的值
		data := "测试数据"
		expiresAt := time.Now().Add(time.Hour).Unix()
		err = cache.Write("test", expiresAt, func(w io.Writer) error {
			_, err := w.Write([]byte(data))
			return err
		})
		require.NoError(t, err)

		// 读回数据
		var result string
		err = cache.Read("test", func(r io.Reader) error {
			b, err := io.ReadAll(r)
			if err != nil {
				return err
			}
			result = string(b)
			return nil
		})
		require.NoError(t, err)
		require.Equal(t, data, result)
	})

	// 测试已过期的令牌
	t.Run("已过期的令牌", func(t *testing.T) {
		cache, err := NewExpiring[string](t.TempDir())
		require.NoError(t, err)

		// 写入一个已经过期的值
		data := "测试数据"
		expiresAt := time.Now().Add(-time.Hour).Unix() // 1小时前已过期
		err = cache.Write("test", expiresAt, func(w io.Writer) error {
			_, err := w.Write([]byte(data))
			return err
		})
		require.NoError(t, err)

		// 尝试读取它
		err = cache.Read("test", func(r io.Reader) error {
			return nil
		})
		require.Error(t, err)
		require.True(t, os.IsNotExist(err))
	})

	// 测试覆盖令牌
	t.Run("覆盖令牌", func(t *testing.T) {
		cache, err := NewExpiring[string](t.TempDir())
		require.NoError(t, err)

		// 写入初始值
		data1 := "测试数据 1"
		expiresAt1 := time.Now().Add(time.Hour).Unix()
		err = cache.Write("test", expiresAt1, func(w io.Writer) error {
			_, err := w.Write([]byte(data1))
			return err
		})
		require.NoError(t, err)

		// 写入新值
		data2 := "测试数据 2"
		expiresAt2 := time.Now().Add(2 * time.Hour).Unix()
		err = cache.Write("test", expiresAt2, func(w io.Writer) error {
			_, err := w.Write([]byte(data2))
			return err
		})
		require.NoError(t, err)

		// 读回数据 - 应该获取到新值
		var result string
		err = cache.Read("test", func(r io.Reader) error {
			b, err := io.ReadAll(r)
			if err != nil {
				return err
			}
			result = string(b)
			return nil
		})
		require.NoError(t, err)
		require.Equal(t, data2, result)
	})
}
