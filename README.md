# diff-mem

AI Agent 长期记忆系统。LLM 作为策略层输出符号化工具调用，确定性引擎负责执行和约束，防止状态漂移。

## 架构

```
┌──────────┐     MCP (SSE/stdio) / HTTP      ┌──────────┐
│  LLM /   │ ◄─────────────────────────────► │  Engine  │
│  Agent   │                                  │          │
└──────────┘                                  └────┬─────┘
                                                    │
                                            ┌───────┼───────┐
                                            │       │       │
                                      ┌─────┴──┐┌──┴──┐┌──┴────┐
                                      │Store  ││Graph││Validator│
                                      └───────┘└─────┘└────────┘
```

## 核心概念

- **节点 (Node)**: 路径 + Header（轻量索引）+ Body（不可变事件流）
- **状态**: active / archived（二态，可恢复）
- **引用**: depends_on / alternative_to / supersedes / references
- **检索**: 逐步导航 + 倒排索引，模型永不面对全量数据

## 快速开始

```bash
# 启动 SSE MCP 服务器（默认，远程连接）
go run ./cmd/mcp-server
# → diff-mem MCP SSE server at http://0.0.0.0:8787/mcp

# 本地 stdio 模式（给 Claude Desktop / Cursor 等客户端 spawn）
go run ./cmd/mcp-server -stdio

# 启动 HTTP API 服务器（给 CLI agent 用）
go run ./cmd/server

# 内置 CLI Agent
go run ./cmd/agent repl
```

## MCP 连接方式

### SSE 模式（远程，推荐）

启动后监听 `http://0.0.0.0:8787/mcp`，客户端通过标准 SSE MCP 协议连接。

**Claude Desktop** — 配置文件中添加（使用支持 SSE 的 MCP client）：

```json
{
  "mcpServers": {
    "diff-mem": {
      "url": "http://your-server:8787/mcp"
    }
  }
}
```

**编程对接**（使用任意 MCP SDK 的 `SSEClientTransport`）：

```typescript
const transport = new SSEClientTransport(new URL("http://your-server:8787/mcp"));
const client = new Client(...);
await client.connect(transport);
```

### stdio 模式（本地，Claude Desktop / Cursor）

客户端通过配置文件启动本地进程：

```json
{
  "mcpServers": {
    "diff-mem": {
      "command": "diff-mem-mcp",
      "args": ["-stdio", "-data", "./diff-mem-data"]
    }
  }
}
```

## 数据持久化

所有 MCP 服务器（SSE 和 stdio）使用 **BadgerDB** 存储，数据默认写入当前目录下的 `./diff-mem-data`。

```bash
# 自定义数据目录
diff-mem-mcp -data /path/to/my-data
diff-mem-mcp -stdio -data /path/to/my-data
```

`cmd/server`（HTTP API）使用内存存储，仅适合开发测试，重启后数据丢失。

## 目录结构

```
cmd/
  agent/         CLI 终端助手
  mcp-server/    MCP 服务器（SSE + stdio，默认 SSE）
  server/        HTTP API 服务器（仅开发测试用）
internal/
  agent/         Agent 核心 + REPL
  api/           HTTP 路由
  engine/        引擎（调度 + 读写 + 生命周期 + 图操作）
  mcp/           MCP 协议层（7 个工具）
  model/         数据模型
  store/         存储层（内存 + BadgerDB）
  validator/     语义验证
docs/            设计规范文档
```

## 设计原则

1. AI 决定内容，引擎保证安全
2. 软索引可以漂移，硬数据不可变
3. 模型永不面对全量数据
4. 每次写入都可审计

## 工具列表

MCP 暴露 7 个工具，与引擎 dispatch 名一一对应：

| 工具 | 说明 |
|------|------|
| `diff_mem_create` | 创建新节点。path 必须以 /short-term 或 /long-term 开头，中间目录无需预先创建。Body 事件中可用 [[/path]] 链接其他记忆 |
| `diff_mem_append` | 向节点追加事件（仅记录新事实）。事件中可用 [[/path]] 链接其他记忆 |
| `diff_mem_update` | 更新节点 Header。fields 传 {字段: 值} 批量改字段；summary 传 {old, new, reason} 刷新摘要。两者可同时提交 |
| `diff_mem_lifecycle` | 节点生命周期状态转换。action: delete / archive / restore |
| `diff_mem_list` | 列出指定路径下的直接子节点。path 为空或 / 时列出根层级 |
| `diff_mem_search` | 搜索节点。支持 tags 精确匹配和 keywords 模糊匹配 |
| `diff_mem_show` | 查看节点。不传 window 返回 Header + 内容链接与反向链接；传 window 附带 Body 事件流 |

`diff_mem_exec`（原子事务）仅通过 HTTP `/tools/exec` 暴露，不占 MCP 工具位。

## 测试

```bash
go test ./...
```
