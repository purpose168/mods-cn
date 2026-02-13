package cache

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ExpiringCache 是支持缓存项过期功能的缓存实现。
type ExpiringCache[T any] struct {
	cache *Cache[T] // 底层缓存实例
}

// NewExpiring 创建一个支持缓存项过期的新缓存实例。
func NewExpiring[T any](path string) (*ExpiringCache[T], error) {
	cache, err := New[T](path, TemporaryCache)
	if err != nil {
		return nil, fmt.Errorf("创建过期缓存: %w", err)
	}
	return &ExpiringCache[T]{cache: cache}, nil
}

// getCacheFilename 生成包含过期时间戳的缓存文件名。
func (c *ExpiringCache[T]) getCacheFilename(id string, expiresAt int64) string {
	return fmt.Sprintf("%s.%d", id, expiresAt)
}

// Read 通过指定的标识符读取缓存数据，如果缓存已过期则返回错误。
// readFn 函数用于处理读取的数据流。
func (c *ExpiringCache[T]) Read(id string, readFn func(io.Reader) error) error {
	pattern := fmt.Sprintf("%s.*", id)
	matches, err := filepath.Glob(filepath.Join(c.cache.dir(), pattern))
	if err != nil {
		return fmt.Errorf("读取过期缓存失败: %w", err)
	}

	if len(matches) == 0 {
		return fmt.Errorf("未找到缓存项")
	}

	filename := filepath.Base(matches[0])
	parts := strings.Split(filename, ".")
	expectedFilenameParts := 2 // 名称和过期时间戳

	if len(parts) != expectedFilenameParts {
		return fmt.Errorf("无效的缓存文件名")
	}

	expiresAt, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return fmt.Errorf("无效的过期时间戳")
	}

	// 检查缓存是否已过期
	if expiresAt < time.Now().Unix() {
		if err := os.Remove(matches[0]); err != nil {
			return fmt.Errorf("删除过期缓存文件失败: %w", err)
		}
		return os.ErrNotExist
	}

	file, err := os.Open(matches[0])
	if err != nil {
		return fmt.Errorf("打开过期缓存文件失败: %w", err)
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			err = cerr
		}
	}()

	return readFn(file)
}

// Write 通过指定的标识符写入缓存数据，并设置过期时间。
// 如果已存在相同标识符的缓存，将先删除旧缓存。
// writeFn 函数用于处理写入的数据流。
func (c *ExpiringCache[T]) Write(id string, expiresAt int64, writeFn func(io.Writer) error) error {
	// 删除相同标识符的旧缓存文件
	pattern := fmt.Sprintf("%s.*", id)
	oldFiles, _ := filepath.Glob(filepath.Join(c.cache.dir(), pattern))
	for _, file := range oldFiles {
		if err := os.Remove(file); err != nil {
			return fmt.Errorf("删除旧缓存文件失败: %w", err)
		}
	}

	filename := c.getCacheFilename(id, expiresAt)
	file, err := os.Create(filepath.Join(c.cache.dir(), filename))
	if err != nil {
		return fmt.Errorf("创建过期缓存文件失败: %w", err)
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			err = cerr
		}
	}()

	return writeFn(file)
}

// Delete 通过标识符删除过期的缓存项。
func (c *ExpiringCache[T]) Delete(id string) error {
	pattern := fmt.Sprintf("%s.*", id)
	matches, err := filepath.Glob(filepath.Join(c.cache.dir(), pattern))
	if err != nil {
		return fmt.Errorf("删除过期缓存失败: %w", err)
	}

	for _, match := range matches {
		if err := os.Remove(match); err != nil {
			return fmt.Errorf("删除过期缓存文件失败: %w", err)
		}
	}

	return nil
}
