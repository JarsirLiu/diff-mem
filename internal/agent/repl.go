// REPL provides an interactive command loop for the Diff-Mem agent.
package agent

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
)

const prompt = "diff-mem-agent> "

// RunREPL starts the interactive loop reading from reader and writing to writer.
func RunREPL(ctx context.Context, agent *Agent, reader io.Reader, writer io.Writer) error {
	// Initialize agent memory
	if err := agent.Init(ctx); err != nil {
		fmt.Fprintf(writer, "⚠ init warning: %v\n", err)
	} else {
		fmt.Fprintf(writer, "✓ agent initialized (session: %s)\n\n", agent.sessionPath)
	}

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for {
		fmt.Fprintf(writer, prompt)
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		err := handleLine(ctx, agent, writer, line)
		if err != nil {
			fmt.Fprintf(writer, "✗ %v\n", err)
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		return err
	}
	return nil
}

func handleLine(ctx context.Context, agent *Agent, w io.Writer, line string) error {
	// Commands
	if strings.HasPrefix(line, ".") {
		return handleCommand(ctx, agent, w, line)
	}

	// Natural language input — determine intent
	return handleNaturalLanguage(ctx, agent, w, line)
}

func handleCommand(ctx context.Context, agent *Agent, w io.Writer, line string) error {
	parts := strings.SplitN(line, " ", 2)
	cmd := parts[0]
	arg := ""
	if len(parts) > 1 {
		arg = strings.TrimSpace(parts[1])
	}

	switch cmd {
	case ".help":
		printHelp(w)
	case ".recall":
		if arg == "" {
			return fmt.Errorf("usage: .recall <query>")
		}
		return cmdRecall(ctx, agent, w, arg)
	case ".store":
		if arg == "" {
			return fmt.Errorf("usage: .store <content>")
		}
		return cmdStore(ctx, agent, w, arg)
	case ".show":
		if arg == "" {
			return fmt.Errorf("usage: .show <path>")
		}
		return cmdShow(ctx, agent, w, arg)
	case ".status":
		return cmdStatus(ctx, agent, w)
	case ".forget":
		if arg == "" {
			return fmt.Errorf("usage: .forget <path>")
		}
		return cmdForget(ctx, agent, w, arg)
	case ".quit", ".exit":
		fmt.Fprintln(w, "goodbye!")
		return io.EOF
	default:
		return fmt.Errorf("unknown command: %s (try .help)", cmd)
	}
	return nil
}

func handleNaturalLanguage(ctx context.Context, agent *Agent, w io.Writer, line string) error {
	// Heuristic: store if it looks like a statement or command;
	// recall if it looks like a question.
	needsRecall := strings.HasSuffix(line, "?") ||
		strings.HasSuffix(line, "？") ||
		strings.HasPrefix(line, "今天") ||
		strings.HasPrefix(line, "昨天") ||
		strings.HasPrefix(line, "最近") ||
		strings.Contains(line, "做什么") ||
		strings.Contains(line, "说过") ||
		strings.Contains(line, "记得") ||
		strings.Contains(line, "之前")

	if needsRecall {
		return cmdRecall(ctx, agent, w, line)
	}

	// Default: store
	return cmdStore(ctx, agent, w, line)
}

func cmdRecall(ctx context.Context, agent *Agent, w io.Writer, query string) error {
	// Record the recall attempt
	agent.RecordInteraction(ctx, "user", "recall: "+query)

	fmt.Fprintf(w, "\n🔍 检索: %s\n", query)
	results, err := agent.Recall(ctx, query)
	if err != nil {
		return fmt.Errorf("recall error: %v", err)
	}

	if len(results) == 0 {
		fmt.Fprintln(w, "  (没有找到相关记忆)")
		return nil
	}

	for i, r := range results {
		fmt.Fprintf(w, "  [%d] %s\n", i+1, r.Path)
		if r.Title != "" {
			fmt.Fprintf(w, "      标题: %s\n", r.Title)
		}
		if r.Status != "" {
			fmt.Fprintf(w, "      状态: %s\n", r.Status)
		}
		if r.Summary != "" {
			fmt.Fprintf(w, "      摘要: %s\n", r.Summary)
		}
		if len(r.Tags) > 0 {
			fmt.Fprintf(w, "      标签: %s\n", strings.Join(r.Tags, ", "))
		}
		if len(r.Events) > 0 {
			fmt.Fprintln(w, "      最近事件:")
			for _, e := range r.Events {
				fmt.Fprintf(w, "        [%s] %s\n", e.Type, e.Content)
			}
		}
		fmt.Fprintln(w)
	}
	return nil
}

func cmdStore(ctx context.Context, agent *Agent, w io.Writer, content string) error {
	runes := []rune(content)
	preview := content
	if len(runes) > 50 {
		preview = string(runes[:50]) + "…"
	}
	reason := fmt.Sprintf("用户输入: %s", preview)
	if err := agent.Store(ctx, content, reason); err != nil {
		return fmt.Errorf("store failed: %v", err)
	}
	fmt.Fprintf(w, "✓ 已记录 (→ %s)\n", agent.sessionPath)
	return nil
}

func cmdShow(ctx context.Context, agent *Agent, w io.Writer, path string) error {
	header, err := agent.Show(ctx, path)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "  路径: %s\n", header.Path)
	fmt.Fprintf(w, "  标题: %s\n", header.Title)
	fmt.Fprintf(w, "  状态: %s\n", header.Status)
	if header.Summary != "" {
		fmt.Fprintf(w, "  摘要: %s\n", header.Summary)
	}
	if len(header.Tags) > 0 {
		fmt.Fprintf(w, "  标签: %s\n", strings.Join(header.Tags, ", "))
	}
	fmt.Fprintf(w, "  事件数: %d\n", header.EventCount)
	if len(header.Fields) > 0 {
		fmt.Fprintln(w, "  字段:")
		for k, v := range header.Fields {
			fmt.Fprintf(w, "    %s: %s\n", k, v)
		}
	}
	return nil
}

func cmdStatus(ctx context.Context, agent *Agent, w io.Writer) error {
	status, err := agent.Status(ctx)
	if err != nil {
		return fmt.Errorf("status error: %v", err)
	}
	fmt.Fprintf(w, "  Agent 状态\n")
	fmt.Fprintf(w, "  ─────────────────────\n")
	fmt.Fprintf(w, "  Server:    %s\n", status.ServerURL)
	fmt.Fprintf(w, "  Session:   %s [%s]\n", status.SessionPath, boolStr(status.SessionExists))
	fmt.Fprintf(w, "  Profile:   %s [%s]\n", status.ProfilePath, boolStr(status.ProfileExists))
	fmt.Fprintf(w, "  交互次数:  %d\n", status.InteractionCount)
	if status.LastInteraction != nil {
		fmt.Fprintf(w, "  最后交互:  %s\n", *status.LastInteraction)
	}
	return nil
}

func cmdForget(ctx context.Context, agent *Agent, w io.Writer, path string) error {
	reason := "用户手动归档"
	if err := agent.ForgetPath(ctx, path, reason); err != nil {
		return err
	}
	fmt.Fprintf(w, "✓ 已归档: %s\n", path)
	return nil
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, `
  diff-mem-agent — 内置记忆助手

  命令:
    .recall <query>     检索相关记忆
    .store <content>    记录新内容
    .show <path>        查看节点详情
    .status             显示 agent 状态
    .forget <path>      归档节点
    .help               本帮助
    .quit / .exit       退出

  自然语言:
    以 '?' 或 '？' 结尾 → 检索
    其他输入 → 自动存储`)
}

func boolStr(v bool) string {
	if v {
		return "存在"
	}
	return "不存在"
}
