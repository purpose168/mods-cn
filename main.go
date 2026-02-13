// Package main 提供 mods CLI 工具。
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"runtime/pprof"
	"slices"
	"strings"

	"github.com/atotto/clipboard"
	timeago "github.com/caarlos0/timea.go"
	tea "github.com/charmbracelet/bubbletea"
	glamour "github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/mods/internal/cache"
	"github.com/charmbracelet/x/editor"
	mcobra "github.com/muesli/mango-cobra"
	"github.com/muesli/roff"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
)

// Build vars 构建变量
var (
	//nolint: gochecknoglobals
	Version   = "" // 版本号
	CommitSHA = "" // 提交 SHA
)

// buildVersion 构建版本信息
func buildVersion() {
	if len(CommitSHA) >= sha1short {
		vt := rootCmd.VersionTemplate()
		rootCmd.SetVersionTemplate(vt[:len(vt)-1] + " (" + CommitSHA[0:7] + ")\n")
	}
	if Version == "" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Sum != "" {
			Version = info.Main.Version
		} else {
			Version = "未知（从源代码构建）"
		}
	}
	rootCmd.Version = Version
}

func init() {
	// XXX: 在 Glamour 深色和浅色样式中取消设置错误样式。
	// 在 glamour 方面，我们可能应该添加构造函数来生成
	// 默认样式，以便它们可以基本上被复制和修改，而无需
	// 在 Glamour 本身中改变定义（或依赖任何深度
	// 复制）。
	glamour.DarkStyleConfig.CodeBlock.Chroma.Error.BackgroundColor = new(string)
	glamour.LightStyleConfig.CodeBlock.Chroma.Error.BackgroundColor = new(string)

	buildVersion()
	rootCmd.SetUsageFunc(usageFunc)
	rootCmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return newFlagParseError(err)
	})

	rootCmd.CompletionOptions.HiddenDefaultCmd = true
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})
}

var (
	config = defaultConfig() // 配置实例
	db     *convoDB          // 数据库实例

	rootCmd = &cobra.Command{
		Use:           "mods",
		Short:         "命令行上的 GPT。专为管道构建。",
		SilenceUsage:  true,
		SilenceErrors: true,
		Example:       randomExample(),
		RunE: func(cmd *cobra.Command, args []string) error {
			config.Prefix = removeWhitespace(strings.Join(args, " "))

			opts := []tea.ProgramOption{}

			if !isInputTTY() || config.Raw {
				opts = append(opts, tea.WithInput(nil))
			}
			if isOutputTTY() && !config.Raw {
				opts = append(opts, tea.WithOutput(os.Stderr))
			} else {
				opts = append(opts, tea.WithoutRenderer())
			}
			if os.Getenv("VIMRUNTIME") != "" {
				config.Quiet = true
			}

			if isNoArgs() && isInputTTY() && config.openEditor {
				prompt, err := prefixFromEditor()
				if err != nil {
					return err
				}
				config.Prefix = prompt
			}

			if (isNoArgs() || config.AskModel) && isInputTTY() {
				if err := askInfo(); err != nil && err == huh.ErrUserAborted {
					return modsError{
						err:    err,
						reason: "用户已取消。",
					}
				} else if err != nil {
					return modsError{
						err:    err,
						reason: "提示失败。",
					}
				}
			}

			cache, err := cache.NewConversations(config.CachePath)
			if err != nil {
				return modsError{err, "无法启动 Bubble Tea 程序。"}
			}
			mods := newMods(cmd.Context(), stderrRenderer(), &config, db, cache)
			p := tea.NewProgram(mods, opts...)
			m, err := p.Run()
			if err != nil {
				return modsError{err, "无法启动 Bubble Tea 程序。"}
			}

			mods = m.(*Mods)
			if mods.Error != nil {
				return *mods.Error
			}

			if config.Dirs {
				if len(args) > 0 {
					switch args[0] {
					case "config":
						fmt.Println(filepath.Dir(config.SettingsPath))
						return nil
					case "cache":
						fmt.Println(config.CachePath)
						return nil
					}
				}
				fmt.Printf("配置: %s\n", filepath.Dir(config.SettingsPath))
				//nolint:mnd
				fmt.Printf("%*s缓存: %s\n", 8, " ", config.CachePath)
				return nil
			}

			if config.Settings {
				c, err := editor.Cmd("mods", config.SettingsPath)
				if err != nil {
					return modsError{
						err:    err,
						reason: "无法编辑您的设置文件。",
					}
				}
				c.Stdin = os.Stdin
				c.Stdout = os.Stdout
				c.Stderr = os.Stderr
				if err := c.Run(); err != nil {
					return modsError{err, fmt.Sprintf(
						"缺少 %s。",
						stderrStyles().InlineCode.Render("$EDITOR"),
					)}
				}

				if !config.Quiet {
					fmt.Fprintln(os.Stderr, "配置文件已写入:", config.SettingsPath)
				}
				return nil
			}

			if config.ResetSettings {
				return resetSettings()
			}

			if mods.Input == "" && isNoArgs() {
				return modsError{
					reason: "您没有提供任何提示输入。",
					err: newUserErrorf(
						"您可以通过参数提供提示和/或通过 STDIN 管道传输。\n示例: %s",
						stdoutStyles().InlineCode.Render("mods [提示]"),
					),
				}
			}

			if config.ShowHelp {
				return cmd.Usage()
			}

			if config.ListRoles {
				listRoles()
				return nil
			}
			if config.List {
				return listConversations(config.Raw)
			}

			if config.MCPList {
				mcpList()
				return nil
			}

			if config.MCPListTools {
				ctx, cancel := context.WithTimeout(cmd.Context(), config.MCPTimeout)
				defer cancel()
				return mcpListTools(ctx)
			}

			if len(config.Delete) > 0 {
				return deleteConversations()
			}

			if config.DeleteOlderThan > 0 {
				return deleteConversationOlderThan()
			}

			// 原始模式已经打印输出，无需再次打印
			if isOutputTTY() && !config.Raw {
				switch {
				case mods.glamOutput != "":
					fmt.Print(mods.glamOutput)
				case mods.Output != "":
					fmt.Print(mods.Output)
				}
			}

			if config.Show != "" || config.ShowLast {
				return nil
			}

			if config.cacheWriteToID != "" {
				return saveConversation(mods)
			}

			return nil
		},
	}
)

var memprofile bool // 内存分析标志

func initFlags() {
	flags := rootCmd.Flags()
	flags.StringVarP(&config.Model, "model", "m", config.Model, stdoutStyles().FlagDesc.Render(help["model"]))
	flags.BoolVarP(&config.AskModel, "ask-model", "M", config.AskModel, stdoutStyles().FlagDesc.Render(help["ask-model"]))
	flags.StringVarP(&config.API, "api", "a", config.API, stdoutStyles().FlagDesc.Render(help["api"]))
	flags.StringVarP(&config.HTTPProxy, "http-proxy", "x", config.HTTPProxy, stdoutStyles().FlagDesc.Render(help["http-proxy"]))
	flags.BoolVarP(&config.Format, "format", "f", config.Format, stdoutStyles().FlagDesc.Render(help["format"]))
	flags.StringVar(&config.FormatAs, "format-as", config.FormatAs, stdoutStyles().FlagDesc.Render(help["format-as"]))
	flags.BoolVarP(&config.Raw, "raw", "r", config.Raw, stdoutStyles().FlagDesc.Render(help["raw"]))
	flags.IntVarP(&config.IncludePrompt, "prompt", "P", config.IncludePrompt, stdoutStyles().FlagDesc.Render(help["prompt"]))
	flags.BoolVarP(&config.IncludePromptArgs, "prompt-args", "p", config.IncludePromptArgs, stdoutStyles().FlagDesc.Render(help["prompt-args"]))
	flags.StringVarP(&config.Continue, "continue", "c", "", stdoutStyles().FlagDesc.Render(help["continue"]))
	flags.BoolVarP(&config.ContinueLast, "continue-last", "C", false, stdoutStyles().FlagDesc.Render(help["continue-last"]))
	flags.BoolVarP(&config.List, "list", "l", config.List, stdoutStyles().FlagDesc.Render(help["list"]))
	flags.StringVarP(&config.Title, "title", "t", config.Title, stdoutStyles().FlagDesc.Render(help["title"]))
	flags.StringArrayVarP(&config.Delete, "delete", "d", config.Delete, stdoutStyles().FlagDesc.Render(help["delete"]))
	flags.Var(newDurationFlag(config.DeleteOlderThan, &config.DeleteOlderThan), "delete-older-than", stdoutStyles().FlagDesc.Render(help["delete-older-than"]))
	flags.StringVarP(&config.Show, "show", "s", config.Show, stdoutStyles().FlagDesc.Render(help["show"]))
	flags.BoolVarP(&config.ShowLast, "show-last", "S", false, stdoutStyles().FlagDesc.Render(help["show-last"]))
	flags.BoolVarP(&config.Quiet, "quiet", "q", config.Quiet, stdoutStyles().FlagDesc.Render(help["quiet"]))
	flags.BoolVarP(&config.ShowHelp, "help", "h", false, stdoutStyles().FlagDesc.Render(help["help"]))
	flags.BoolVarP(&config.Version, "version", "v", false, stdoutStyles().FlagDesc.Render(help["version"]))
	flags.IntVar(&config.MaxRetries, "max-retries", config.MaxRetries, stdoutStyles().FlagDesc.Render(help["max-retries"]))
	flags.BoolVar(&config.NoLimit, "no-limit", config.NoLimit, stdoutStyles().FlagDesc.Render(help["no-limit"]))
	flags.Int64Var(&config.MaxTokens, "max-tokens", config.MaxTokens, stdoutStyles().FlagDesc.Render(help["max-tokens"]))
	flags.IntVar(&config.WordWrap, "word-wrap", config.WordWrap, stdoutStyles().FlagDesc.Render(help["word-wrap"]))
	flags.Float64Var(&config.Temperature, "temp", config.Temperature, stdoutStyles().FlagDesc.Render(help["temp"]))
	flags.StringArrayVar(&config.Stop, "stop", config.Stop, stdoutStyles().FlagDesc.Render(help["stop"]))
	flags.Float64Var(&config.TopP, "topp", config.TopP, stdoutStyles().FlagDesc.Render(help["topp"]))
	flags.Int64Var(&config.TopK, "topk", config.TopK, stdoutStyles().FlagDesc.Render(help["topk"]))
	flags.UintVar(&config.Fanciness, "fanciness", config.Fanciness, stdoutStyles().FlagDesc.Render(help["fanciness"]))
	flags.StringVar(&config.StatusText, "status-text", config.StatusText, stdoutStyles().FlagDesc.Render(help["status-text"]))
	flags.BoolVar(&config.NoCache, "no-cache", config.NoCache, stdoutStyles().FlagDesc.Render(help["no-cache"]))
	flags.BoolVar(&config.ResetSettings, "reset-settings", config.ResetSettings, stdoutStyles().FlagDesc.Render(help["reset-settings"]))
	flags.BoolVar(&config.Settings, "settings", false, stdoutStyles().FlagDesc.Render(help["settings"]))
	flags.BoolVar(&config.Dirs, "dirs", false, stdoutStyles().FlagDesc.Render(help["dirs"]))
	flags.StringVarP(&config.Role, "role", "R", config.Role, stdoutStyles().FlagDesc.Render(help["role"]))
	flags.BoolVar(&config.ListRoles, "list-roles", config.ListRoles, stdoutStyles().FlagDesc.Render(help["list-roles"]))
	flags.StringVar(&config.Theme, "theme", "charm", stdoutStyles().FlagDesc.Render(help["theme"]))
	flags.BoolVarP(&config.openEditor, "editor", "e", false, stdoutStyles().FlagDesc.Render(help["editor"]))
	flags.BoolVar(&config.MCPList, "mcp-list", false, stdoutStyles().FlagDesc.Render(help["mcp-list"]))
	flags.BoolVar(&config.MCPListTools, "mcp-list-tools", false, stdoutStyles().FlagDesc.Render(help["mcp-list-tools"]))
	flags.StringArrayVar(&config.MCPDisable, "mcp-disable", nil, stdoutStyles().FlagDesc.Render(help["mcp-disable"]))
	flags.Lookup("prompt").NoOptDefVal = "-1"
	flags.SortFlags = false

	flags.BoolVar(&memprofile, "memprofile", false, "Write memory profiles to CWD")
	_ = flags.MarkHidden("memprofile")

	for _, name := range []string{"show", "delete", "continue"} {
		_ = rootCmd.RegisterFlagCompletionFunc(name, func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			results, _ := db.Completions(toComplete)
			return results, cobra.ShellCompDirectiveDefault
		})
	}
	_ = rootCmd.RegisterFlagCompletionFunc("role", func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return roleNames(toComplete), cobra.ShellCompDirectiveDefault
	})

	if config.FormatText == nil {
		config.FormatText = defaultConfig().FormatText
	}

	if config.Format && config.FormatAs == "" {
		config.FormatAs = "markdown"
	}

	if config.Format && config.FormatAs != "" && config.FormatText[config.FormatAs] == "" {
		config.FormatText[config.FormatAs] = defaultConfig().FormatText[config.FormatAs]
	}

	if config.MCPTimeout == 0 {
		config.MCPTimeout = defaultConfig().MCPTimeout
	}

	rootCmd.MarkFlagsMutuallyExclusive(
		"settings",
		"show",
		"show-last",
		"delete",
		"delete-older-than",
		"list",
		"continue",
		"continue-last",
		"reset-settings",
		"mcp-list",
		"mcp-list-tools",
	)
}

func main() {
	defer maybeWriteMemProfile()
	var err error
	config, err = ensureConfig()
	if err != nil {
		handleError(modsError{err, "无法加载您的配置文件。"})
		// 如果用户正在编辑设置，只打印错误，但不退出。
		if !slices.Contains(os.Args, "--settings") {
			os.Exit(1)
		}
	}

	// XXX: 这必须在创建配置之后执行。
	initFlags()

	if !isCompletionCmd(os.Args) && !isManCmd(os.Args) && !isVersionOrHelpCmd(os.Args) {
		db, err = openDB(filepath.Join(config.CachePath, "conversations", "mods.db"))
		if err != nil {
			handleError(modsError{err, "无法打开数据库。"})
			os.Exit(1)
		}
		defer db.Close() //nolint:errcheck
	}

	if isCompletionCmd(os.Args) {
		// XXX: 由于 mods 没有任何子命令，Cobra 不会创建
		// 默认的 `completion` 命令。在使用补全时，
		// 通过添加一个假命令来强制创建补全相关的子命令。
		rootCmd.AddCommand(&cobra.Command{
			Use:    "____fake_command_to_enable_completions",
			Hidden: true,
		})
		rootCmd.InitDefaultCompletionCmd()
	}

	if isManCmd(os.Args) {
		rootCmd.AddCommand(&cobra.Command{
			Use:                   "man",
			Short:                 "生成手册页",
			SilenceUsage:          true,
			DisableFlagsInUseLine: true,
			Hidden:                true,
			Args:                  cobra.NoArgs,
			RunE: func(*cobra.Command, []string) error {
				manPage, err := mcobra.NewManPage(1, rootCmd)
				if err != nil {
					//nolint:wrapcheck
					return err
				}
				_, err = fmt.Fprint(os.Stdout, manPage.Build(roff.NewDocument()))
				//nolint:wrapcheck
				return err
			},
		})
	}

	if err := rootCmd.Execute(); err != nil {
		handleError(err)
		_ = db.Close()
		os.Exit(1)
	}
}

// maybeWriteMemProfile 可能写入内存分析文件
func maybeWriteMemProfile() {
	if !memprofile {
		return
	}

	closers := []func() error{db.Close}
	defer func() {
		for _, cl := range closers {
			_ = cl()
		}
	}()

	handle := func(err error) {
		fmt.Println(err)
		for _, cl := range closers {
			_ = cl()
		}
		os.Exit(1)
	}

	heap, err := os.Create("mods_heap.profile")
	if err != nil {
		handle(err)
	}
	closers = append(closers, heap.Close)
	allocs, err := os.Create("mods_allocs.profile")
	if err != nil {
		handle(err)
	}
	closers = append(closers, allocs.Close)

	if err := pprof.Lookup("heap").WriteTo(heap, 0); err != nil {
		handle(err)
	}
	if err := pprof.Lookup("allocs").WriteTo(allocs, 0); err != nil {
		handle(err)
	}
}

// handleError 处理错误
func handleError(err error) {
	maybeWriteMemProfile()
	// 排空 stdin
	if !isInputTTY() {
		_, _ = io.ReadAll(os.Stdin)
	}

	format := "\n%s\n\n"

	var args []any
	var ferr flagParseError
	var merr modsError
	if errors.As(err, &ferr) {
		format += "%s\n\n"
		args = []any{
			fmt.Sprintf(
				"查看 %s %s",
				stderrStyles().InlineCode.Render("mods -h"),
				stderrStyles().Comment.Render("获取帮助。"),
			),
			fmt.Sprintf(
				ferr.ReasonFormat(),
				stderrStyles().InlineCode.Render(ferr.Flag()),
			),
		}
	} else if errors.As(err, &merr) {
		args = []any{
			stderrStyles().ErrPadding.Render(stderrStyles().ErrorHeader.String(), merr.reason),
		}

		// 如果用户只是取消了 huh，则跳过错误详细信息。
		if merr.err != huh.ErrUserAborted {
			format += "%s\n\n"
			args = append(args, stderrStyles().ErrPadding.Render(stderrStyles().ErrorDetails.Render(err.Error())))
		}
	} else {
		args = []any{
			stderrStyles().ErrPadding.Render(stderrStyles().ErrorDetails.Render(err.Error())),
		}
	}

	fmt.Fprintf(os.Stderr, format, args...)
}

// resetSettings 重置设置
func resetSettings() error {
	_, err := os.Stat(config.SettingsPath)
	if err != nil {
		return modsError{err, "无法读取配置文件。"}
	}
	inputFile, err := os.Open(config.SettingsPath)
	if err != nil {
		return modsError{err, "无法打开配置文件。"}
	}
	defer inputFile.Close() //nolint:errcheck
	outputFile, err := os.Create(config.SettingsPath + ".bak")
	if err != nil {
		return modsError{err, "无法备份配置文件。"}
	}
	defer outputFile.Close() //nolint:errcheck
	_, err = io.Copy(outputFile, inputFile)
	if err != nil {
		return modsError{err, "无法写入配置文件。"}
	}
	// 复制成功，现在删除原始文件
	err = os.Remove(config.SettingsPath)
	if err != nil {
		return modsError{err, "无法删除配置文件。"}
	}
	err = writeConfigFile(config.SettingsPath)
	if err != nil {
		return modsError{err, "无法写入新配置文件。"}
	}
	if !config.Quiet {
		fmt.Fprintln(os.Stderr, "\n设置已恢复为默认值！")
		fmt.Fprintf(os.Stderr,
			"\n  %s %s\n\n",
			stderrStyles().Comment.Render("您的旧设置已保存到:"),
			stderrStyles().Link.Render(config.SettingsPath+".bak"),
		)
	}
	return nil
}

// deleteConversationOlderThan 删除早于指定时间的对话
func deleteConversationOlderThan() error {
	conversations, err := db.ListOlderThan(config.DeleteOlderThan)
	if err != nil {
		return modsError{err, "无法找到要删除的对话。"}
	}

	if len(conversations) == 0 {
		if !config.Quiet {
			fmt.Fprintln(os.Stderr, "未找到对话。")
			return nil
		}
		return nil
	}

	if !config.Quiet {
		printList(conversations)

		if !isOutputTTY() || !isInputTTY() {
			fmt.Fprintln(os.Stderr)
			return newUserErrorf(
				"要删除上述对话，请运行: %s",
				strings.Join(append(os.Args, "--quiet"), " "),
			)
		}
		var confirm bool
		if err := huh.Run(
			huh.NewConfirm().
				Title(fmt.Sprintf("删除早于 %s 的对话？", config.DeleteOlderThan)).
				Description(fmt.Sprintf("这将删除上面列出的所有 %d 个对话。", len(conversations))).
				Value(&confirm),
		); err != nil {
			return modsError{err, "无法删除旧对话。"}
		}
		if !confirm {
			return newUserErrorf("用户中止")
		}
	}

	cache, err := cache.NewConversations(config.CachePath)
	if err != nil {
		return modsError{err, "无法删除对话。"}
	}
	for _, c := range conversations {
		if err := db.Delete(c.ID); err != nil {
			return modsError{err, "无法删除对话。"}
		}

		if err := cache.Delete(c.ID); err != nil {
			return modsError{err, "无法删除对话。"}
		}

		if !config.Quiet {
			fmt.Fprintln(os.Stderr, "对话已删除:", c.ID[:sha1minLen])
		}
	}

	return nil
}

// deleteConversations 删除对话
func deleteConversations() error {
	for _, del := range config.Delete {
		convo, err := db.Find(del)
		if err != nil {
			return modsError{err, "无法找到要删除的对话。"}
		}
		if err := deleteConversation(convo); err != nil {
			return err
		}
	}
	return nil
}

// deleteConversation 删除单个对话
func deleteConversation(convo *Conversation) error {
	if err := db.Delete(convo.ID); err != nil {
		return modsError{err, "无法删除对话。"}
	}

	cache, err := cache.NewConversations(config.CachePath)
	if err != nil {
		return modsError{err, "无法删除对话。"}
	}
	if err := cache.Delete(convo.ID); err != nil {
		return modsError{err, "无法删除对话。"}
	}

	if !config.Quiet {
		fmt.Fprintln(os.Stderr, "对话已删除:", convo.ID[:sha1minLen])
	}
	return nil
}

// listConversations 列出对话
func listConversations(raw bool) error {
	conversations, err := db.List()
	if err != nil {
		return modsError{err, "无法列出保存的对话。"}
	}

	if len(conversations) == 0 {
		fmt.Fprintln(os.Stderr, "未找到对话。")
		return nil
	}

	if isInputTTY() && isOutputTTY() && !raw {
		selectFromList(conversations)
		return nil
	}
	printList(conversations)
	return nil
}

// roleNames 获取角色名称列表
// prefix: 前缀过滤
// 返回：角色名称列表
func roleNames(prefix string) []string {
	roles := make([]string, 0, len(config.Roles))
	for role := range config.Roles {
		if prefix != "" && !strings.HasPrefix(role, prefix) {
			continue
		}
		roles = append(roles, role)
	}
	slices.Sort(roles)
	return roles
}

// listRoles 列出角色
func listRoles() {
	for _, role := range roleNames("") {
		s := role
		if role == config.Role {
			s = role + stdoutStyles().Timeago.Render(" (默认)")
		}
		fmt.Println(s)
	}
}

// makeOptions 创建选项列表
// conversations: 对话列表
// 返回：选项列表
func makeOptions(conversations []Conversation) []huh.Option[string] {
	opts := make([]huh.Option[string], 0, len(conversations))
	for _, c := range conversations {
		timea := stdoutStyles().Timeago.Render(timeago.Of(c.UpdatedAt))
		left := stdoutStyles().SHA1.Render(c.ID[:sha1short])
		right := stdoutStyles().ConversationList.Render(c.Title, timea)
		if c.Model != nil {
			right += stdoutStyles().Comment.Render(*c.Model)
		}
		if c.API != nil {
			right += stdoutStyles().Comment.Render(" (" + *c.API + ")")
		}
		opts = append(opts, huh.NewOption(left+" "+right, c.ID))
	}
	return opts
}

// selectFromList 从列表中选择
// conversations: 对话列表
func selectFromList(conversations []Conversation) {
	var selected string
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("对话").
				Value(&selected).
				Options(makeOptions(conversations)...),
		),
	).Run(); err != nil {
		if !errors.Is(err, huh.ErrUserAborted) {
			fmt.Fprintln(os.Stderr, err.Error())
		}
		return
	}

	_ = clipboard.WriteAll(selected)
	termenv.Copy(selected)
	printConfirmation("已复制", selected)
	// 建议使用此对话 ID 的操作
	fmt.Println(stdoutStyles().Comment.Render(
		"您可以在以下命令中使用此对话 ID:",
	))
	suggestions := []string{"show", "continue", "delete"}
	for _, flag := range suggestions {
		fmt.Printf(
			"  %-44s %s\n",
			stdoutStyles().Flag.Render("--"+flag),
			stdoutStyles().FlagDesc.Render(help[flag]),
		)
	}
}

// printList 打印对话列表
// conversations: 对话列表
func printList(conversations []Conversation) {
	for _, conversation := range conversations {
		_, _ = fmt.Fprintf(
			os.Stdout,
			"%s\t%s\t%s\n",
			stdoutStyles().SHA1.Render(conversation.ID[:sha1short]),
			conversation.Title,
			stdoutStyles().Timeago.Render(timeago.Of(conversation.UpdatedAt)),
		)
	}
}

// saveConversation 保存对话
// mods: Mods 实例
// 返回：错误信息
func saveConversation(mods *Mods) error {
	if config.NoCache {
		if !config.Quiet {
			fmt.Fprintf(
				os.Stderr,
				"\n对话未保存，因为设置了 %s 或 %s。\n",
				stderrStyles().InlineCode.Render("--no-cache"),
				stderrStyles().InlineCode.Render("NO_CACHE"),
			)
		}
		return nil
	}

	// 如果消息是 sha1，则使用最后的提示代替。
	id := config.cacheWriteToID
	title := strings.TrimSpace(config.cacheWriteToTitle)

	if sha1reg.MatchString(title) || title == "" {
		title = firstLine(lastPrompt(mods.messages))
	}

	errReason := fmt.Sprintf(
		"将 %s 写入缓存时出现问题。使用 %s / %s 禁用它。",
		config.cacheWriteToID,
		stderrStyles().InlineCode.Render("--no-cache"),
		stderrStyles().InlineCode.Render("NO_CACHE"),
	)
	cache, err := cache.NewConversations(config.CachePath)
	if err != nil {
		return modsError{err, errReason}
	}
	if err := cache.Write(id, &mods.messages); err != nil {
		return modsError{err, errReason}
	}
	if err := db.Save(id, title, config.API, config.Model); err != nil {
		_ = cache.Delete(id) // 删除残留数据
		return modsError{err, errReason}
	}

	if !config.Quiet {
		fmt.Fprintln(
			os.Stderr,
			"\n对话已保存:",
			stderrStyles().InlineCode.Render(config.cacheWriteToID[:sha1short]),
			stderrStyles().Comment.Render(title),
		)
	}
	return nil
}

// isNoArgs 检查是否没有参数
func isNoArgs() bool {
	return config.Prefix == "" &&
		config.Show == "" &&
		!config.ShowLast &&
		len(config.Delete) == 0 &&
		config.DeleteOlderThan == 0 &&
		!config.ShowHelp &&
		!config.List &&
		!config.ListRoles &&
		!config.MCPList &&
		!config.MCPListTools &&
		!config.Dirs &&
		!config.Settings &&
		!config.ResetSettings
}

// askInfo 询问信息
func askInfo() error {
	var foundModel bool
	apis := make([]huh.Option[string], 0, len(config.APIs))
	opts := map[string][]huh.Option[string]{}
	for _, api := range config.APIs {
		apis = append(apis, huh.NewOption(api.Name, api.Name))
		for name, model := range api.Models {
			opts[api.Name] = append(opts[api.Name], huh.NewOption(name, name))

			// 检查这是否是我们要使用的模型（如果不使用 `--ask-model`）：
			if !config.AskModel &&
				(config.API == "" || config.API == api.Name) &&
				(config.Model == name || slices.Contains(model.Aliases, config.Model)) {
				// 如果是，调整 api 和 model 以便后续更便宜。
				config.API = api.Name
				config.Model = name
				foundModel = true
			}
		}
	}

	if config.ContinueLast {
		found, err := db.FindHEAD()
		if err == nil && found != nil && found.Model != nil && found.API != nil {
			config.Model = *found.Model
			config.API = *found.API
			foundModel = true
		}
	}

	// 包装由调用者完成
	//nolint:wrapcheck
	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("选择 API:").
				Options(apis...).
				Value(&config.API),
			huh.NewSelect[string]().
				TitleFunc(func() string {
					return fmt.Sprintf("选择 '%s' 的模型:", config.API)
				}, &config.API).
				OptionsFunc(func() []huh.Option[string] {
					return opts[config.API]
				}, &config.API).
				Value(&config.Model),
		).WithHideFunc(func() bool {
			// AskModel 为 true 表示用户传递了询问标志；
			// FoundModel 为 true 表示找到了用户配置的模型
			// （无论是 --api/--model 还是设置中的 default-api 和
			// default-model）。
			// 因此，只有当用户没有使用 `--ask-model` 运行
			// 且配置产生了有效模型时，才会隐藏此项。
			return !config.AskModel && foundModel
		}),
		huh.NewGroup(
			huh.NewText().
				TitleFunc(func() string {
					return fmt.Sprintf("输入 %s/%s 的提示:", config.API, config.Model)
				}, &config.Model).
				Value(&config.Prefix),
		).WithHideFunc(func() bool {
			return config.Prefix != ""
		}),
	).
		WithTheme(themeFrom(config.Theme)).
		Run()
}

// isManCmd 检查是否为手册命令
//nolint:mnd
func isManCmd(args []string) bool {
	if len(args) == 2 {
		return args[1] == "man"
	}
	if len(args) == 3 && args[1] == "man" {
		return args[2] == "-h" || args[2] == "--help"
	}
	return false
}

// isCompletionCmd 检查是否为补全命令
//nolint:mnd
func isCompletionCmd(args []string) bool {
	if len(args) <= 1 {
		return false
	}
	if args[1] == "__complete" {
		return true
	}
	if args[1] != "completion" {
		return false
	}
	if len(args) == 3 {
		_, ok := map[string]any{
			"bash":       nil,
			"fish":       nil,
			"zsh":        nil,
			"powershell": nil,
			"-h":         nil,
			"--help":     nil,
			"help":       nil,
		}[args[2]]
		return ok
	}
	if len(args) == 4 {
		_, ok := map[string]any{
			"-h":     nil,
			"--help": nil,
		}[args[3]]
		return ok
	}
	return false
}

// isVersionOrHelpCmd 检查是否为版本或帮助命令
//nolint:mnd
func isVersionOrHelpCmd(args []string) bool {
	if len(args) <= 1 {
		return false
	}
	for _, arg := range args[1:] {
		if arg == "--version" || arg == "-v" || arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

// themeFrom 从主题名称获取主题
// theme: 主题名称
// 返回：主题实例
func themeFrom(theme string) *huh.Theme {
	switch theme {
	case "dracula":
		return huh.ThemeDracula()
	case "catppuccin":
		return huh.ThemeCatppuccin()
	case "base16":
		return huh.ThemeBase16()
	default:
		return huh.ThemeCharm()
	}
}

// prefixFromEditor 创建临时文件，在用户的编辑器中打开它，然后返回其内容。
func prefixFromEditor() (string, error) {
	f, err := os.CreateTemp("", "prompt")
	if err != nil {
		return "", fmt.Errorf("无法创建临时文件: %w", err)
	}
	_ = f.Close()
	defer func() { _ = os.Remove(f.Name()) }()
	cmd, err := editor.Cmd(
		"mods",
		f.Name(),
	)
	if err != nil {
		return "", fmt.Errorf("无法打开编辑器: %w", err)
	}
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("无法打开编辑器: %w", err)
	}
	prompt, err := os.ReadFile(f.Name())
	if err != nil {
		return "", fmt.Errorf("无法读取文件: %w", err)
	}
	return string(prompt), nil
}
