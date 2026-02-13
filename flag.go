package main

import (
	"regexp"
	"strings"
	"time"

	"github.com/caarlos0/duration"
)

// newFlagParseError 创建标志解析错误
// err: 原始错误
// 返回：标志解析错误
func newFlagParseError(err error) flagParseError {
	var reason, flag string
	s := err.Error()
	switch {
	case strings.HasPrefix(s, "flag needs an argument:"):
		reason = "标志 %s 需要参数。"
		ps := strings.Split(s, "-")
		switch len(ps) {
		case 2: //nolint:mnd
			flag = "-" + ps[len(ps)-1]
		case 3: //nolint:mnd
			flag = "--" + ps[len(ps)-1]
		}
	case strings.HasPrefix(s, "unknown flag:"):
		reason = "标志 %s 不存在。"
		flag = strings.TrimPrefix(s, "unknown flag: ")
	case strings.HasPrefix(s, "unknown shorthand flag:"):
		reason = "短标志 %s 不存在。"
		re := regexp.MustCompile(`unknown shorthand flag: '.*' in (-\w)`)
		parts := re.FindStringSubmatch(s)
		if len(parts) > 1 {
			flag = parts[1]
		}
	case strings.HasPrefix(s, "invalid argument"):
		reason = "标志 %s 的参数无效。"
		re := regexp.MustCompile(`invalid argument ".*" for "(.*)" flag: .*`)
		parts := re.FindStringSubmatch(s)
		if len(parts) > 1 {
			flag = parts[1]
		}
	default:
		reason = s
	}
	return flagParseError{
		err:    err,
		reason: reason,
		flag:   flag,
	}
}

// flagParseError 标志解析错误
type flagParseError struct {
	err    error  // 原始错误
	reason string // 原因
	flag   string // 标志名称
}

// Error 返回错误消息
func (f flagParseError) Error() string {
	return f.err.Error()
}

// ReasonFormat 返回原因格式字符串
func (f flagParseError) ReasonFormat() string {
	return f.reason
}

// Flag 返回标志名称
func (f flagParseError) Flag() string {
	return f.flag
}

// newDurationFlag 创建持续时间标志
// val: 默认值
// p: 指向持续时间变量的指针
// 返回：持续时间标志
func newDurationFlag(val time.Duration, p *time.Duration) *durationFlag {
	*p = val
	return (*durationFlag)(p)
}

// durationFlag 持续时间标志类型
type durationFlag time.Duration

// Set 设置标志值
// s: 字符串值
// 返回：错误信息
func (d *durationFlag) Set(s string) error {
	v, err := duration.Parse(s)
	*d = durationFlag(v)
	//nolint: wrapcheck
	return err
}

// String 返回字符串表示
func (d *durationFlag) String() string {
	return time.Duration(*d).String()
}

// Type 返回类型名称
func (*durationFlag) Type() string {
	return "duration"
}
