package main

import "fmt"

// newUserErrorf 创建面向用户的错误。
// 此函数主要是为了避免代码检查工具抱怨错误以大写字母开头。
func newUserErrorf(format string, a ...any) error {
	return fmt.Errorf(format, a...)
}

// modsError 是错误的包装器，用于添加额外的上下文信息。
type modsError struct {
	err    error  // 原始错误
	reason string // 原因说明
}

// Error 返回错误消息
func (m modsError) Error() string {
	return m.err.Error()
}

// Reason 返回错误原因
func (m modsError) Reason() string {
	return m.reason
}
