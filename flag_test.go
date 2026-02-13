package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// flagParseErrorTests 标志解析错误测试用例
var flagParseErrorTests = []struct {
	in     string // 输入字符串
	flag   string // 标志名称
	reason string // 原因
}{
	{
		"unknown flag: --nope",
		"--nope",
		"标志 %s 不存在。",
	},
	{
		"flag needs an argument: --delete",
		"--delete",
		"标志 %s 需要参数。",
	},
	{
		"flag needs an argument: 'd' in -d",
		"-d",
		"标志 %s 需要参数。",
	},
	{
		`invalid argument "20dd" for "--delete-older-than" flag: time: unknown unit "dd" in duration "20dd"`,
		"--delete-older-than",
		"标志 %s 的参数无效。",
	},
	{
		`invalid argument "sdfjasdl" for "--max-tokens" flag: strconv.ParseInt: parsing "sdfjasdl": invalid syntax`,
		"--max-tokens",
		"标志 %s 的参数无效。",
	},
	{
		`invalid argument "nope" for "-r, --raw" flag: strconv.ParseBool: parsing "nope": invalid syntax`,
		"-r, --raw",
		"标志 %s 的参数无效。",
	},
}

// TestFlagParseError 测试标志解析错误
func TestFlagParseError(t *testing.T) {
	for _, tf := range flagParseErrorTests {
		t.Run(tf.in, func(t *testing.T) {
			err := newFlagParseError(errors.New(tf.in))
			require.Equal(t, tf.flag, err.Flag())
			require.Equal(t, tf.reason, err.ReasonFormat())
			require.Equal(t, tf.in, err.Error())
		})
	}
}
