package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/caarlos0/go-shellwords"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/mods/internal/anthropic"
	"github.com/charmbracelet/mods/internal/cache"
	"github.com/charmbracelet/mods/internal/cohere"
	"github.com/charmbracelet/mods/internal/google"
	"github.com/charmbracelet/mods/internal/ollama"
	"github.com/charmbracelet/mods/internal/openai"
	"github.com/charmbracelet/mods/internal/proto"
	"github.com/charmbracelet/mods/internal/stream"
	"github.com/charmbracelet/x/exp/ordered"
)

// state 表示应用程序的状态类型
type state int

// 定义应用程序的各种状态常量
const (
	startState state = iota // 起始状态
	configLoadedState       // 配置加载完成状态
	requestState            // 请求状态
	responseState           // 响应状态
	doneState               // 完成状态
	errorState              // 错误状态
)

// Mods 是 Bubble Tea 模型，负责管理标准输入读取和 OpenAI API 查询
type Mods struct {
	Output        string              // 输出内容
	Input         string              // 输入内容
	Styles        styles              // 样式配置
	Error         *modsError          // 错误信息
	state         state               // 当前状态
	retries       int                 // 重试次数
	renderer      *lipgloss.Renderer  // 渲染器
	glam          *glamour.TermRenderer // Glamour 终端渲染器
	glamViewport  viewport.Model      // 视口模型
	glamOutput    string              // Glamour 输出内容
	glamHeight    int                 // Glamour 输出高度
	messages      []proto.Message     // 消息列表
	cancelRequest []context.CancelFunc // 取消请求函数列表
	anim          tea.Model           // 动画模型
	width         int                 // 宽度
	height        int                 // 高度

	db     *convoDB              // 对话数据库
	cache  *cache.Conversations  // 对话缓存
	Config *Config               // 配置信息

	content      []string     // 内容列表
	contentMutex *sync.Mutex  // 内容互斥锁

	ctx context.Context // 上下文
}

func newMods(
	ctx context.Context,
	r *lipgloss.Renderer,
	cfg *Config,
	db *convoDB,
	cache *cache.Conversations,
) *Mods {
	gr, _ := glamour.NewTermRenderer(
		glamour.WithEnvironmentConfig(),
		glamour.WithWordWrap(cfg.WordWrap),
	)
	vp := viewport.New(0, 0)
	vp.GotoBottom()
	return &Mods{
		Styles:       makeStyles(r),
		glam:         gr,
		state:        startState,
		renderer:     r,
		glamViewport: vp,
		contentMutex: &sync.Mutex{},
		db:           db,
		cache:        cache,
		Config:       cfg,
		ctx:          ctx,
	}
}

// completionInput 是一个 tea.Msg，封装了从标准输入读取的内容
type completionInput struct {
	content string
}

// completionOutput 是一个 tea.Msg，封装了从 OpenAI 返回的内容
type completionOutput struct {
	content string
	stream  stream.Stream
	errh    func(error) tea.Msg
}

// Init 实现 tea.Model 接口，初始化模型
func (m *Mods) Init() tea.Cmd {
	return m.findCacheOpsDetails()
}

// Update implements tea.Model.
func (m *Mods) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case cacheDetailsMsg:
		m.Config.cacheWriteToID = msg.WriteID
		m.Config.cacheWriteToTitle = msg.Title
		m.Config.cacheReadFromID = msg.ReadID
		m.Config.API = msg.API
		m.Config.Model = msg.Model

		if !m.Config.Quiet {
			m.anim = newAnim(m.Config.Fanciness, m.Config.StatusText, m.renderer, m.Styles)
			cmds = append(cmds, m.anim.Init())
		}
		m.state = configLoadedState
		cmds = append(cmds, m.readStdinCmd)

	case completionInput:
		// 处理补全输入消息
		if msg.content != "" {
			m.Input = removeWhitespace(msg.content)
		}
		// 检查是否有有效的输入或配置
		if m.Input == "" && m.Config.Prefix == "" && m.Config.Show == "" && !m.Config.ShowLast {
			return m, m.quit
		}
		// 检查是否需要显示帮助或配置信息
		if m.Config.Dirs ||
			len(m.Config.Delete) > 0 ||
			m.Config.DeleteOlderThan != 0 ||
			m.Config.ShowHelp ||
			m.Config.List ||
			m.Config.ListRoles ||
			m.Config.Settings ||
			m.Config.ResetSettings {
			return m, m.quit
		}

		// 如果配置了包含提示参数，添加到输出
		if m.Config.IncludePromptArgs {
			m.appendToOutput(m.Config.Prefix + "\n\n")
		}

		// 如果配置了包含提示行数，添加相应行数到输出
		if m.Config.IncludePrompt > 0 {
			parts := strings.Split(m.Input, "\n")
			if len(parts) > m.Config.IncludePrompt {
				parts = parts[0:m.Config.IncludePrompt]
			}
			m.appendToOutput(strings.Join(parts, "\n") + "\n")
		}
		m.state = requestState
		cmds = append(cmds, m.startCompletionCmd(msg.content))
	case completionOutput:
		// 处理补全输出消息
		if msg.stream == nil {
			m.state = doneState
			return m, m.quit
		}
		if msg.content != "" {
			m.appendToOutput(msg.content)
			m.state = responseState
		}
		cmds = append(cmds, m.receiveCompletionStreamCmd(completionOutput{
			stream: msg.stream,
			errh:   msg.errh,
		}))
	case modsError:
		// 处理错误消息
		m.Error = &msg
		m.state = errorState
		return m, m.quit
	case tea.WindowSizeMsg:
		// 处理窗口大小变化消息
		m.width, m.height = msg.Width, msg.Height
		m.glamViewport.Width = m.width
		m.glamViewport.Height = m.height
		return m, nil
	case tea.KeyMsg:
		// 处理按键消息
		switch msg.String() {
		case "q", "ctrl+c":
			m.state = doneState
			return m, m.quit
		}
	}
	// 如果不是静默模式且处于配置加载或请求状态，更新动画
	if !m.Config.Quiet && (m.state == configLoadedState || m.state == requestState) {
		var cmd tea.Cmd
		m.anim, cmd = m.anim.Update(msg)
		cmds = append(cmds, cmd)
	}
	if m.viewportNeeded() {
		// 仅当视口（即内容）高度超过窗口高度时响应按键
		var cmd tea.Cmd
		m.glamViewport, cmd = m.glamViewport.Update(msg)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

// viewportNeeded 检查是否需要视口（当内容高度超过窗口高度时）
func (m Mods) viewportNeeded() bool {
	return m.glamHeight > m.height
}

// View 实现 tea.Model 接口，渲染视图
func (m *Mods) View() string {
	//nolint:exhaustive
	switch m.state {
	case errorState:
		return ""
	case requestState:
		// 请求状态下显示动画
		if !m.Config.Quiet {
			return m.anim.View()
		}
	case responseState:
		// 响应状态下渲染输出
		if !m.Config.Raw && isOutputTTY() {
			if m.viewportNeeded() {
				return m.glamViewport.View()
			}
			// 还不需要视口
			return m.glamOutput
		}

		if isOutputTTY() && !m.Config.Raw {
			return m.Output
		}

		// 输出到非 TTY 终端
		m.contentMutex.Lock()
		for _, c := range m.content {
			fmt.Print(c)
		}
		m.content = []string{}
		m.contentMutex.Unlock()
	case doneState:
		// 完成状态
		if !isOutputTTY() {
			fmt.Printf("\n")
		}
		return ""
	}
	return ""
}

// quit 退出应用程序
func (m *Mods) quit() tea.Msg {
	// 取消所有正在进行的请求
	for _, cancel := range m.cancelRequest {
		cancel()
	}
	return tea.Quit()
}

// retry 重试补全请求
func (m *Mods) retry(content string, err modsError) tea.Msg {
	m.retries++
	// 检查是否达到最大重试次数
	if m.retries >= m.Config.MaxRetries {
		return err
	}
	// 指数退避等待
	wait := time.Millisecond * 100 * time.Duration(math.Pow(2, float64(m.retries))) //nolint:mnd
	time.Sleep(wait)
	return completionInput{content}
}

// startCompletionCmd 启动补全请求命令
func (m *Mods) startCompletionCmd(content string) tea.Cmd {
	// 如果配置了显示或显示最后，从缓存读取
	if m.Config.Show != "" || m.Config.ShowLast {
		return m.readFromCache()
	}

	return func() tea.Msg {
		var mod Model
		var api API
		var ccfg openai.Config
		var accfg anthropic.Config
		var cccfg cohere.Config
		var occfg ollama.Config
		var gccfg google.Config

		cfg := m.Config
		// 解析模型配置
		api, mod, err := m.resolveModel(cfg)
		cfg.API = mod.API
		if err != nil {
			return err
		}
		// 检查 API 端点是否配置
		if api.Name == "" {
			eps := make([]string, 0)
			for _, a := range cfg.APIs {
				eps = append(eps, m.Styles.InlineCode.Render(a.Name))
			}
			return modsError{
				err: newUserErrorf(
					"您配置的 API 端点有：%s",
					eps,
				),
				reason: fmt.Sprintf(
					"API 端点 %s 未配置。",
					m.Styles.InlineCode.Render(cfg.API),
				),
			}
		}

		// 根据不同的 API 类型配置客户端
		switch mod.API {
		case "ollama":
			occfg = ollama.DefaultConfig()
			if api.BaseURL != "" {
				occfg.BaseURL = api.BaseURL
			}
		case "anthropic":
			key, err := m.ensureKey(api, "ANTHROPIC_API_KEY", "https://console.anthropic.com/settings/keys")
			if err != nil {
				return modsError{err, "Anthropic 认证失败"}
			}
			accfg = anthropic.DefaultConfig(key)
			if api.BaseURL != "" {
				accfg.BaseURL = api.BaseURL
			}
		case "google":
			key, err := m.ensureKey(api, "GOOGLE_API_KEY", "https://aistudio.google.com/app/apikey")
			if err != nil {
				return modsError{err, "Google 认证失败"}
			}
			gccfg = google.DefaultConfig(mod.Name, key)
			gccfg.ThinkingBudget = mod.ThinkingBudget
		case "cohere":
			key, err := m.ensureKey(api, "COHERE_API_KEY", "https://dashboard.cohere.com/api-keys")
			if err != nil {
				return modsError{err, "Cohere 认证失败"}
			}
			cccfg = cohere.DefaultConfig(key)
			if api.BaseURL != "" {
				ccfg.BaseURL = api.BaseURL
			}
		case "azure", "azure-ad": //nolint:goconst
			key, err := m.ensureKey(api, "AZURE_OPENAI_KEY", "https://aka.ms/oai/access")
			if err != nil {
				return modsError{err, "Azure 认证失败"}
			}
			ccfg = openai.Config{
				AuthToken: key,
				BaseURL:   api.BaseURL,
			}
			if mod.API == "azure-ad" {
				ccfg.APIType = "azure-ad"
			}
			if api.User != "" {
				cfg.User = api.User
			}
		default:
			key, err := m.ensureKey(api, "OPENAI_API_KEY", "https://platform.openai.com/account/api-keys")
			if err != nil {
				return modsError{err, "OpenAI 认证失败"}
			}
			ccfg = openai.Config{
				AuthToken: key,
				BaseURL:   api.BaseURL,
			}
		}

		// 配置 HTTP 代理
		if cfg.HTTPProxy != "" {
			proxyURL, err := url.Parse(cfg.HTTPProxy)
			if err != nil {
				return modsError{err, "解析代理 URL 时出错。"}
			}
			httpClient := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
			ccfg.HTTPClient = httpClient
			accfg.HTTPClient = httpClient
			cccfg.HTTPClient = httpClient
			occfg.HTTPClient = httpClient
		}

		// 设置最大字符数
		if mod.MaxChars == 0 {
			mod.MaxChars = cfg.MaxInputChars
		}

		// 检查模型是否为 o1 模型，并相应地取消设置 max_tokens 参数，
		// 因为 o1 不支持该参数。
		// 我们改为设置 max_completion_tokens，这是支持的。
		// 发布版本不会有带破折号的前缀，所以只需匹配 o1。
		if strings.HasPrefix(mod.Name, "o1") {
			cfg.MaxTokens = 0
		}

		// 创建带超时的上下文
		ctx, cancel := context.WithTimeout(m.ctx, config.MCPTimeout)
		m.cancelRequest = append(m.cancelRequest, cancel)

		// 获取 MCP 工具
		tools, err := mcpTools(ctx)
		if err != nil {
			return err
		}

		// 设置流上下文
		if err := m.setupStreamContext(content, mod); err != nil {
			return err
		}

		// 构建请求
		request := proto.Request{
			Messages:    m.messages,
			API:         mod.API,
			Model:       mod.Name,
			User:        cfg.User,
			Temperature: ptrOrNil(cfg.Temperature),
			TopP:        ptrOrNil(cfg.TopP),
			TopK:        ptrOrNil(cfg.TopK),
			Stop:        cfg.Stop,
			Tools:       tools,
			ToolCaller: func(name string, data []byte) (string, error) {
				ctx, cancel := context.WithTimeout(m.ctx, config.MCPTimeout)
				m.cancelRequest = append(m.cancelRequest, cancel)
				return toolCall(ctx, name, data)
			},
		}
		if cfg.MaxTokens > 0 {
			request.MaxTokens = &cfg.MaxTokens
		}

		var client stream.Client
		switch mod.API {
		case "anthropic":
			client = anthropic.New(accfg)
		case "google":
			client = google.New(gccfg)
		case "cohere":
			client = cohere.New(cccfg)
		case "ollama":
			client, err = ollama.New(occfg)
		default:
			client = openai.New(ccfg)
			if cfg.Format && config.FormatAs == "json" {
				request.ResponseFormat = &config.FormatAs
			}
		}
		if err != nil {
			return modsError{err, "无法设置客户端"}
		}

		// 发起请求并返回流
		stream := client.Request(m.ctx, request)
		return m.receiveCompletionStreamCmd(completionOutput{
			stream: stream,
			errh: func(err error) tea.Msg {
				return m.handleRequestError(err, mod, m.Input)
			},
		})()
	}
}

// ensureKey 确保 API 密钥可用
func (m Mods) ensureKey(api API, defaultEnv, docsURL string) (string, error) {
	key := api.APIKey
	// 如果密钥为空且配置了环境变量，从环境变量获取
	if key == "" && api.APIKeyEnv != "" && api.APIKeyCmd == "" {
		key = os.Getenv(api.APIKeyEnv)
	}
	// 如果密钥为空且配置了命令，执行命令获取
	if key == "" && api.APIKeyCmd != "" {
		args, err := shellwords.Parse(api.APIKeyCmd)
		if err != nil {
			return "", modsError{err, "解析 api-key-cmd 失败"}
		}
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput() //nolint:gosec
		if err != nil {
			return "", modsError{err, "无法执行 api-key-cmd"}
		}
		key = strings.TrimSpace(string(out))
	}
	// 如果密钥为空，从默认环境变量获取
	if key == "" {
		key = os.Getenv(defaultEnv)
	}
	if key != "" {
		return key, nil
	}
	// 返回错误信息
	return "", modsError{
		reason: fmt.Sprintf(
			"需要 %[1]s；设置环境变量 %[1]s 或通过 %[3]s 更新 %[2]s。",
			m.Styles.InlineCode.Render(defaultEnv),
			m.Styles.InlineCode.Render("mods.yaml"),
			m.Styles.InlineCode.Render("mods --settings"),
		),
		err: newUserErrorf(
			"您可以在 %s 获取密钥",
			m.Styles.Link.Render(docsURL),
		),
	}
}

// receiveCompletionStreamCmd 接收补全流命令
func (m *Mods) receiveCompletionStreamCmd(msg completionOutput) tea.Cmd {
	return func() tea.Msg {
		// 读取流中的下一个数据块
		if msg.stream.Next() {
			chunk, err := msg.stream.Current()
			if err != nil && !errors.Is(err, stream.ErrNoContent) {
				_ = msg.stream.Close()
				return msg.errh(err)
			}
			return completionOutput{
				content: chunk.Content,
				stream:  msg.stream,
				errh:    msg.errh,
			}
		}

		// 流已完成，检查错误
		if err := msg.stream.Err(); err != nil {
			return msg.errh(err)
		}

		// 调用工具并处理结果
		results := msg.stream.CallTools()
		toolMsg := completionOutput{
			stream: msg.stream,
			errh:   msg.errh,
		}
		for _, call := range results {
			toolMsg.content += call.String()
		}
		if len(results) == 0 {
			m.messages = msg.stream.Messages()
			return completionOutput{
				errh: msg.errh,
			}
		}
		return toolMsg
	}
}

// cacheDetailsMsg 缓存详情消息
type cacheDetailsMsg struct {
	WriteID, Title, ReadID, API, Model string
}

// findCacheOpsDetails 查找缓存操作详情
func (m *Mods) findCacheOpsDetails() tea.Cmd {
	return func() tea.Msg {
		continueLast := m.Config.ContinueLast || (m.Config.Continue != "" && m.Config.Title == "")
		readID := ordered.First(m.Config.Continue, m.Config.Show)
		writeID := ordered.First(m.Config.Title, m.Config.Continue)
		title := writeID
		model := m.Config.Model
		api := m.Config.API

		// 查找读取 ID
		if readID != "" || continueLast || m.Config.ShowLast {
			found, err := m.findReadID(readID)
			if err != nil {
				return modsError{
					err:    err,
					reason: "无法找到对话。",
				}
			}
			if found != nil {
				readID = found.ID
				if found.Model != nil && found.API != nil {
					model = *found.Model
					api = *found.API
				}
			}
		}

		// 如果继续上一次对话，更新现有对话
		if continueLast {
			writeID = readID
		}

		// 如果写入 ID 为空，生成新的对话 ID
		if writeID == "" {
			writeID = newConversationID()
		}

		// 检查写入 ID 是否为 SHA1 格式
		if !sha1reg.MatchString(writeID) {
			convo, err := m.db.Find(writeID)
			if err != nil {
				// 这是一个带标题的新对话
				writeID = newConversationID()
			} else {
				writeID = convo.ID
			}
		}

		return cacheDetailsMsg{
			WriteID: writeID,
			Title:   title,
			ReadID:  readID,
			API:     api,
			Model:   model,
		}
	}
}

// findReadID 查找读取 ID
func (m *Mods) findReadID(in string) (*Conversation, error) {
	convo, err := m.db.Find(in)
	if err == nil {
		return convo, nil
	}
	// 如果没有匹配且不是显示模式，查找最新的对话
	if errors.Is(err, errNoMatches) && m.Config.Show == "" {
		convo, err := m.db.FindHEAD()
		if err != nil {
			return nil, err
		}
		return convo, nil
	}
	return nil, err
}

// readStdinCmd 读取标准输入命令
func (m *Mods) readStdinCmd() tea.Msg {
	if !isInputTTY() {
		reader := bufio.NewReader(os.Stdin)
		stdinBytes, err := io.ReadAll(reader)
		if err != nil {
			return modsError{err, "无法读取标准输入。"}
		}

		return completionInput{increaseIndent(string(stdinBytes))}
	}
	return completionInput{""}
}

// readFromCache 从缓存读取命令
func (m *Mods) readFromCache() tea.Cmd {
	return func() tea.Msg {
		var messages []proto.Message
		if err := m.cache.Read(m.Config.cacheReadFromID, &messages); err != nil {
			return modsError{err, "加载对话时出错。"}
		}

		m.appendToOutput(proto.Conversation(messages).String())
		return completionOutput{
			errh: func(err error) tea.Msg {
				return modsError{err: err}
			},
		}
	}
}

const tabWidth = 4

// appendToOutput 将内容追加到输出
func (m *Mods) appendToOutput(s string) {
	m.Output += s
	// 如果输出不是 TTY 或为原始模式，直接输出
	if !isOutputTTY() || m.Config.Raw {
		m.contentMutex.Lock()
		m.content = append(m.content, s)
		m.contentMutex.Unlock()
		return
	}

	// 渲染 Glamour 输出
	wasAtBottom := m.glamViewport.ScrollPercent() == 1.0
	oldHeight := m.glamHeight
	m.glamOutput, _ = m.glam.Render(m.Output)
	m.glamOutput = strings.TrimRightFunc(m.glamOutput, unicode.IsSpace)
	m.glamOutput = strings.ReplaceAll(m.glamOutput, "\t", strings.Repeat(" ", tabWidth))
	m.glamHeight = lipgloss.Height(m.glamOutput)
	m.glamOutput += "\n"
	truncatedGlamOutput := m.renderer.NewStyle().
		MaxWidth(m.width).
		Render(m.glamOutput)
	m.glamViewport.SetContent(truncatedGlamOutput)
	if oldHeight < m.glamHeight && wasAtBottom {
		// 如果视口在底部且收到了新的内容行，
		// 通过自动滚动到底部来跟随输出。
		m.glamViewport.GotoBottom()
	}
}

// removeWhitespace 如果输入仅包含空白字符，则将其置空
func removeWhitespace(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return s
}

var tokenErrRe = regexp.MustCompile(`This model's maximum context length is (\d+) tokens. However, your messages resulted in (\d+) tokens`)

// cutPrompt 裁剪提示词以适应模型的最大上下文长度
func cutPrompt(msg, prompt string) string {
	found := tokenErrRe.FindStringSubmatch(msg)
	if len(found) != 3 { //nolint:mnd
		return prompt
	}

	maxt, _ := strconv.Atoi(found[1])
	current, _ := strconv.Atoi(found[2])

	if maxt > current {
		return prompt
	}

	// 1 个 token 约等于 4 个字符
	// 额外裁剪 10 个字符"以防万一"
	reduceBy := 10 + (current-maxt)*4 //nolint:mnd
	if len(prompt) > reduceBy {
		return prompt[:len(prompt)-reduceBy]
	}

	return prompt
}

// increaseIndent 增加缩进
func increaseIndent(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = "\t" + lines[i]
	}
	return strings.Join(lines, "\n")
}

// resolveModel 解析模型配置
func (m *Mods) resolveModel(cfg *Config) (API, Model, error) {
	for _, api := range cfg.APIs {
		if api.Name != cfg.API && cfg.API != "" {
			continue
		}
		// 检查模型名称或别名
		for name, mod := range api.Models {
			if name == cfg.Model || slices.Contains(mod.Aliases, cfg.Model) {
				cfg.Model = name
				break
			}
		}
		mod, ok := api.Models[cfg.Model]
		if ok {
			mod.Name = cfg.Model
			mod.API = api.Name
			return api, mod, nil
		}
		// 如果指定了 API 但未找到模型，返回错误
		if cfg.API != "" {
			return API{}, Model{}, modsError{
				err: newUserErrorf(
					"可用的模型有：%s",
					strings.Join(slices.Collect(maps.Keys(api.Models)), ", "),
				),
				reason: fmt.Sprintf(
					"API 端点 %s 不包含模型 %s",
					m.Styles.InlineCode.Render(cfg.API),
					m.Styles.InlineCode.Render(cfg.Model),
				),
			}
		}
	}

	// 未找到模型，返回错误
	return API{}, Model{}, modsError{
		reason: fmt.Sprintf(
			"模型 %s 不在设置文件中。",
			m.Styles.InlineCode.Render(cfg.Model),
		),
		err: newUserErrorf(
			"请使用 %s 指定 API 端点或在设置中配置模型：%s",
			m.Styles.InlineCode.Render("--api"),
			m.Styles.InlineCode.Render("mods --settings"),
		),
	}
}

// number 数字类型约束接口
type number interface{ int64 | float64 }

// ptrOrNil 如果值为负数则返回 nil，否则返回指针
func ptrOrNil[T number](t T) *T {
	if t < 0 {
		return nil
	}
	return &t
}
