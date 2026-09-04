// Package mcp provides the MCP server that wraps the Diff-Mem engine.
// Supports both stdio transport (for local client spawning) and SSE transport
// (for remote HTTP connections).
package mcp

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/diff-mem/diff-mem/internal/engine"
	stdmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type Server struct {
	srv    *stdmcp.Server
	engine *engine.Engine
	tools  map[string]*stdmcp.Tool
}

const serverInstructions = `
你正在管理用户的长期记忆系统（Diff-Mem）。你的职责是选择性地把有价值的信息写入记忆树，
而不是记录所有对话内容。以下是你必须遵守的记录规则。

【记忆分区】记忆树只有两个根目录：
- /short-term/…：短期记忆。记录近期事件、当前任务状态、临时上下文。默认写入这里。
- /long-term/…：长期记忆。只放经过沉淀、长期有价值的内容：用户偏好、项目结构、决策结论、人物关系。
- 判断规则：先写 /short-term；当你认为某条记忆 3 个月后仍有价值（或被反复检索到），
  用 create 在 /long-term 建立沉淀节点，并归档（action=archive）短期原件。
- 不要在 /long-term 记录流水账；不要让 /short-term 里的内容无限堆积。

【该记的】
- 用户说出的长期习惯、工作方式、偏好（例如"我每天早上喝咖啡""我习惯用 git 管理代码"）
- 具体事实：项目进度、决策结论、截止日期、人物关系、组织架构
- 用户的待办事项和明确承诺
- 学习到的新知识和需要记住的教训
- 用户状态的重大变化（如"张三离职了""项目从 A 切到 B"）

【不该记的】
- 闲聊寒暄（"早上好""在吗""哈哈"）
- 单纯的情绪表达（"好累""好开心""烦死了"）——除非与具体事实关联
- 已经记录过的重复信息
- 用户明确说"不用记了""忘了吧""当没说过"的内容
- 临时状态（"现在在吃饭""刚到家""在等车"）——除非是长期行为模式的一部分

【判断原则】
1. 3 个月后这条信息还有价值吗？没有就写 /short-term，有价值才考虑 /long-term。
2. 能清楚说出"谁在什么时候做了什么/说了什么"吗？说不清楚就别记。
3. 同一条信息已经记过了吗？记过了就别重复记，需要更新就用 update。
4. 每次追加事件时，reason 字段必须说明"为什么值得记"——写不出 reason 就说明不该记。
`

func New(e *engine.Engine) *Server {
	s := &Server{
		srv: stdmcp.NewServer(&stdmcp.Implementation{
			Name:    "diff-mem",
			Version: "0.1.0",
		}, &stdmcp.ServerOptions{
			Instructions: serverInstructions,
		}),
		engine: e,
		tools:  make(map[string]*stdmcp.Tool),
	}
	s.registerTools()
	return s
}

func (s *Server) registerTools() {
	s.addTool(s.srv, &stdmcp.Tool{
		Name:        "diff_mem_create",
		Description: "在记忆树中创建新节点。path 必须以 /short-term（短期）或 /long-term（长期）开头；中间目录无需预先创建。Body 事件中可用 [[/path]] 链接其他记忆，链接目标必须已存在（或是指向现有节点的目录路径）。",
		InputSchema: toolSchemas["create"],
	})
	s.addTool(s.srv, &stdmcp.Tool{
		Name:        "diff_mem_append",
		Description: "向节点追加事件。仅出现值得记录的新事实时调用。事件中可用 [[/path]] 链接其他记忆，链接目标必须已存在，否则写入被拒绝。",
		InputSchema: toolSchemas["append"],
	})
	s.addTool(s.srv, &stdmcp.Tool{
		Name:        "diff_mem_update",
		Description: "更新节点 Header。fields 传 {字段: 值} 批量改字段；summary 传 {old, new, reason} 刷新摘要（实体消失需解释原因）。两者可同时提交。",
		InputSchema: toolSchemas["update"],
	})
	s.addTool(s.srv, &stdmcp.Tool{
		Name:        "diff_mem_lifecycle",
		Description: "节点生命周期状态转换。action: delete（删除，不可逆，有活跃依赖时拒绝）/ archive（归档，冻结并移出默认搜索，可恢复）/ restore（恢复归档节点）。",
		InputSchema: toolSchemas["lifecycle"],
	})
	s.addTool(s.srv, &stdmcp.Tool{
		Name:        "diff_mem_list",
		Description: "列出指定路径下的直接子节点。path 为空或 / 时列出根层级。",
		InputSchema: toolSchemas["list"],
	})
	s.addTool(s.srv, &stdmcp.Tool{
		Name:        "diff_mem_search",
		Description: "搜索节点。支持 tags 精确匹配和 keywords 模糊匹配。",
		InputSchema: toolSchemas["search"],
	})
	s.addTool(s.srv, &stdmcp.Tool{
		Name:        "diff_mem_show",
		Description: "查看节点。不传 window：返回 Header + 内容链接（links）与反向链接（backlinks），用于发现相关记忆。传 window（recent/last_10/last_50/last_100/all）：附带 Body 事件流，先看摘要再决定是否深入。",
		InputSchema: toolSchemas["show"],
	})
}

// Run runs the MCP server on the given transport (stdio).
func (s *Server) Run(ctx context.Context, transport stdmcp.Transport) error {
	return s.srv.Run(ctx, transport)
}

// SSEHandler returns an http.Handler that serves MCP over SSE.
func (s *Server) SSEHandler(endpoint string) http.Handler {
	return stdmcp.NewSSEHandler(func(req *http.Request) *stdmcp.Server {
		return s.srv
	}, &stdmcp.SSEOptions{
		DisableLocalhostProtection: true,
	})
}

func (s *Server) addTool(server *stdmcp.Server, t *stdmcp.Tool) {
	toolName := t.Name
	engineName := toolName[len("diff_mem_"):]

	// Strictness: every tool MUST ship a complete input schema so the model
	// sees the exact parameter contract. A tool without one is a programming
	// error — fail fast at registration time instead of confusing the model.
	if t.InputSchema == nil {
		panic("tool " + toolName + " has no InputSchema")
	}

	s.tools[toolName] = t

	server.AddTool(t, func(ctx context.Context, req *stdmcp.CallToolRequest) (*stdmcp.CallToolResult, error) {
		var args map[string]interface{}
		if req.Params != nil && req.Params.Arguments != nil {
			b, _ := json.Marshal(req.Params.Arguments)
			json.Unmarshal(b, &args)
		}
		if args == nil {
			args = make(map[string]interface{})
		}

		resp := s.engine.Dispatch(engineName, args)
		respBytes, _ := json.Marshal(resp)

		var hasError bool
		var check struct{ Success bool }
		json.Unmarshal(respBytes, &check)
		if !check.Success {
			hasError = true
		}

		return &stdmcp.CallToolResult{
			Content: []stdmcp.Content{&stdmcp.TextContent{Text: string(respBytes)}},
			IsError: hasError,
		}, nil
	})
}

// Serve is the public entry point.
func Serve(ctx context.Context, engine *engine.Engine, transport stdmcp.Transport) error {
	return New(engine).Run(ctx, transport)
}
