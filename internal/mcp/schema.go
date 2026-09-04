// Package mcp — JSON Schemas for tool inputs, aligned with docs/01-tool-registry.md.
// These schemas are sent to MCP clients via tools/list so the model sees
// the exact parameter contract before calling.
package mcp

import "encoding/json"

const (
	schemaCreate = `{
	  "type": "object",
	  "properties": {
	    "path": {
	      "type": "string",
	      "description": "新节点的完整路径，如 /Projects/Alpha/Backend。引擎自动创建缺失的父路径并做规范化（转小写、去特殊字符）。"
	    },
	    "title": {
	      "type": "string",
	      "description": "节点的人类可读标题。"
	    },
	    "summary": {
	      "type": "string",
	      "description": "节点的一句话摘要，用于检索匹配。应包含关键实体（人名/数字/日期/专有名词）。"
	    },
	    "tags": {
	      "type": "array",
	      "items": { "type": "string" },
	      "description": "标签列表，用于倒排索引精确搜索。"
	    },
	    "initial_events": {
	      "type": "array",
	      "items": { "type": "string" },
	      "description": "初始事件列表。第一条事件会记录节点创建的原因和内容。事件文本中可用 [[/path]] 链接其他节点。"
	    },
	    "reason": {
	      "type": "string",
	      "description": "创建此节点的原因，用于审计追溯。"
	    }
	  },
	  "required": ["path", "title", "summary", "reason"],
	  "additionalProperties": false
	}`

	schemaAppend = `{
	  "type": "object",
	  "properties": {
	    "path": {
	      "type": "string",
	      "description": "目标节点的完整路径。"
	    },
	    "event": {
	      "type": "string",
	      "description": "要追加的事件内容，按时间顺序不可修改或删除。事件文本中可用 [[/path]] 链接其他节点，链接目标必须已存在。"
	    },
	    "reason": {
	      "type": "string",
	      "description": "追加此事件的原因——必须说明为什么值得记，写不出 reason 就说明不该记。"
	    }
	  },
	  "required": ["path", "event", "reason"],
	  "additionalProperties": false
	}`

	schemaUpdate = `{
	  "type": "object",
	  "properties": {
	    "path": {
	      "type": "string",
	      "description": "目标节点的完整路径。"
	    },
	    "fields": {
	      "type": "object",
	      "additionalProperties": { "type": "string" },
	      "description": "{字段名: 新值} 映射，批量更新字段。字段不存在时自动注册。外层 reason 作为整批字段变更的审计原因。"
	    },
	    "summary": {
	      "type": "object",
	      "properties": {
	        "old": {
	          "type": "string",
	          "description": "当前摘要（从 diff_mem_show 获取，供引擎做新旧对比）。"
	        },
	        "new": {
	          "type": "string",
	          "description": "新摘要。应保留旧摘要中的关键事实，仅更新变化部分。"
	        },
	        "reason": {
	          "type": "string",
	          "description": "更新摘要的原因。关键实体消失时必须解释，否则拒绝。"
	        }
	      },
	      "required": ["old", "new"],
	      "additionalProperties": false,
	      "description": "摘要刷新。引擎对比 old/new，发现关键实体（人名/数字/日期/专有名词）消失时检查 reason，缺失或过短则拒绝。"
	    },
	    "reason": {
	      "type": "string",
	      "description": "字段变更的审计原因（配合 fields 使用）。"
	    }
	  },
	  "required": ["path"],
	  "additionalProperties": false
	}`

	schemaLifecycle = `{
	  "type": "object",
	  "properties": {
	    "action": {
	      "type": "string",
	      "enum": ["delete", "archive", "restore"],
	      "description": "delete：删除节点及所有子节点，不可逆，被其他活跃节点链接时拒绝。archive：归档节点，冻结并移出默认搜索，可恢复，被链接时返回警告。restore：恢复已归档节点。"
	    },
	    "path": {
	      "type": "string",
	      "description": "目标节点的完整路径。"
	    },
	    "reason": {
	      "type": "string",
	      "description": "操作原因，用于审计。"
	    }
	  },
	  "required": ["action", "path", "reason"],
	  "additionalProperties": false
	}`

	schemaList = `{
	  "type": "object",
	  "properties": {
	    "path": {
	      "type": "string",
	      "description": "要列出的路径。空字符串或 / 表示根层级。每次只返回一层，不会递归展开。子节点超过 50 个时自动按前缀聚合分组并置 has_more=true。"
	    },
	    "include_archived": {
	      "type": "boolean",
	      "default": false,
	      "description": "是否包含已归档节点。默认排除。"
	    }
	  },
	  "required": ["path"],
	  "additionalProperties": false
	}`

	schemaSearch = `{
	  "type": "object",
	  "properties": {
	    "tags": {
	      "type": "array",
	      "items": { "type": "string" },
	      "description": "按标签精确匹配（倒排索引，快）。传入多个标签时取交集。"
	    },
	    "keywords": {
	      "type": "string",
	      "description": "在 path 层级名和 summary 中做关键词模糊匹配。"
	    },
	    "limit": {
	      "type": "integer",
	      "default": 10,
	      "maximum": 50,
	      "description": "最大返回数量，默认 10，硬上限 50。"
	    }
	  },
	  "additionalProperties": false,
	  "description": "tags 与 keywords 可同时传入，结果取并集后按相关性排序。"
	}`

	schemaShow = `{
	  "type": "object",
	  "properties": {
	    "path": {
	      "type": "string",
	      "description": "节点的完整路径。"
	    },
	    "window": {
	      "type": "string",
	      "enum": ["recent", "last_10", "last_50", "last_100", "all"],
	      "description": "可选。传入时附带 Body 事件流：recent=最近 5 条，last_N=最近 N 条，all=全部（单次封顶 500 条，超过置 has_more=true）。不传则返回轻量结果：Header + links/backlinks。建议先看不带 window 的轻量结果再决定是否深入。"
	    }
	  },
	  "required": ["path"],
	  "additionalProperties": false
	}`
)

// toolSchemas maps MCP tool names (without the diff_mem_ prefix) to their input schemas.
var toolSchemas = map[string]json.RawMessage{
	"create":    json.RawMessage(schemaCreate),
	"append":    json.RawMessage(schemaAppend),
	"update":    json.RawMessage(schemaUpdate),
	"lifecycle": json.RawMessage(schemaLifecycle),
	"list":      json.RawMessage(schemaList),
	"search":    json.RawMessage(schemaSearch),
	"show":      json.RawMessage(schemaShow),
}
