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

**用途**：加载单个节点的完整 Header（不含 Body 事件流）。

```json
{
  "name": "diff_mem_show",
  "description": "获取指定节点的完整 Header 信息，包括标题、状态、标签、摘要、字段、更新时间、事件计数等。不包含 Body 事件详情。",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "节点的完整路径。"
      }
    },
    "required": ["path"]
  }
}
```

**返回**：
```json
{
  "path": "/Projects/Alpha/Backend",
  "title": "Alpha 项目后端",
  "status": "active",
  "tags": ["backend", "api", "in-progress"],
  "summary": "负责 Alpha 项目的 API 设计与服务端开发，当前处于开发阶段。",
  "fields": ["owner", "deadline", "milestones"],
  "updated": "2026-09-01T18:30:00Z",
  "event_count": 47
}
```

---

### 2.4 `diff_mem_deep_load`

**用途**：加载节点的 Body 事件流。这是唯一能获取历史细节的操作。

```json
{
  "name": "diff_mem_deep_load",
  "description": "加载指定节点的 Body 事件流（时间线）。支持分页和范围查询。用于需要查看历史细节的场景。注意：单节点 Body 可能很长，建议按需加载。",
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
        "description": "加载范围。recent=最近 5 条，last_N=最近 N 条，all=全部。"
      }
    },
    "required": ["path", "window"]
  }
}
```

**返回**：
```json
{
  "path": "/Projects/Alpha/Backend",
  "events": [
    {"ts": "2026-08-28T10:00:00Z", "type": "create", "content": "创建后端模块，负责人张三"},
    {"ts": "2026-08-30T14:00:00Z", "type": "update", "content": "完成 API 设计，进入开发"}
  ],
  "total": 47,
  "has_more": true
}
```

**引擎规则**：
- `all` 窗口也有上限：单次最多返回 500 条事件
- 超过 500 条时返回 `has_more: true`，模型需用 `offset` 分页（Phase 2 扩展）
- 默认 `recent`，鼓励模型按需加载

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

### 3.3 `diff_mem_update_field`

**用途**：修改 Header 中的某个字段值。

```json
{
  "name": "diff_mem_update_field",
  "description": "更新节点 Header 中的指定字段值。field 必须是该节点已注册过的字段名（通过 diff_mem_show 可查看）。如果需要新增字段，field 可以是新名字，引擎会自动注册。value 支持字符串类型。",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "节点的完整路径。"
      },
      "field": {
        "type": "string",
        "description": "要更新的字段名。"
      },
      "value": {
        "type": "string",
        "description": "字段的新值。"
      },
      "reason": {
        "type": "string",
        "description": "更新原因。"
      }
    },
    "required": ["path", "field", "value", "reason"]
  }
}
```

**引擎规则**：
- 字段名自动注册：如果 field 不存在，引擎创建它并记录初始值
- `status` 和 `summary` 是**受保护字段**，有特殊校验（见 04-state-drift-defense.md）

---

### 3.4 `diff_mem_update_summary`

**用途**：显式刷新节点的 summary。

> **注意**：这是独立 tool，不是 `diff_mem_update_field(path, "summary", ...)`。目的是让引擎对 summary 的更新施加特殊控制。

```json
{
  "name": "diff_mem_update_summary",
  "description": "刷新节点的摘要描述。仅在节点内容发生显著变化时使用。引擎会做新旧摘要对比，保留旧摘要的关键事实，防止信息丢失。不要频繁调用此 tool。",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "节点的完整路径。"
      },
      "old_summary": {
        "type": "string",
        "description": "当前摘要内容（供引擎做新旧对比，防止丢失关键事实）。从 diff_mem_show 获取。"
      },
      "new_summary": {
        "type": "string",
        "description": "新的摘要内容。应保留旧摘要中的关键事实，仅更新变化部分。"
      },
      "reason": {
        "type": "string",
        "description": "更新摘要的原因。"
      }
    },
    "required": ["path", "old_summary", "new_summary", "reason"]
  }
}
```

**引擎规则（关键）**：
- 引擎接收 old_summary 和 new_summary，做实体抽取对比
- 如果发现旧摘要中有关键实体消失（人名/数字/日期/专有名词），检查 reason 字段
  - reason 非空且 ≥10 字符 → 放行，同时自动向 Body 追加审计事件记录消失的实体
  - reason 为空或 <10 字符 → 拒绝，返回消失实体列表
- 引擎不对"消失的实体是否合理"做判断——只要求 AI 给出解释
- 这个设计保证了 summary 的迭代是**增量式的**，历史变更可追溯

---

### 3.5 `diff_mem_delete`

**用途**：删除节点。

```json
{
  "name": "diff_mem_delete",
  "description": "删除指定节点及其所有子节点。此操作不可逆。删除前建议先 diff_mem_archive 进行归档备份。",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "要删除的节点路径。"
      },
      "reason": {
        "type": "string",
        "description": "删除原因。"
      }
    },
    "required": ["path", "reason"]
  }
}
```

**引擎规则**：
- 归档后节点永久冻结，只能通过 `diff_mem_restore` 恢复
- 有活跃节点引用该节点时，返回警告但不阻止
- 自动向 Body 追加归档事件（时间戳 + reason）

---

### 3.7 `diff_mem_restore`

**用途**：恢复已归档节点。

```json
{
  "name": "diff_mem_restore",
  "description": "恢复已归档节点为活跃状态。恢复后节点可正常读写，重新出现在搜索结果中。归档不是永久的。",
  "parameters": {
    "type": "object",
    "properties": {
      "path": {
        "type": "string",
        "description": "要恢复的节点路径。"
      },
      "reason": {
        "type": "string",
        "description": "恢复原因。"
      }
    },
    "required": ["path", "reason"]
  }
}
```

**引擎规则**：
- 只有 archived 状态的节点才能恢复
- 恢复后 Header 和 Body 解冻
- 自动向 Body 追加恢复事件

---

### 3.8 `diff_mem_link`

**用途**：建立节点间的引用关系。

```json
{
  "name": "diff_mem_link",
  "description": "在两个记忆节点间建立引用关系，记录依赖、替代、引用等关联。用于建立记忆间的结构连接。",
  "parameters": {
    "type": "object",
    "properties": {
      "from": {
        "type": "string",
        "description": "发起引用的节点路径。"
      },
      "to": {
        "type": "string",
        "description": "被引用的节点路径。"
      },
      "type": {
        "type": "string",
        "enum": ["depends_on", "alternative_to", "supersedes", "references"],
        "description": "引用类型：depends_on=依赖、alternative_to=替代方案、supersedes=替代了、references=引用了"
      },
      "reason": {
        "type": "string",
        "description": "建立引用的原因。"
      }
    },
    "required": ["from", "to", "type", "reason"]
  }
}
```

**引擎规则**：
- 两端节点必须存在
- 不能自引用
- 同类型关系已存在则拒绝（防重复）
- 归档节点也可以建立引用，但不推荐

---

### 3.9 `diff_mem_unlink`

**用途**：移除节点间的引用关系。

```json
{
  "name": "diff_mem_unlink",
  "description": "移除两个节点间的引用关系。",
  "parameters": {
    "type": "object",
    "properties": {
      "from": {
        "type": "string",
        "description": "发起引用的节点路径。"
      },
      "to": {
        "type": "string",
        "description": "被引用的节点路径。"
      }
    },
    "required": ["from", "to"]
  }
}
```

**引擎规则**：
- 关系不存在 → 返回 404

---

### 3.10 `diff_mem_exec`

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
          "description": "每个操作是一个 JSON 对象，包含 op 字段标识操作类型，以及对应的参数。支持的操作：CREATE, APPEND, UPDATE_FIELD, DELETE, ARCHIVE, RESTORE, LINK, UNLINK。"
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
- 不支持 `diff_mem_deep_load` 和 `diff_mem_list` 在事务中（只读操作无事务需求）

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
