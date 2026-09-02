// Package mcp provides the MCP server that wraps the Diff-Mem engine.
package mcp

import (
	"context"
	"encoding/json"

	stdmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/diff-mem/diff-mem/internal/engine"
)

type Server struct {
	engine *engine.Engine
}

func New(e *engine.Engine) *Server {
	return &Server{engine: e}
}

func (s *Server) Run(ctx context.Context, transport stdmcp.Transport) error {
	server := stdmcp.NewServer(&stdmcp.Implementation{Name: "diff-mem", Version: "0.1.0"}, nil)

	s.addTool(server, &stdmcp.Tool{
		Name:        "diff_mem_create",
		Description: "在记忆树中创建新节点。path 由你决定，引擎自动创建缺失的父路径。Body 事件中可用 [[/path]] 链接其他记忆，链接目标必须已存在。",
	})
	s.addTool(server, &stdmcp.Tool{
		Name:        "diff_mem_append",
		Description: "向节点追加事件。仅出现值得记录的新事实时调用。事件中可用 [[/path]] 链接其他记忆，链接目标必须已存在，否则写入被拒绝。",
	})
	s.addTool(server, &stdmcp.Tool{
		Name:        "diff_mem_update",
		Description: "更新节点 Header。fields 传 {字段: 值} 批量改字段；summary 传 {old, new, reason} 刷新摘要（实体消失需解释原因）。两者可同时提交。",
	})
	s.addTool(server, &stdmcp.Tool{
		Name:        "diff_mem_lifecycle",
		Description: "节点生命周期状态转换。action: delete（删除，不可逆，有活跃依赖时拒绝）/ archive（归档，冻结并移出默认搜索，可恢复）/ restore（恢复归档节点）。",
	})
	s.addTool(server, &stdmcp.Tool{
		Name:        "diff_mem_list",
		Description: "列出指定路径下的直接子节点。path 为空或 / 时列出根层级。",
	})
	s.addTool(server, &stdmcp.Tool{
		Name:        "diff_mem_search",
		Description: "搜索节点。支持 tags 精确匹配和 keywords 模糊匹配。",
	})
	s.addTool(server, &stdmcp.Tool{
		Name:        "diff_mem_show",
		Description: "查看节点。不传 window：返回 Header + 内容链接（links）与反向链接（backlinks），用于发现相关记忆。传 window（recent/last_10/last_50/last_100/all）：附带 Body 事件流，先看摘要再决定是否深入。",
	})

	return server.Run(ctx, transport)
}

func (s *Server) addTool(server *stdmcp.Server, t *stdmcp.Tool) {
	toolName := t.Name
	toolEngineName := toolName[len("diff_mem_"):]

	server.AddTool(t, func(ctx context.Context, req *stdmcp.CallToolRequest) (*stdmcp.CallToolResult, error) {
		// Extract arguments
		var args map[string]interface{}
		if req.Params != nil && req.Params.Arguments != nil {
			b, _ := json.Marshal(req.Params.Arguments)
			json.Unmarshal(b, &args)
		}
		if args == nil {
			args = make(map[string]interface{})
		}

		resp := s.engine.Dispatch(toolEngineName, args)
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
