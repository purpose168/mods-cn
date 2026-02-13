# API 接口文档

## 文档信息

- **文档版本**: v1.0
- **创建日期**: 2026-02-13
- **最后更新**: 2026-02-13
- **维护人员**: purpose168@outlook.com

---

## 1. 概述

本文档详细描述了 Mods 项目的内部 API 接口定义，包括核心接口、数据结构、错误码定义等。Mods 作为一个命令行工具，主要通过内部模块间的接口进行通信，同时也与外部 AI 服务 API 进行交互。

---

## 2. 核心 API 接口

### 2.1 流客户端接口 (stream.Client)

**定义位置**: [internal/stream/stream.go](../../internal/stream/stream.go)

**接口说明**: 定义了所有 AI 服务提供商必须实现的客户端接口。

```go
// Client 流客户端接口
type Client interface {
    // Request 发起请求并返回流
    // ctx: 上下文，用于控制请求生命周期
    // req: 统一的请求结构
    // 返回: 流式响应对象
    Request(ctx context.Context, req proto.Request) Stream
}
```

**实现类**:
- `openai.Client` - OpenAI 客户端
- `anthropic.Client` - Anthropic 客户端
- `cohere.Client` - Cohere 客户端
- `google.Client` - Google 客户端
- `ollama.Client` - Ollama 客户端

### 2.2 流响应接口 (stream.Stream)

**定义位置**: [internal/stream/stream.go](../../internal/stream/stream.go)

**接口说明**: 定义了流式响应的标准接口。

```go
// Stream 流接口
type Stream interface {
    // Next 推进到下一个数据块
    // 返回: 是否还有更多数据
    Next() bool
    
    // Current 获取当前数据块
    // 返回: 内容块和可能的错误
    Current() (proto.Chunk, error)
    
    // Err 返回流中的错误
    Err() error
    
    // Close 关闭流
    Close() error
    
    // Messages 返回消息列表
    Messages() []proto.Message
    
    // CallTools 调用工具
    // 返回: 工具调用状态列表
    CallTools() []proto.ToolCallStatus
}
```

**使用示例**:

```go
// 遍历流式响应
for stream.Next() {
    chunk, err := stream.Current()
    if err != nil {
        // 处理错误
        break
    }
    // 处理内容块
    fmt.Print(chunk.Content)
}

// 检查流结束后的错误
if err := stream.Err(); err != nil {
    // 处理错误
}
```

---

## 3. 数据结构定义

### 3.1 消息结构 (proto.Message)

**定义位置**: [internal/proto/proto.go](../../internal/proto/proto.go)

```go
// Message 消息结构
type Message struct {
    Role    Role   `json:"role"`    // 消息角色
    Content string `json:"content"` // 消息内容
}

// Role 消息角色类型
type Role string

const (
    RoleSystem    Role = "system"    // 系统消息
    RoleUser      Role = "user"      // 用户消息
    RoleAssistant Role = "assistant" // 助手消息
)
```

**字段说明**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| Role | Role | 是 | 消息角色，可选值：system, user, assistant |
| Content | string | 是 | 消息内容文本 |

### 3.2 请求结构 (proto.Request)

**定义位置**: [internal/proto/proto.go](../../internal/proto/proto.go)

```go
// Request 统一请求结构
type Request struct {
    Messages    []Message         // 消息列表
    Model       string            // 模型名称
    User        string            // 用户标识
    Temperature *float64          // 温度参数 (0.0-2.0)
    TopP        *float64          // Top-P 参数 (0.0-1.0)
    TopK        *int64            // Top-K 参数
    MaxTokens   *int64            // 最大令牌数
    Stop        []string          // 停止序列
    Tools       map[string][]mcp.Tool // MCP 工具
    ToolCaller  func(name string, data []byte) (string, error) // 工具调用函数
    API         string            // API 类型标识
    ResponseFormat *string        // 响应格式 (json)
}
```

**字段说明**:

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| Messages | []Message | 是 | 对话消息列表 |
| Model | string | 是 | 模型标识符 |
| User | string | 否 | 用户唯一标识 |
| Temperature | *float64 | 否 | 采样温度，控制随机性 |
| TopP | *float64 | 否 | 核采样参数 |
| TopK | *int64 | 否 | Top-K 采样参数 |
| MaxTokens | *int64 | 否 | 响应最大令牌数 |
| Stop | []string | 否 | 停止生成的序列 |
| Tools | map | 否 | MCP 工具映射 |
| ToolCaller | func | 否 | 工具调用回调函数 |
| API | string | 否 | API 提供商标识 |
| ResponseFormat | *string | 否 | 响应格式类型 |

### 3.3 响应块结构 (proto.Chunk)

**定义位置**: [internal/proto/proto.go](../../internal/proto/proto.go)

```go
// Chunk 响应内容块
type Chunk struct {
    Content string // 文本内容
}
```

### 3.4 工具调用状态 (proto.ToolCallStatus)

**定义位置**: [internal/proto/proto.go](../../internal/proto/proto.go)

```go
// ToolCallStatus 工具调用状态
type ToolCallStatus struct {
    ID     string // 工具调用 ID
    Name   string // 工具名称
    Status string // 调用状态
    Err    error  // 错误信息
}
```

---

## 4. 配置 API

### 4.1 配置结构 (Config)

**定义位置**: [config.go](../../config.go)

```go
// Config 主配置结构
type Config struct {
    // 基础配置
    API                 string     `yaml:"default-api" env:"API"`
    Model               string     `yaml:"default-model" env:"MODEL"`
    Format              bool       `yaml:"format" env:"FORMAT"`
    FormatText          FormatText `yaml:"format-text"`
    FormatAs            string     `yaml:"format-as" env:"FORMAT_AS"`
    Raw                 bool       `yaml:"raw" env:"RAW"`
    Quiet               bool       `yaml:"quiet" env:"QUIET"`
    
    // 模型参数
    MaxTokens           int64      `yaml:"max-tokens" env:"MAX_TOKENS"`
    MaxCompletionTokens int64      `yaml:"max-completion-tokens"`
    MaxInputChars       int64      `yaml:"max-input-chars" env:"MAX_INPUT_CHARS"`
    Temperature         float64    `yaml:"temp" env:"TEMP"`
    Stop                []string   `yaml:"stop" env:"STOP"`
    TopP                float64    `yaml:"topp" env:"TOPP"`
    TopK                int64      `yaml:"topk" env:"TOPK"`
    NoLimit             bool       `yaml:"no-limit" env:"NO_LIMIT"`
    
    // 缓存配置
    CachePath           string     `yaml:"cache-path" env:"CACHE_PATH"`
    NoCache             bool       `yaml:"no-cache" env:"NO_CACHE"`
    
    // 提示配置
    IncludePromptArgs   bool       `yaml:"include-prompt-args"`
    IncludePrompt       int        `yaml:"include-prompt"`
    
    // 重试配置
    MaxRetries          int        `yaml:"max-retries" env:"MAX_RETRIES"`
    
    // 显示配置
    WordWrap            int        `yaml:"word-wrap" env:"WORD_WRAP"`
    Fanciness           uint       `yaml:"fanciness" env:"FANCINESS"`
    StatusText          string     `yaml:"status-text" env:"STATUS_TEXT"`
    
    // 网络配置
    HTTPProxy           string     `yaml:"http-proxy" env:"HTTP_PROXY"`
    
    // API 配置
    APIs                APIs       `yaml:"apis"`
    
    // 角色配置
    System              string     `yaml:"system"`
    Role                string     `yaml:"role" env:"ROLE"`
    Roles               map[string][]string
    
    // MCP 配置
    MCPServers   map[string]MCPServerConfig `yaml:"mcp-servers"`
    MCPTimeout   time.Duration `yaml:"mcp-timeout" env:"MCP_TIMEOUT"`
}
```

### 4.2 API 配置结构 (API)

```go
// API API 端点配置
type API struct {
    Name      string           `yaml:"-"`       // API 名称
    APIKey    string           `yaml:"api-key"` // API 密钥
    APIKeyEnv string           `yaml:"api-key-env"` // 密钥环境变量
    APIKeyCmd string           `yaml:"api-key-cmd"` // 密钥获取命令
    Version   string           `yaml:"version"` // API 版本
    BaseURL   string           `yaml:"base-url"` // 基础 URL
    Models    map[string]Model `yaml:"models"`  // 模型映射
    User      string           `yaml:"user"`    // 用户标识
}
```

### 4.3 模型配置结构 (Model)

```go
// Model 模型配置
type Model struct {
    Name           string   `yaml:"-"`              // 模型名称
    API            string   `yaml:"-"`              // API 名称
    MaxChars       int64    `yaml:"max-input-chars"` // 最大输入字符
    Aliases        []string `yaml:"aliases"`        // 别名列表
    Fallback       string   `yaml:"fallback"`       // 回退模型
    ThinkingBudget int      `yaml:"thinking-budget,omitempty"` // 思考预算
}
```

### 4.4 MCP 服务器配置 (MCPServerConfig)

```go
// MCPServerConfig MCP 服务器配置
type MCPServerConfig struct {
    Type    string   `yaml:"type"`    // 类型: stdio, sse, http
    Command string   `yaml:"command"` // 命令 (stdio)
    Env     []string `yaml:"env"`     // 环境变量
    Args    []string `yaml:"args"`    // 参数
    URL     string   `yaml:"url"`     // URL (sse/http)
}
```

---

## 5. 数据库 API

### 5.1 数据库接口 (convoDB)

**定义位置**: [db.go](../../db.go)

```go
// convoDB 对话数据库
type convoDB struct {
    db *sqlx.DB
}
```

### 5.2 数据库方法

#### Save - 保存对话

```go
// Save 保存或更新对话记录
// id: 对话 ID (SHA-1)
// title: 对话标题
// api: API 名称
// model: 模型名称
// 返回: 错误信息
func (c *convoDB) Save(id, title, api, model string) error
```

#### Delete - 删除对话

```go
// Delete 删除对话记录
// id: 对话 ID
// 返回: 错误信息
func (c *convoDB) Delete(id string) error
```

#### Find - 查找对话

```go
// Find 查找对话
// in: ID 或标题
// 返回: 对话记录和错误信息
func (c *convoDB) Find(in string) (*Conversation, error)
```

#### List - 列出对话

```go
// List 列出所有对话
// 返回: 对话列表和错误信息
func (c *convoDB) List() ([]Conversation, error)
```

#### FindHEAD - 查找最新对话

```go
// FindHEAD 查找最新的对话
// 返回: 对话记录和错误信息
func (c *convoDB) FindHEAD() (*Conversation, error)
```

#### Completions - 获取补全列表

```go
// Completions 获取自动补全列表
// in: 输入前缀
// 返回: 补全列表和错误信息
func (c *convoDB) Completions(in string) ([]string, error)
```

### 5.3 对话记录结构 (Conversation)

```go
// Conversation 对话记录
type Conversation struct {
    ID        string    `db:"id"`         // 对话 ID
    Title     string    `db:"title"`      // 对话标题
    UpdatedAt time.Time `db:"updated_at"` // 更新时间
    API       *string   `db:"api"`        // API 名称
    Model     *string   `db:"model"`      // 模型名称
}
```

---

## 6. 缓存 API

### 6.1 缓存接口 (Cache)

**定义位置**: [internal/cache/cache.go](../../internal/cache/cache.go)

```go
// Cache 泛型缓存接口
type Cache[T any] struct {
    baseDir string
    cType   Type
}
```

### 6.2 缓存方法

#### New - 创建缓存实例

```go
// New 创建缓存实例
// baseDir: 基础目录
// cacheType: 缓存类型
// 返回: 缓存实例和错误
func New[T any](baseDir string, cacheType Type) (*Cache[T], error)
```

#### Read - 读取缓存

```go
// Read 读取缓存数据
// id: 缓存标识
// readFn: 读取处理函数
// 返回: 错误信息
func (c *Cache[T]) Read(id string, readFn func(io.Reader) error) error
```

#### Write - 写入缓存

```go
// Write 写入缓存数据
// id: 缓存标识
// writeFn: 写入处理函数
// 返回: 错误信息
func (c *Cache[T]) Write(id string, writeFn func(io.Writer) error) error
```

#### Delete - 删除缓存

```go
// Delete 删除缓存条目
// id: 缓存标识
// 返回: 错误信息
func (c *Cache[T]) Delete(id string) error
```

### 6.3 对话缓存 (Conversations)

```go
// Conversations 对话缓存
type Conversations struct {
    *Cache[[]proto.Message]
}

// Read 读取对话消息
func (c *Conversations) Read(id string, messages *[]proto.Message) error

// Write 写入对话消息
func (c *Conversations) Write(id string, messages *[]proto.Message) error
```

---

## 7. MCP API

### 7.1 MCP 工具获取

**定义位置**: [mcp.go](../../mcp.go)

```go
// mcpTools 获取所有 MCP 工具
// ctx: 上下文
// 返回: 工具映射和错误信息
func mcpTools(ctx context.Context) (map[string][]mcp.Tool, error)
```

### 7.2 工具调用

```go
// toolCall 调用 MCP 工具
// ctx: 上下文
// name: 工具名称 (格式: server_tool)
// data: 工具参数 JSON 数据
// 返回: 工具执行结果和错误信息
func toolCall(ctx context.Context, name string, data []byte) (string, error)
```

---

## 8. 命令行接口 (CLI)

### 8.1 全局选项

| 选项 | 短选项 | 类型 | 默认值 | 说明 |
|------|--------|------|--------|------|
| `--model` | `-m` | string | gpt-4o | 指定使用的模型 |
| `--ask-model` | `-M` | bool | false | 交互式选择模型 |
| `--api` | `-a` | string | openai | 指定 API 端点 |
| `--format` | `-f` | bool | false | 格式化输出为 Markdown |
| `--format-as` | | string | markdown | 指定输出格式 |
| `--raw` | `-r` | bool | false | 原始文本输出 |
| `--quiet` | `-q` | bool | false | 安静模式 |
| `--max-tokens` | | int | 0 | 最大响应令牌数 |
| `--temp` | | float | 1.0 | 采样温度 |
| `--topp` | | float | 1.0 | Top-P 参数 |
| `--topk` | | int | 50 | Top-K 参数 |
| `--word-wrap` | | int | 80 | 自动换行宽度 |
| `--role` | `-R` | string | default | 使用角色 |
| `--theme` | | string | charm | UI 主题 |

### 8.2 对话管理选项

| 选项 | 短选项 | 说明 |
|------|--------|------|
| `--title` | `-t` | 设置对话标题 |
| `--list` | `-l` | 列出保存的对话 |
| `--continue` | `-c` | 继续指定对话 |
| `--continue-last` | `-C` | 继续上次对话 |
| `--show` | `-s` | 显示指定对话 |
| `--show-last` | `-S` | 显示上次对话 |
| `--delete` | `-d` | 删除指定对话 |
| `--delete-older-than` | | 删除早于指定时间的对话 |
| `--no-cache` | | 禁用对话缓存 |

### 8.3 MCP 选项

| 选项 | 说明 |
|------|------|
| `--mcp-list` | 列出所有 MCP 服务器 |
| `--mcp-list-tools` | 列出所有 MCP 工具 |
| `--mcp-disable` | 禁用指定 MCP 服务器 |

### 8.4 其他选项

| 选项 | 短选项 | 说明 |
|------|--------|------|
| `--settings` | | 打开配置文件编辑 |
| `--dirs` | | 显示数据目录 |
| `--reset-settings` | | 重置配置为默认值 |
| `--help` | `-h` | 显示帮助 |
| `--version` | `-v` | 显示版本 |

---

## 9. 错误码定义

### 9.1 错误类型

```go
// modsError 应用程序错误
type modsError struct {
    err    error  // 原始错误
    reason string // 错误原因描述
}

// flagParseError 标志解析错误
type flagParseError struct {
    err   error
    flag  string
    usage string
}
```

### 9.2 常见错误场景

| 错误类型 | 原因 | 解决方案 |
|----------|------|----------|
| API 密钥缺失 | 未设置 API 密钥 | 设置环境变量或配置文件 |
| 模型不存在 | 指定的模型未配置 | 检查配置文件中的模型定义 |
| API 端点未配置 | 指定的 API 未定义 | 检查配置文件中的 API 定义 |
| 对话未找到 | 指定的对话 ID 不存在 | 使用 `--list` 查看可用对话 |
| 网络错误 | 无法连接到 API | 检查网络连接或代理设置 |
| 上下文长度超限 | 输入超过模型限制 | 减少输入长度或使用更大上下文的模型 |

### 9.3 错误处理示例

```go
// 错误处理
var merr modsError
if errors.As(err, &merr) {
    fmt.Printf("错误: %s\n", merr.reason)
    fmt.Printf("详情: %v\n", merr.err)
}
```

---

## 10. 环境变量

### 10.1 支持的环境变量

| 环境变量 | 说明 | 示例 |
|----------|------|------|
| `MODS_API` | 默认 API | `openai` |
| `MODS_MODEL` | 默认模型 | `gpt-4o` |
| `MODS_FORMAT` | 格式化输出 | `true` |
| `MODS_TEMP` | 温度参数 | `1.0` |
| `MODS_QUIET` | 安静模式 | `true` |
| `OPENAI_API_KEY` | OpenAI 密钥 | `sk-...` |
| `ANTHROPIC_API_KEY` | Anthropic 密钥 | `sk-ant-...` |
| `COHERE_API_KEY` | Cohere 密钥 | `...` |
| `GOOGLE_API_KEY` | Google 密钥 | `...` |
| `GROQ_API_KEY` | Groq 密钥 | `...` |
| `AZURE_OPENAI_KEY` | Azure 密钥 | `...` |

---

## 11. 使用示例

### 11.1 基本使用

```bash
# 简单问答
mods "什么是 Go 语言？"

# 使用管道
echo "解释这段代码" | cat main.go - | mods

# 指定模型
mods -m claude-sonnet-4 "写一个排序算法"

# 格式化输出
mods -f "列出 5 种编程语言"

# JSON 格式输出
mods -f --format-as json "生成用户数据"
```

### 11.2 对话管理

```bash
# 保存对话
mods -t "代码审查" "审查这段代码..."

# 继续对话
mods -c "代码审查" "请详细解释第三点"

# 列出对话
mods -l

# 显示对话
mods -s "代码审查"

# 删除对话
mods -d "代码审查"
```

### 11.3 高级用法

```bash
# 使用角色
mods --role shell "列出当前目录文件"

# 使用 MCP 工具
mods --mcp-list
mods --mcp-list-tools

# 设置温度
mods --temp 0.5 "创意写作"

# 设置最大令牌
mods --max-tokens 100 "简短回答"
```

---

## 附录

### A. 相关文档

- [项目架构概述](../architecture/01-项目架构概述.md)
- [模块划分](../architecture/03-模块划分.md)
- [常见问题解决方案](../guides/01-常见问题解决方案.md)

### B. 外部 API 文档

- [OpenAI API 文档](https://platform.openai.com/docs/api-reference)
- [Anthropic API 文档](https://docs.anthropic.com/claude/reference)
- [Cohere API 文档](https://docs.cohere.com/reference)
- [Google Gemini API 文档](https://ai.google.dev/docs)
- [MCP 协议文档](https://modelcontextprotocol.io)
