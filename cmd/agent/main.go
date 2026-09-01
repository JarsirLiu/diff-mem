// Command agent is the Diff-Mem CLI agent — an interactive terminal assistant
// that connects to a Diff-Mem HTTP server and manages long-term memory on
// behalf of the user.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/diff-mem/diff-mem/internal/agent"
)

func main() {
	var serverURL string
	flag.StringVar(&serverURL, "server", agent.DefaultServerURL, "Diff-Mem HTTP server URL")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Diff-Mem Agent — 内置记忆助手

用法:
  agent [flags] [command]

命令:
  repl         启动交互式 REPL（默认）
  init         初始化 agent 记忆节点
  recall Q     检索记忆: agent recall "关键词"
  store S      存储内容: agent store "要记的内容"
  status       显示 agent 状态
  forget P     归档节点: agent forget "/path/to/node"

示例:
  agent repl
  agent -server http://localhost:9090 repl
  agent recall "昨天做了什么"
  agent store "明天开会讨论 API 设计"

标志:
`)
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr)
	}

	flag.Parse()

	ctx := context.Background()
	a := agent.New(serverURL)
	args := flag.Args()

	if len(args) == 0 {
		err := runRepl(ctx, a)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	switch args[0] {
	case "repl":
		err := runRepl(ctx, a)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

	case "init":
		fmt.Println("初始化 agent...")
		if err := a.Init(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✓ 初始化完成")

	case "recall":
		if len(args) < 2 || args[1] == "" {
			fmt.Fprintln(os.Stderr, "用法: agent recall <query>")
			os.Exit(1)
		}
		if err := a.Init(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "init warning: %v\n", err)
		}
		results, err := a.Recall(ctx, args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		printRecallResults(results)

	case "store":
		if len(args) < 2 || args[1] == "" {
			fmt.Fprintln(os.Stderr, "用法: agent store <content>")
			os.Exit(1)
		}
		if err := a.Init(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "init warning: %v\n", err)
			os.Exit(1)
		}
		content := strings.Join(args[1:], " ")
		if err := a.Store(ctx, content, "CLI 单次存储"); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ 已记录: %s\n", content[:min(len(content), 40)])

	case "status":
		status, err := a.Status(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Agent 状态\n")
		fmt.Printf("  Server:    %s\n", status.ServerURL)
		fmt.Printf("  Session:   %s [%s]\n", status.SessionPath, boolStr(status.SessionExists))
		fmt.Printf("  Profile:   %s [%s]\n", status.ProfilePath, boolStr(status.ProfileExists))
		fmt.Printf("  交互次数:  %d\n", status.InteractionCount)
		if status.LastInteraction != nil {
			fmt.Printf("  最后交互:  %s\n", *status.LastInteraction)
		}

	case "forget":
		if len(args) < 2 || args[1] == "" {
			fmt.Fprintln(os.Stderr, "用法: agent forget <path>")
			os.Exit(1)
		}
		if err := a.Init(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "init warning: %v\n", err)
			os.Exit(1)
		}
		path := args[1]
		if err := a.ForgetPath(ctx, path, "CLI 单次归档"); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ 已归档: %s\n", path)

	default:
		fmt.Fprintf(os.Stderr, "未知命令: %s\n", args[0])
		flag.Usage()
		os.Exit(1)
	}
}

func runRepl(ctx context.Context, a *agent.Agent) error {
	return agent.RunREPL(ctx, a, os.Stdin, os.Stdout)
}

func printRecallResults(results []*agent.RecallResult) {
	if len(results) == 0 {
		fmt.Println("没有找到相关记忆")
		return
	}
	for i, r := range results {
		fmt.Printf("[%d] %s\n", i+1, r.Path)
		if r.Title != "" {
			fmt.Printf("    标题: %s\n", r.Title)
		}
		if r.Status != "" {
			fmt.Printf("    状态: %s\n", r.Status)
		}
		if r.Summary != "" {
			fmt.Printf("    摘要: %s\n", r.Summary)
		}
		if len(r.Tags) > 0 {
			fmt.Printf("    标签: %s\n", strings.Join(r.Tags, ", "))
		}
		if len(r.Events) > 0 {
			fmt.Println("    最近事件:")
			for _, e := range r.Events {
				fmt.Printf("      [%s] %s\n", e.Type, e.Content)
			}
		}
		fmt.Println()
	}
}

func boolStr(v bool) string {
	if v {
		return "存在"
	}
	return "不存在"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
