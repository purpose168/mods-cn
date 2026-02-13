package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/sync/errgroup"
)

// enabledMCPs 返回已启用的 MCP 服务器迭代器
func enabledMCPs() iter.Seq2[string, MCPServerConfig] {
	return func(yield func(string, MCPServerConfig) bool) {
		names := slices.Collect(maps.Keys(config.MCPServers))
		slices.Sort(names)
		for _, name := range names {
			if !isMCPEnabled(name) {
				continue
			}
			if !yield(name, config.MCPServers[name]) {
				return
			}
		}
	}
}

// isMCPEnabled 检查 MCP 服务器是否已启用
// name: MCP 服务器名称
// 返回：是否已启用
func isMCPEnabled(name string) bool {
	return !slices.Contains(config.MCPDisable, "*") &&
		!slices.Contains(config.MCPDisable, name)
}

// mcpList 列出所有 MCP 服务器
func mcpList() {
	for name := range config.MCPServers {
		s := name
		if isMCPEnabled(name) {
			s += stdoutStyles().Timeago.Render(" (已启用)")
		}
		fmt.Println(s)
	}
}

// mcpListTools 列出所有 MCP 工具
// ctx: 上下文
// 返回：错误信息
func mcpListTools(ctx context.Context) error {
	servers, err := mcpTools(ctx)
	if err != nil {
		return err
	}
	for sname, tools := range servers {
		for _, tool := range tools {
			fmt.Print(stdoutStyles().Timeago.Render(sname + " > "))
			fmt.Println(tool.Name)
		}
	}
	return nil
}

// mcpTools 获取所有 MCP 工具
// ctx: 上下文
// 返回：工具映射和错误信息
func mcpTools(ctx context.Context) (map[string][]mcp.Tool, error) {
	var mu sync.Mutex
	var wg errgroup.Group
	result := map[string][]mcp.Tool{}
	for sname, server := range enabledMCPs() {
		wg.Go(func() error {
			serverTools, err := mcpToolsFor(ctx, sname, server)
			if errors.Is(err, context.DeadlineExceeded) {
				return modsError{
					err:    fmt.Errorf("列出 %q 的工具时超时 - 请确保配置正确。如果您的服务器需要 docker 容器，请确保它正在运行", sname),
					reason: "无法列出工具",
				}
			}
			if err != nil {
				return modsError{
					err:    err,
					reason: "无法列出工具",
				}
			}
			mu.Lock()
			result[sname] = append(result[sname], serverTools...)
			mu.Unlock()
			return nil
		})
	}
	if err := wg.Wait(); err != nil {
		return nil, err //nolint:wrapcheck
	}
	return result, nil
}

// initMcpClient 创建并初始化 MCP 客户端
// ctx: 上下文
// server: MCP 服务器配置
// 返回：MCP 客户端和错误信息
func initMcpClient(ctx context.Context, server MCPServerConfig) (*client.Client, error) {
	var cli *client.Client
	var err error

	switch server.Type {
	case "", "stdio":
		cli, err = client.NewStdioMCPClient(
			server.Command,
			append(os.Environ(), server.Env...),
			server.Args...,
		)
	case "sse":
		cli, err = client.NewSSEMCPClient(server.URL)
	case "http":
		cli, err = client.NewStreamableHttpClient(server.URL)
	default:
		return nil, fmt.Errorf("不支持的 MCP 服务器类型: %q，支持的类型有: stdio、sse、http", server.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("创建 MCP 客户端失败: %w", err)
	}

	if err := cli.Start(ctx); err != nil {
		cli.Close() //nolint:errcheck,gosec
		return nil, fmt.Errorf("启动 MCP 客户端失败: %w", err)
	}

	if _, err := cli.Initialize(ctx, mcp.InitializeRequest{}); err != nil {
		cli.Close() //nolint:errcheck,gosec
		return nil, fmt.Errorf("初始化 MCP 客户端失败: %w", err)
	}

	return cli, nil
}

// mcpToolsFor 获取指定 MCP 服务器的工具列表
// ctx: 上下文
// name: 服务器名称
// server: MCP 服务器配置
// 返回：工具列表和错误信息
func mcpToolsFor(ctx context.Context, name string, server MCPServerConfig) ([]mcp.Tool, error) {
	cli, err := initMcpClient(ctx, server)
	if err != nil {
		return nil, fmt.Errorf("无法设置 %s: %w", name, err)
	}
	defer cli.Close() //nolint:errcheck

	tools, err := cli.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("无法设置 %s: %w", name, err)
	}
	return tools.Tools, nil
}

// toolCall 调用工具
// ctx: 上下文
// name: 工具名称（格式: server_tool）
// data: 工具参数 JSON 数据
// 返回：工具执行结果和错误信息
func toolCall(ctx context.Context, name string, data []byte) (string, error) {
	sname, tool, ok := strings.Cut(name, "_")
	if !ok {
		return "", fmt.Errorf("mcp: 无效的工具名称: %q", name)
	}
	server, ok := config.MCPServers[sname]
	if !ok {
		return "", fmt.Errorf("mcp: 无效的服务器名称: %q", sname)
	}
	if !isMCPEnabled(sname) {
		return "", fmt.Errorf("mcp: 服务器已禁用: %q", sname)
	}
	client, err := initMcpClient(ctx, server)
	if err != nil {
		return "", fmt.Errorf("mcp: %w", err)
	}
	defer client.Close() //nolint:errcheck

	var args map[string]any
	if len(data) > 0 {
		if err := json.Unmarshal(data, &args); err != nil {
			return "", fmt.Errorf("mcp: %w: %s", err, string(data))
		}
	}

	request := mcp.CallToolRequest{}
	request.Params.Name = tool
	request.Params.Arguments = args
	result, err := client.CallTool(context.Background(), request)
	if err != nil {
		return "", fmt.Errorf("mcp: %w", err)
	}

	var sb strings.Builder
	for _, content := range result.Content {
		switch content := content.(type) {
		case mcp.TextContent:
			sb.WriteString(content.Text)
		default:
			sb.WriteString("[非文本内容]")
		}
	}

	if result.IsError {
		return "", errors.New(sb.String())
	}
	return sb.String(), nil
}
