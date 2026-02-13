package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
	"time"

	_ "embed"

	"github.com/adrg/xdg"
	"github.com/caarlos0/duration"
	"github.com/caarlos0/env/v9"
	"github.com/charmbracelet/x/exp/strings"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

//go:embed config_template.yml
var configTemplate string

const (
	defaultMarkdownFormatText = "将响应格式化为 markdown，不包含包围的反引号。"
	defaultJSONFormatText     = "将响应格式化为 json，不包含包围的反引号。"
)

var help = map[string]string{
	"api":               "OpenAI 兼容的 REST API（openai、localai、anthropic 等）",
	"apis":              "OpenAI 兼容 REST API 的别名和端点",
	"http-proxy":        "用于 API 请求的 HTTP 代理",
	"model":             "默认模型（gpt-3.5-turbo、gpt-4、ggml-gpt4all-j...）",
	"ask-model":         "通过交互式提示询问使用哪个模型",
	"max-input-chars":   "模型输入的默认字符限制",
	"format":            "要求将响应格式化为 markdown，除非另有设置",
	"format-text":       "使用 -f 标志时要追加的文本",
	"role":              "要使用的系统角色",
	"roles":             "可用作角色的预定义系统消息列表",
	"list-roles":        "列出配置文件中定义的角色",
	"prompt":            "在响应中包含来自参数和 stdin 的提示，将 stdin 截断为指定行数",
	"prompt-args":       "在响应中包含来自参数的提示",
	"raw":               "连接到 TTY 时将输出渲染为原始文本",
	"quiet":             "安静模式（加载时隐藏旋转器，成功时隐藏 stderr 消息）",
	"help":              "显示帮助并退出",
	"version":           "显示版本并退出",
	"max-retries":       "重试 API 调用的最大次数",
	"no-limit":          "关闭客户端对模型输入大小的限制",
	"word-wrap":         "以特定宽度换行格式化输出（默认为 80）",
	"max-tokens":        "响应中的最大令牌数",
	"temp":              "结果的温度（随机性），从 0.0 到 2.0，-1.0 表示禁用",
	"stop":              "最多 4 个序列，API 将在这些序列处停止生成更多令牌",
	"topp":              "TopP，温度的替代方案，用于缩小响应范围，从 0.0 到 1.0，-1.0 表示禁用",
	"topk":              "TopK，仅从每个后续令牌的前 K 个选项中采样，-1 表示禁用",
	"fanciness":         "您期望的花哨程度",
	"status-text":       "生成时显示的文本",
	"settings":          "在 $EDITOR 中打开设置",
	"dirs":              "打印 mods 存储其数据的目录",
	"reset-settings":    "备份旧设置文件并将所有内容重置为默认值",
	"continue":          "从上次响应或给定的保存标题继续",
	"continue-last":     "从上次响应继续",
	"no-cache":          "禁用提示/响应的缓存",
	"title":             "以给定标题保存当前对话",
	"list":              "列出已保存的对话",
	"delete":            "删除具有给定标题或 ID 的一个或多个已保存对话",
	"delete-older-than": "删除所有早于指定持续时间的已保存对话；有效值为 " + strings.EnglishJoin(duration.ValidUnits(), true),
	"show":              "显示具有给定标题或 ID 的已保存对话",
	"theme":             "在表单中使用的主题；有效选择为 charm、catppuccin、dracula 和 base16",
	"show-last":         "显示上次保存的对话",
	"editor":            "在 $EDITOR 中编辑提示；仅在没有其他参数且 STDIN 是 TTY 时才生效",
	"mcp-servers":       "MCP 服务器配置",
	"mcp-disable":       "禁用特定的 MCP 服务器",
	"mcp-list":          "列出所有可用的 MCP 服务器",
	"mcp-list-tools":    "列出已启用 MCP 服务器的所有可用工具",
	"mcp-timeout":       "MCP 服务器调用的超时时间，默认为 15 秒",
}

// Model 表示 API 调用中使用的 LLM 模型。
type Model struct {
	Name           string   // 模型名称
	API            string   // API 名称
	MaxChars       int64    `yaml:"max-input-chars"` // 最大输入字符数
	Aliases        []string `yaml:"aliases"`         // 别名列表
	Fallback       string   `yaml:"fallback"`        // 回退模型
	ThinkingBudget int      `yaml:"thinking-budget,omitempty"` // 思考预算
}

// API 表示 API 端点及其模型。
type API struct {
	Name      string           // API 名称
	APIKey    string           `yaml:"api-key"`     // API 密钥
	APIKeyEnv string           `yaml:"api-key-env"` // API 密钥环境变量
	APIKeyCmd string           `yaml:"api-key-cmd"` // API 密钥命令
	Version   string           `yaml:"version"`     // 版本（XXX: 未在任何地方使用）
	BaseURL   string           `yaml:"base-url"`    // 基础 URL
	Models    map[string]Model `yaml:"models"`      // 模型映射
	User      string           `yaml:"user"`        // 用户
}

// APIs 是类型别名，用于自定义 YAML 解码。
type APIs []API

// UnmarshalYAML 实现排序的 API YAML 解码。
func (apis *APIs) UnmarshalYAML(node *yaml.Node) error {
	for i := 0; i < len(node.Content); i += 2 {
		var api API
		if err := node.Content[i+1].Decode(&api); err != nil {
			return fmt.Errorf("解码 YAML 文件时出错: %s", err)
		}
		api.Name = node.Content[i].Value
		*apis = append(*apis, api)
	}
	return nil
}

// FormatText 是 map[format]formatting_text 类型。
type FormatText map[string]string

// UnmarshalYAML 符合 yaml.Unmarshaler 接口。
func (ft *FormatText) UnmarshalYAML(unmarshal func(any) error) error {
	var text string
	if err := unmarshal(&text); err != nil {
		var formats map[string]string
		if err := unmarshal(&formats); err != nil {
			return err
		}
		*ft = (FormatText)(formats)
		return nil
	}

	*ft = map[string]string{
		"markdown": text,
	}
	return nil
}

// Config 保存主配置，映射到 YAML 设置文件。
type Config struct {
	API                 string     `yaml:"default-api" env:"API"`                         // 默认 API
	Model               string     `yaml:"default-model" env:"MODEL"`                     // 默认模型
	Format              bool       `yaml:"format" env:"FORMAT"`                           // 格式化
	FormatText          FormatText `yaml:"format-text"`                                   // 格式化文本
	FormatAs            string     `yaml:"format-as" env:"FORMAT_AS"`                     // 格式化为
	Raw                 bool       `yaml:"raw" env:"RAW"`                                 // 原始输出
	Quiet               bool       `yaml:"quiet" env:"QUIET"`                             // 安静模式
	MaxTokens           int64      `yaml:"max-tokens" env:"MAX_TOKENS"`                   // 最大令牌数
	MaxCompletionTokens int64      `yaml:"max-completion-tokens" env:"MAX_COMPLETION_TOKENS"` // 最大完成令牌数
	MaxInputChars       int64      `yaml:"max-input-chars" env:"MAX_INPUT_CHARS"`         // 最大输入字符数
	Temperature         float64    `yaml:"temp" env:"TEMP"`                               // 温度
	Stop                []string   `yaml:"stop" env:"STOP"`                               // 停止序列
	TopP                float64    `yaml:"topp" env:"TOPP"`                               // TopP
	TopK                int64      `yaml:"topk" env:"TOPK"`                               // TopK
	NoLimit             bool       `yaml:"no-limit" env:"NO_LIMIT"`                       // 无限制
	CachePath           string     `yaml:"cache-path" env:"CACHE_PATH"`                   // 缓存路径
	NoCache             bool       `yaml:"no-cache" env:"NO_CACHE"`                       // 禁用缓存
	IncludePromptArgs   bool       `yaml:"include-prompt-args" env:"INCLUDE_PROMPT_ARGS"` // 包含提示参数
	IncludePrompt       int        `yaml:"include-prompt" env:"INCLUDE_PROMPT"`           // 包含提示
	MaxRetries          int        `yaml:"max-retries" env:"MAX_RETRIES"`                 // 最大重试次数
	WordWrap            int        `yaml:"word-wrap" env:"WORD_WRAP"`                     // 自动换行
	Fanciness           uint       `yaml:"fanciness" env:"FANCINESS"`                     // 花哨程度
	StatusText          string     `yaml:"status-text" env:"STATUS_TEXT"`                 // 状态文本
	HTTPProxy           string     `yaml:"http-proxy" env:"HTTP_PROXY"`                   // HTTP 代理
	APIs                APIs       `yaml:"apis"`                                          // API 列表
	System              string     `yaml:"system"`                                        // 系统消息
	Role                string     `yaml:"role" env:"ROLE"`                               // 角色
	AskModel            bool                                                          // 询问模型
	Roles               map[string][]string                                           // 角色映射
	ShowHelp            bool                                                          // 显示帮助
	ResetSettings       bool                                                          // 重置设置
	Prefix              string                                                        // 前缀
	Version             bool                                                          // 版本
	Settings            bool                                                          // 设置
	Dirs                bool                                                          // 目录
	Theme               string                                                        // 主题
	SettingsPath        string                                                        // 设置路径
	ContinueLast        bool                                                          // 继续上次
	Continue            string                                                        // 继续
	Title               string                                                        // 标题
	ShowLast            bool                                                          // 显示上次
	Show                string                                                        // 显示
	List                bool                                                          // 列表
	ListRoles           bool                                                          // 列出角色
	Delete              []string                                                      // 删除
	DeleteOlderThan     time.Duration                                                 // 删除早于
	User                string                                                        // 用户

	MCPServers   map[string]MCPServerConfig `yaml:"mcp-servers"` // MCP 服务器配置
	MCPList      bool                                          // MCP 列表
	MCPListTools bool                                          // MCP 工具列表
	MCPDisable   []string                                      // MCP 禁用
	MCPTimeout   time.Duration `yaml:"mcp-timeout" env:"MCP_TIMEOUT"` // MCP 超时

	openEditor                                         bool   // 打开编辑器
	cacheReadFromID, cacheWriteToID, cacheWriteToTitle string // 缓存相关
}

// MCPServerConfig 保存 MCP 服务器的配置。
type MCPServerConfig struct {
	Type    string   `yaml:"type"`    // 类型
	Command string   `yaml:"command"` // 命令
	Env     []string `yaml:"env"`     // 环境变量
	Args    []string `yaml:"args"`    // 参数
	URL     string   `yaml:"url"`     // URL
}

// ensureConfig 确保配置文件存在并返回配置
func ensureConfig() (Config, error) {
	var c Config
	sp, err := xdg.ConfigFile(filepath.Join("mods", "mods.yml"))
	if err != nil {
		return c, modsError{err, "无法找到设置路径。"}
	}
	c.SettingsPath = sp

	dir := filepath.Dir(sp)
	if dirErr := os.MkdirAll(dir, 0o700); dirErr != nil { //nolint:mnd
		return c, modsError{dirErr, "无法创建缓存目录。"}
	}

	if dirErr := writeConfigFile(sp); dirErr != nil {
		return c, dirErr
	}
	content, err := os.ReadFile(sp)
	if err != nil {
		return c, modsError{err, "无法读取设置文件。"}
	}
	if err := yaml.Unmarshal(content, &c); err != nil {
		return c, modsError{err, "无法解析设置文件。"}
	}

	if err := env.ParseWithOptions(&c, env.Options{Prefix: "MODS_"}); err != nil {
		return c, modsError{err, "无法将环境变量解析到设置文件。"}
	}

	if c.CachePath == "" {
		c.CachePath = filepath.Join(xdg.DataHome, "mods")
	}

	if err := os.MkdirAll(
		filepath.Join(c.CachePath, "conversations"),
		0o700,
	); err != nil { //nolint:mnd
		return c, modsError{err, "无法创建缓存目录。"}
	}

	if c.WordWrap == 0 {
		c.WordWrap = 80
	}

	return c, nil
}

// writeConfigFile 写入配置文件
func writeConfigFile(path string) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return createConfigFile(path)
	} else if err != nil {
		return modsError{err, "无法获取路径状态。"}
	}
	return nil
}

// createConfigFile 创建配置文件
func createConfigFile(path string) error {
	tmpl := template.Must(template.New("config").Parse(configTemplate))

	f, err := os.Create(path)
	if err != nil {
		return modsError{err, "无法创建配置文件。"}
	}
	defer func() { _ = f.Close() }()

	m := struct {
		Config Config
		Help   map[string]string
	}{
		Config: defaultConfig(),
		Help:   help,
	}
	if err := tmpl.Execute(f, m); err != nil {
		return modsError{err, "无法渲染模板。"}
	}
	return nil
}

// defaultConfig 返回默认配置
func defaultConfig() Config {
	return Config{
		FormatAs: "markdown",
		FormatText: FormatText{
			"markdown": defaultMarkdownFormatText,
			"json":     defaultJSONFormatText,
		},
		MCPTimeout: 15 * time.Second,
	}
}

// useLine 返回使用行文本
func useLine() string {
	appName := filepath.Base(os.Args[0])

	if stdoutRenderer().ColorProfile() == termenv.TrueColor {
		appName = makeGradientText(stdoutStyles().AppName, appName)
	}

	return fmt.Sprintf(
		"%s %s",
		appName,
		stdoutStyles().CliArgs.Render("[选项] [前缀 词项]"),
	)
}

// usageFunc 返回使用函数
func usageFunc(cmd *cobra.Command) error {
	fmt.Printf(
		"用法:\n  %s\n\n",
		useLine(),
	)
	fmt.Println("选项:")
	cmd.Flags().VisitAll(func(f *flag.Flag) {
		if f.Hidden {
			return
		}
		if f.Shorthand == "" {
			fmt.Printf(
				"  %-44s %s\n",
				stdoutStyles().Flag.Render("--"+f.Name),
				stdoutStyles().FlagDesc.Render(f.Usage),
			)
		} else {
			fmt.Printf(
				"  %s%s %-40s %s\n",
				stdoutStyles().Flag.Render("-"+f.Shorthand),
				stdoutStyles().FlagComma,
				stdoutStyles().Flag.Render("--"+f.Name),
				stdoutStyles().FlagDesc.Render(f.Usage),
			)
		}
	})
	if cmd.HasExample() {
		fmt.Printf(
			"\n示例:\n  %s\n  %s\n",
			stdoutStyles().Comment.Render("# "+cmd.Example),
			cheapHighlighting(stdoutStyles(), examples[cmd.Example]),
		)
	}

	return nil
}
