package main

import (
	"crypto/rand"
	"crypto/sha1" //nolint: gosec
	"fmt"
	"regexp"
)

const (
	sha1short         = 7    // SHA1 短格式长度
	sha1minLen        = 4    // SHA1 最小长度
	sha1ReadBlockSize = 4096 // SHA1 读取块大小
)

var sha1reg = regexp.MustCompile(`\b[0-9a-f]{40}\b`)

// newConversationID 生成新的对话 ID
// 返回：SHA1 格式的对话 ID
func newConversationID() string {
	b := make([]byte, sha1ReadBlockSize)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", sha1.Sum(b)) //nolint: gosec
}
