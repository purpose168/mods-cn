// Package cache 提供了一个简单的文件缓存实现。
package cache

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Type 表示正在使用的缓存类型。
type Type string

// 不同用途的缓存类型常量。
const (
	ConversationCache Type = "conversations" // 对话缓存
	TemporaryCache    Type = "temp"          // 临时缓存
)

const cacheExt = ".gob" // 缓存文件扩展名

var errInvalidID = errors.New("无效的标识符")

// Cache 是一个泛型缓存实现，将数据存储在文件中。
type Cache[T any] struct {
	baseDir string // 基础目录
	cType   Type   // 缓存类型
}

// New 创建一个新的缓存实例，使用指定的基础目录和缓存类型。
func New[T any](baseDir string, cacheType Type) (*Cache[T], error) {
	dir := filepath.Join(baseDir, string(cacheType))
	if err := os.MkdirAll(dir, os.ModePerm); err != nil { //nolint:gosec
		return nil, fmt.Errorf("创建缓存目录: %w", err)
	}
	return &Cache[T]{
		baseDir: baseDir,
		cType:   cacheType,
	}, nil
}

// dir 返回缓存目录的完整路径。
func (c *Cache[T]) dir() string {
	return filepath.Join(c.baseDir, string(c.cType))
}

// Read 通过指定的标识符读取缓存数据，使用 readFn 函数处理读取的数据流。
func (c *Cache[T]) Read(id string, readFn func(io.Reader) error) error {
	if id == "" {
		return fmt.Errorf("读取: %w", errInvalidID)
	}
	file, err := os.Open(filepath.Join(c.dir(), id+cacheExt))
	if err != nil {
		return fmt.Errorf("读取: %w", err)
	}
	defer file.Close() //nolint:errcheck

	if err := readFn(file); err != nil {
		return fmt.Errorf("读取: %w", err)
	}
	return nil
}

// Write 通过指定的标识符写入缓存数据，使用 writeFn 函数处理写入的数据流。
func (c *Cache[T]) Write(id string, writeFn func(io.Writer) error) error {
	if id == "" {
		return fmt.Errorf("写入: %w", errInvalidID)
	}

	file, err := os.Create(filepath.Join(c.dir(), id+cacheExt))
	if err != nil {
		return fmt.Errorf("写入: %w", err)
	}
	defer file.Close() //nolint:errcheck

	if err := writeFn(file); err != nil {
		return fmt.Errorf("写入: %w", err)
	}

	return nil
}

// Delete 通过标识符删除缓存的条目。
func (c *Cache[T]) Delete(id string) error {
	if id == "" {
		return fmt.Errorf("删除: %w", errInvalidID)
	}
	if err := os.Remove(filepath.Join(c.dir(), id+cacheExt)); err != nil {
		return fmt.Errorf("删除: %w", err)
	}
	return nil
}
