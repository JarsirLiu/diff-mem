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
                        │Store  ││Graph││Validator│
                        └────────┘└─────┘└────────┘
```

## 核心概念

- **节点 (Node)**: 路径 + Header（轻量索引）+ Body（不可变事件流）
- **状态**: active / archived（二态，可恢复）
- **引用**: depends_on / alternative_to / supersedes / references
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
  engine/        引擎（调度 + 读写 + 生命周期 + 图操作）
  graph/         图算法（BFS / 环检测 / 并查集 / 实体提取）
  mcp/           MCP 协议层（14 个工具）
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

| 工具 | 说明 |
|------|------|
| `diff_mem_create` | 创建新节点 |
| `diff_mem_append` | 追加事件 |
| `diff_mem_update_field` | 更新字段 |
| `diff_mem_update_summary` | 更新摘要（含漂移检测） |
| `diff_mem_delete` | 永久删除 |
| `diff_mem_archive` | 归档 |
| `diff_mem_restore` | 恢复 |
| `diff_mem_link` | 建立引用 |
| `diff_mem_unlink` | 解除引用 |
| `diff_mem_list` | 列出子节点 |
| `diff_mem_search` | 搜索 |
| `diff_mem_show` | 查看节点 |
| `diff_mem_deep_load` | 加载事件流 |
| `diff_mem_exec` | 原子事务 |

## 测试

```bash
go test ./...
```
