// Package mcp provides the MCP server that wraps the Diff-Mem engine.
// Supports both stdio transport (for local client spawning) and SSE transport
// (for remote HTTP connections).
package mcp

import (
	"context"
	"encoding/json"
	"net/http"

	stdmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/diff-mem/diff-mem/internal/engine"
)

type Server struct {
	srv    *stdmcp.Server
	engine *engine.Engine
}

func New(e *engine.Engine) *Server {
	s := &Server{
		srv:    stdmcp.NewServer(&stdmcp.Implementation{Name: "diff-mem", Version: "0.1.0"}, nil),
		engine: e,
	}
	s.registerTools()
	return s
}

func (s *Server) registerTools() {
	s.addTool(s.srv, &stdmcp.Tool{
		Name:        "diff_mem_create",
		Description: "在记忆树中创建新节点。path 由你决定，引擎自动创建缺失的父路径。",
	})
	s.addTool(s.srv, &stdmcp.Tool{
		Name:        "diff_mem_append",
		Description: "向节点追加事件。仅出现值得记录的新事实时调用。",
	})
	s.addTool(s.srv, &stdmcp.Tool{
		Name:        "diff_mem_update_field",
		Description: "更新节点 Header 中的字段值。",
	})
	s.addTool(s.srv, &stdmcp.Tool{
		Name:        "diff_mem_update_summary",
		Description: "刷新节点摘要。提交 old_summary 做新旧对比，有实体消失时需解释原因。",
	})
	s.addTool(s.srv, &stdmcp.Tool{
		Name:        "diff_mem_delete",
		Description: "删除节点及其所有子节点。不可逆，有活跃依赖时拒绝。建议先 archive。",
	})
	s.addTool(s.srv, &stdmcp.Tool{
		Name:        "diff_mem_archive",
		Description: "归档节点。归档后冻结，从默认搜索中排除。可恢复。",
	})
	s.addTool(s.srv, &stdmcp.Tool{
		Name:        "diff_mem_restore",
		Description: "恢复已归档节点为活跃状态。",
	})
	s.addTool(s.srv, &stdmcp.Tool{
		Name:        "diff_mem_link",
		Description: "在两个节点间建立引用关系。type: depends_on/alternative_to/supersedes/references。",
	})
	s.addTool(s.srv, &stdmcp.Tool{
		Name:        "diff_mem_unlink",
		Description: "移除两个节点间的引用关系。",
	})
	s.addTool(s.srv, &stdmcp.Tool{
		Name:        "diff_mem_list",
		Description: "列出指定路径下的直接子节点。path 为空或 / 时列出根层级。",
	})
	s.addTool(s.srv, &stdmcp.Tool{
		Name:        "diff_mem_search",
		Description: "搜索节点。支持 tags 精确匹配和 keywords 模糊匹配。",
	})
	s.addTool(s.srv, &stdmcp.Tool{
		Name:        "diff_mem_show",
		Description: "获取节点完整 Header 信息。",
	})
	s.addTool(s.srv, &stdmcp.Tool{
		Name:        "diff_mem_deep_load",
		Description: "加载节点 Body 事件流。window: recent/last_10/last_50/last_100/all。",
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
	// Raw AddTool requires InputSchema.
	if t.InputSchema == nil {
		t.InputSchema = json.RawMessage(`{"type":"object"}`)
	}

	server.AddTool(t, func(ctx context.Context, req *stdmcp.CallToolRequest) (*stdmcp.CallToolResult, error) {
		var rawArgs map[string]interface{}
		if req.Params != nil && req.Params.Arguments != nil {
			b, _ := json.Marshal(req.Params.Arguments)
			json.Unmarshal(b, &rawArgs)
		}
		if rawArgs == nil {
			rawArgs = make(map[string]interface{})
		}

		// Map MCP tool name + params → engine dispatch name + params.
		mappedName, mappedArgs := translateArgs(engineName, rawArgs)

		resp := s.engine.Dispatch(mappedName, mappedArgs)
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

// translateArgs maps MCP tool calls to the engine's internal dispatch format.
// Engine supports: create, append, update, lifecycle, list, search, show.
func translateArgs(tool string, args map[string]interface{}) (string, map[string]interface{}) {
	switch tool {
	case "create":
		return "create", args

	case "append":
		return "append", args

	case "update_field":
		field, _ := args["field"].(string)
		value, _ := args["value"].(string)
		delete(args, "field")
		delete(args, "value")
		args["fields"] = map[string]interface{}{field: value}
		return "update", args

	case "update_summary":
		oldSummary, _ := args["old_summary"].(string)
		newSummary, _ := args["new_summary"].(string)
		reason, _ := args["reason"].(string)
		delete(args, "old_summary")
		delete(args, "new_summary")
		args["summary"] = map[string]interface{}{
			"old":    oldSummary,
			"new":    newSummary,
			"reason": reason,
		}
		return "update", args

	case "delete":
		args["action"] = "delete"
		return "lifecycle", args

	case "archive":
		args["action"] = "archive"
		return "lifecycle", args

	case "restore":
		args["action"] = "restore"
		return "lifecycle", args

	case "link":
		from, _ := args["from"].(string)
		to, _ := args["to"].(string)
		edgeType, _ := args["type"].(string)
		reason, _ := args["reason"].(string)
		delete(args, "to")
		delete(args, "type")
		args["path"] = from
		args["event"] = "[" + edgeType + "] [[" + to + "]]"
		args["reason"] = reason
		return "append", args

	case "unlink":
		from, _ := args["from"].(string)
		to, _ := args["to"].(string)
		delete(args, "to")
		args["path"] = from
		args["event"] = "unlink [[" + to + "]]"
		args["reason"] = "解除引用"
		return "append", args

	case "list":
		return "list", args

	case "search":
		return "search", args

	case "show":
		return "show", args

	case "deep_load":
		path, _ := args["path"].(string)
		window, _ := args["window"].(string)
		return "show", map[string]interface{}{"path": path, "window": window}

	default:
		return tool, args
	}
}

// Serve is the public entry point.
func Serve(ctx context.Context, engine *engine.Engine, transport stdmcp.Transport) error {
	return New(engine).Run(ctx, transport)
}
