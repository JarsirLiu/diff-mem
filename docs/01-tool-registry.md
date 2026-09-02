# 01. Tool Registry

> Diff-Mem 暴露给 LLM 调用的完整工具清单

---

## 一、核心设计

Diff-Mem 是一个 Tool Provider。每个记忆操作都是一个独立 Tool，通过标准 Tool-Calling 协议被 Agent 调用。

**关键约束**：Tool 调用的参数由框架层做 Schema 校验。不合法的调用**不执行、零副作用**，Agent 收到错误后自行重试。

---

## 二、读取类 Tool

### 2.1 `diff_mem_list`

**用途**：逐层浏览记忆树结构。类似 `ls` 命令。

```json
{
  "name": "diff_mem_list",
  "description": "列出指定路径下的直接子节点。path 为空或 '/' 时列出根层级。每次只返回一层，不会递归展开。如果子节点超过 50 个，引擎自动按命名前缀聚合成分组返回。",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "要列出的路径。空字符串或 '/' 表示根目录。"
      }
    },
    "required": ["path"]
  }
}
```

**返回**：
```json
{
  "path": "/Projects",
  "children": [
    {"path": "/Projects/Alpha", "type": "node"},
    {"path": "/Projects/Beta",  "type": "node"}
  ],
  "has_more": false
}
```

**引擎规则**：
- 如果 path 不存在 → 返回 404，附带最近存在路径的 did-you-mean 建议
- 如果子节点 > 50 → 按前缀分组返回（如 `alpha-*`, `beta-*`），设置 `has_more: true`
- 分组后模型可以用 `diff_mem_search` 在分组内进一步缩小

---

### 2.2 `diff_mem_search`

**用途**：按标签、路径子串或摘要关键词搜索节点。

```json
{
  "name": "diff_mem_search",
  "description": "在记忆树中搜索匹配的节点。优先使用 tags 搜索（精确、快），模糊匹配时使用 keywords。返回最多 limit 个结果。",
  "parameters": {
    "type": "object",
    "properties": {
      "tags": {
        "type": "array",
        "items": {"type": "string"},
        "description": "按标签精确匹配。传入多个标签时取交集。"
      },
      "keywords": {
        "type": "string",
        "description": "在 path 和 summary 中做关键词模糊匹配。"
      },
      "limit": {
        "type": "integer",
        "default": 10,
        "maximum": 50,
        "description": "最大返回数量，默认 10，最大 50。"
      }
    },
    "required": []
  }
}
```

**返回**：
```json
{
  "results": [
    {"path": "/Projects/Alpha/Backend", "tags": ["backend", "api"], "summary": "API 服务端开发"},
    {"path": "/Projects/Alpha/Frontend", "tags": ["frontend", "ui"], "summary": "前端界面开发"}
  ]
}
```

**引擎规则**：
- tags 匹配是精确的倒排索引查询
- keywords 匹配同时扫描 path 层级名和 summary 字段
- 两个参数可同时传入，结果取并集后按 relevance 排序
- limit 上限 50，硬约束，模型不能绕过

---

### 2.3 `diff_mem_show`

**用途**：查看节点。不传 window 返回 Header + 内容链接（轻）；传 window 附带 Body 事件流（重）。AI 的自然阅读路径"先看摘要再决定要不要看正文"是同一个工具的两档深度。

```json
{
  "name": "diff_mem_show",
  "description": "查看指定节点。不传 window：返回完整 Header（标题、状态、标签、摘要、字段等）以及 Body 中的内容链接（links）与反向链接（backlinks），用于发现相关记忆。传 window：附带 Body 事件流，用于查看历史细节。单节点 Body 可能很长，建议先看不带 window 的轻量结果再决定。",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "节点的完整路径。"
      },
      "window": {
        "type": "string",
        "enum": ["recent", "last_10", "last_50", "last_100", "all"],
        "description": "可选。传入时附带 Body 事件流：recent=最近 5 条，last_N=最近 N 条，all=全部。"
      }
    },
    "required": ["path"]
  }
}
```

**返回（不传 window）**：
```json
{
  "header": {
    "path": "/Projects/Alpha/Backend",
    "title": "Alpha 项目后端",
    "status": "active",
    "tags": ["backend", "api", "in-progress"],
    "summary": "负责 Alpha 项目的 API 设计与服务端开发，当前处于开发阶段。",
    "fields": ["owner", "deadline", "milestones"],
    "updated": "2026-09-01T18:30:00Z",
    "event_count": 47
  },
  "links": ["/Decisions/认证方案"],
  "backlinks": ["/Projects/Alpha"]
}
```

**返回（传 window）**：在上述基础上附带事件流：
```json
{
  "path": "/Projects/Alpha/Backend",
  "events": [
    {"ts": "2026-08-28T10:00:00Z", "type": "create", "content": "创建后端模块，负责人张三"},
    {"ts": "2026-08-30T14:00:00Z", "type": "update", "content": "完成 API 设计，进入开发"}
  ],
  "total": 47,
  "has_more": true,
  "links": ["/Decisions/认证方案"]
}
```

**引擎规则**：
- `links`：该节点 Body 中所有 `[[/path]]` 内容链接指向的节点
- `backlinks`：其他活跃节点 Body 中链接到本节点的反向链接
- window 版本 `all` 窗口也有上限：单次最多返回 500 条事件；超过返回 `has_more: true`
- AI 看到一条记忆即可发现相关记忆，自行决定是否传 window 深入

---

## 三、写入类 Tool

### 3.1 `diff_mem_create`

**用途**：创建新节点。

```json
{
  "name": "diff_mem_create",
  "description": "在记忆树中创建一个新节点。path 由你决定，引擎会做规范化处理（转小写、去重、去除特殊字符）。如果 path 已存在，操作失败，你需要先 diff_mem_show 查看已有内容再决定如何操作。",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "新节点的完整路径，如 /Projects/Alpha/Backend。引擎会做规范化。"
      },
      "title": {
        "type": "string",
        "description": "节点的人类可读标题。"
      },
      "summary": {
        "type": "string",
        "description": "节点的一句话摘要描述。用于检索匹配。"
      },
      "tags": {
        "type": "array",
        "items": {"type": "string"},
        "description": "标签列表，用于倒排索引。"
      },
      "initial_events": {
        "type": "array",
        "items": {"type": "string"},
        "description": "初始事件列表。第一条事件会记录节点创建的原因和内容。"
      },
      "reason": {
        "type": "string",
        "description": "创建此节点的原因，用于审计追溯。"
      }
    },
    "required": ["path", "title", "summary", "reason"]
  }
}
```

---

### 3.2 `diff_mem_append`

**用途**：向节点的 Body 追加事件。

```json
{
  "name": "diff_mem_append",
  "description": "向指定节点的 Body 追加一条事件记录。事件按时间顺序追加，不可修改或删除已有事件。这是记录历史变化的主要方式。",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "节点的完整路径。"
      },
      "event": {
        "type": "string",
        "description": "要追加的事件内容。"
      },
      "reason": {
        "type": "string",
        "description": "追加此事件的原因。"
      }
    },
    "required": ["path", "event", "reason"]
  }
}
```

---

### 3.3 `diff_mem_update`

**用途**：更新节点 Header。fields 批量改字段和/或 summary 刷新摘要，一次调用完成。

```json
{
  "name": "diff_mem_update",
  "description": "更新节点 Header。fields 传 {字段名: 新值} 批量修改字段（字段不存在则自动注册）；summary 传 {old, new, reason} 刷新摘要——引擎对比新旧摘要，关键实体消失时需在 reason 中解释。两者可同时提交。",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "节点的完整路径。"
      },
      "fields": {
        "type": "object",
        "description": "{字段名: 新值} 映射，批量更新字段。reason 字段作为整批字段变更的审计原因。"
      },
      "summary": {
        "type": "object",
        "properties": {
          "old": {"type": "string", "description": "当前摘要（从 diff_mem_show 获取，供引擎做新旧对比）。"},
          "new": {"type": "string", "description": "新摘要。应保留旧摘要中的关键事实，仅更新变化部分。"},
          "reason": {"type": "string", "description": "更新摘要的原因。"}
        },
        "description": "摘要刷新。实体消失时 reason 需解释，否则拒绝。"
      },
      "reason": {
        "type": "string",
        "description": "字段变更的审计原因（配合 fields 使用）。"
      }
    },
    "required": ["path"]
  }
}
```

**引擎规则**：
- fields 和 summary 至少提供一个，否则 `VALIDATION_FAILED`
- 字段名自动注册：字段不存在时引擎创建它并记录初始值
- `status` 和 `summary` 是**受保护字段**，有特殊校验（见 04-state-drift-defense.md）
- summary 漂移检测：引擎对比 old/new 摘要，发现关键实体消失（人名/数字/日期/专有名词）时检查 reason
  - reason 非空且 ≥10 字符 → 放行，同时自动向 Body 追加审计事件
  - reason 为空或 <10 字符 → 拒绝（`SUMMARY_DRIFT_DETECTED`），返回消失实体列表
- summary 在**任何写入前预校验**（mismatch + drift），避免 fields 已生效而 summary 失败的部分写入
- 引擎不对"消失的实体是否合理"做判断——只要求 AI 给出解释

---

### 3.4 `diff_mem_lifecycle`

**用途**：节点生命周期状态转换。active → deleted（delete）、active ⇄ archived（archive/restore）三条边共用一个工具入口。

```json
{
  "name": "diff_mem_lifecycle",
  "description": "节点生命周期状态转换。action=delete：删除节点及所有子节点，不可逆，被其他活跃节点链接时拒绝。action=archive：归档节点，冻结（不可追加/修改）并移出默认搜索，可恢复；被链接时返回警告及处置指引。action=restore：恢复已归档节点为活跃状态。",
  "parameters": {
    "type": "object",
    "properties": {
      "action": {
        "type": "string",
        "enum": ["delete", "archive", "restore"],
        "description": "状态转换类型。"
      },
      "path": {
        "type": "string",
        "description": "目标节点路径。"
      },
      "reason": {
        "type": "string",
        "description": "操作原因，用于审计。"
      }
    },
    "required": ["action", "path", "reason"]
  }
}
```

**引擎规则**：
- **delete**（不可逆）：被其他活跃节点 Body 链接 → 拒绝（`LINKED_BY_OTHERS`），列出引用方，AI 需先修正链接；有子节点时一并删除
- **archive**（可逆）：节点冻结，从默认搜索排除；被链接 → 警告不阻止，warning 附三选一处置指引：1) 改指到新的承接节点；2) 有意引用历史快照则保留（归档节点仍是有效链接目标）；3) 更新引用方 Body 移除链接
- **restore**：只有 archived 节点能恢复；恢复时检查出站 `[[链接]]`，目标已删除/归档 → 警告悬空链接，AI 可立即修复
- 操作自动向 Body 追加审计事件（archived/restored）

---

### 3.5 内容链接门禁（作用于 create / append）

> Diff-Mem 的节点间关联通过 **内容链接** 表达：在 Body 事件文本中写 `[[/path/to/node]]`。
> 链接属于内容而非元数据——没有独立的 link/unlink 工具。

**语法**：

```
事件文本示例：
"后端模块依赖 [[/Decisions/认证方案]]，负责人张三"
```

**引擎规则（写侧门禁）**：

- create / append 时引擎用正则扫描事件文本中的所有 `[[...]]` 引用
- 链接目标必须满足其一，否则拒绝写入（零副作用）：
  - 目标路径已存在
  - 目标是节点自身或其祖先路径（祖先在 create 时自动创建）
- 目标不存在 → 返回 `LINK_TARGET_NOT_FOUND`，附带 did-you-mean 建议（最多 3 个相似路径），AI 修正后重试
- 链接不是合法路径（不以 `/` 开头）→ 返回 `LINK_TARGET_INVALID`

**读侧发现**：

- `diff_mem_show` 返回 `links`（本节点指向谁）与 `backlinks`（谁链接到本节点）
- `diff_mem_show(window)` 响应附带 `links`（全节点内容链接汇总）

**生命周期门禁**（`diff_mem_lifecycle`）：

- `action=delete`：目标被其他活跃节点 Body 链接 → 拒绝（`LINKED_BY_OTHERS`），列出引用方
- `action=archive`：被链接 → 警告（附三选一处置指引），不阻止

---

### 3.6 `diff_mem_exec`（事务，仅 HTTP）

**用途**：事务执行。

```json
{
  "name": "diff_mem_exec",
  "description": "原子性执行多个记忆操作。所有操作全部成功则提交，任意一个失败则全部回滚。用于需要保证一致性的批量操作，比如同时创建节点并追加事件。",
  "parameters": {
    "type": "object",
    "properties": {
      "operations": {
        "type": "array",
        "items": {
          "type": "object",
          "description": "每个操作是一个 JSON 对象，包含 op 字段标识操作类型，以及对应的参数。支持的操作：CREATE, APPEND, UPDATE_FIELD, DELETE, ARCHIVE, RESTORE。"
        },
        "minItems": 1,
        "maxItems": 20,
        "description": "操作列表，最多 20 个。"
      }
    },
    "required": ["operations"]
  }
}
```

**引擎规则**：
- 按顺序执行操作
- 任意操作失败 → 全部回滚
- 最大 20 个操作，防止单次事务过大
- 不支持 list/show/search 在事务中（只读操作无事务需求）

---

## 四、Tool 调用流程图

```
用户 Query
    ↓
Agent 决策：我需要操作记忆
    ↓
┌─────────────────────────────────┐
│  选择合适 Tool                   │
│  list? search? create? append?  │
└────────────┬────────────────────┘
             ↓
┌─────────────────────────────────┐
│  Tool-Calling 通道              │
│  框架层做 Schema 校验            │
│  类型错误 → 拒绝 + 错误信息     │
└────────────┬────────────────────┘
             ↓
┌─────────────────────────────────┐
│  引擎层做语义校验                │
│  path 存在性 / 字段合法性 / ... │
│  校验失败 → 拒绝 + 具体原因     │
└────────────┬────────────────────┘
             ↓
┌─────────────────────────────────┐
│  执行                           │
│  写入磁盘 / 返回结果            │
└─────────────────────────────────┘
```

## 五、返回约定

所有 Tool 返回统一的响应格式：

```json
{
  "success": true,
  "result": { ... },
  "warnings": [ ... ],
  "cost": {
    "read_tokens": 150,
    "write_ops": 1
  }
}
```

失败时：

```json
{
  "success": false,
  "error": {
    "code": "PATH_NOT_FOUND",
    "message": "路径 /Projects/Gamma 不存在",
    "suggestion": "你可能想找 /Projects/Alpha 或 /Projects/Beta"
  }
}
```
