# diff-mem

AI Agent 长期记忆系统。LLM 作为策略层输出符号化工具调用，确定性引擎负责执行和约束，防止状态漂移。

## 架构

```
┌──────────┐     MCP / HTTP      ┌──────────┐
│  LLM /   │ ◄─────────────────► │  Engine  │
│  Agent   │                     │          │
└──────────┘                     └────┬─────┘
                                      │
                              ┌───────┼───────┐
                              │       │       │
                        ┌─────┴──┐┌──┴──┐┌──┴────┐
                        │Store  ││Validator│
                        └────────┘└─────┘└────────┘
```

## 核心概念

- **节点 (Node)**: 路径 + Header（轻量索引）+ Body（不可变事件流）
- **状态**: active / archived（二态，可恢复）
- **内容链接**: Body 事件中的 `[[/path]]` 引用，写侧门禁校验、读侧自动发现（links + backlinks）
- **检索**: 逐步导航 + 倒排索引，模型永不面对全量数据

## 快速开始

```bash
# 启动 HTTP 服务器
go run ./cmd/server

# 启动 MCP 服务器（stdio transport）
go run ./cmd/mcp-server

# 内置 CLI Agent
go run ./cmd/agent repl
```

## 目录结构

```
cmd/
  agent/         CLI 终端助手
  mcp-server/    MCP stdio 服务
  server/        HTTP API 服务
internal/
  agent/         Agent 核心 + REPL
  api/           HTTP 路由
  engine/        引擎（调度 + 读写 + 生命周期 + 内容链接门禁）
  mcp/           MCP 协议层（11 个工具）
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

MCP 暴露 7 个工具：

| 工具 | 说明 |
|------|------|
| `diff_mem_create` | 创建新节点（Body 支持 `[[/path]]` 内容链接） |
| `diff_mem_append` | 追加事件（链接目标不存在则拒绝） |
| `diff_mem_update` | 更新 Header：fields 批量改字段 / summary 刷新摘要（含漂移检测），可同时提交 |
| `diff_mem_lifecycle` | 生命周期状态转换：delete（被链接时拒绝）/ archive（被链接时警告）/ restore |
| `diff_mem_list` | 列出子节点 |
| `diff_mem_search` | 搜索 |
| `diff_mem_show` | 查看节点：不传 window 看 Header + links/backlinks；传 window 加载 Body 事件流 |

事务（`diff_mem_exec`）仅通过 HTTP `/tools/exec` 暴露，不占 MCP 工具位。

## 测试

```bash
go test ./...
```
