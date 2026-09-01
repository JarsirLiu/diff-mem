# 06. 检索完整链路设计

> 从用户 Query 到命中节点——Agent 如何使用 Diff-Mem 找到目标

---

## 一、检索的本质：Agent 的探索行为

Diff-Mem 的检索不是"一次查询返回结果"的 RAG 模式，而是一个**Agent 逐步探索**的过程：

```
用户: "上次那个登录 bug 修好没？"
    ↓
Agent 思考: 我需要找到相关记忆
    ↓
┌─────────────────────────────────────┐
│ Step 1: search tags=["bug", "login"]│  ← 引擎倒排索引，毫秒级
│ 返回: ["/projects/alpha/backend",    │     返回 ≤10 个候选
│        "/tasks/修复登录bug"]          │
└─────────────────────────────────────┘
    ↓
┌─────────────────────────────────────┐
│ Step 2: show 两个候选节点的 Header   │  ← 快速概览
│ 判断哪个相关                         │
└─────────────────────────────────────┘
    ↓
┌─────────────────────────────────────┐
│ Step 3: deep_load 相关节点          │  ← 看历史事件细节
│ 回答用户问题                         │
└─────────────────────────────────────┘
```

**关键特性**：
- Agent 可以多次调用 search（不同关键词）
- Agent 可以用 list 导航替代 search
- Agent 自己决定何时停止探索、何时回答问题
- 引擎只提供查找能力，不做"最佳回答"

---

## 二、检索工具使用场景矩阵

| Agent 已知信息程度 | 推荐工具 | 示例 |
|-------------------|---------|------|
| 完全不知道在哪 | `list("/")` → 逐层钻取 | "我想找所有项目" |
| 知道大致分类 | `list("/projects")` | "看看项目目录" |
| 知道标签 | `search(tags=["bug"])` | "找 bug 相关的" |
| 知道关键词 | `search(keywords="登录")` | "搜索登录相关" |
| 知道精确路径 | `show("/projects/alpha")` | "看 Alpha 项目" |
| 需要历史细节 | `deep_load("/tasks/修复bug")` | "看修复过程的记录" |

---

## 三、搜索引擎设计

### 3.1 倒排索引结构

```
索引 1: tags → path 倒排
┌────────────┬─────────────────────────┐
│ tag        │ paths                   │
├────────────┼─────────────────────────┤
│ bug        │ /projects/alpha, ...    │
│ login      │ /tasks/fix-login, ...   │
│ backend    │ /projects/alpha/backend │
└────────────┴─────────────────────────┘

索引 2: keywords → path 倒排（扫描 path 名 + summary）
┌────────────┬─────────────────────────┐
│ keyword    │ paths                   │
├────────────┼─────────────────────────┤
│ 登录       │ /tasks/修复登录, ...    │
│ alpha      │ /projects/alpha, ...    │
│ backend    │ /projects/alpha/backend │
└────────────┴─────────────────────────┘
```

**索引更新时机**：
- `CREATE` → 立即建立索引
- `UPDATE_FIELD` → 如果改了 summary 或 tags → 重建索引
- `UPDATE_SUMMARY` → 旧索引删除，新索引建立
- `DELETE` / `ARCHIVE` → 从索引中移除
- `APPEND` → 不更新索引（事件不进索引）

### 3.2 搜索结果排序

```
排序规则：
1. tags 精确匹配 > keywords 模糊匹配
2. 路径深度浅的优先（根目录下节点 > 深层节点）
3. 最近更新的优先
4. 节点事件数量多的优先（信息量更大）
```

### 3.3 搜索的 limit 硬约束

```
limit 默认 10，最大 50。
超过 50 → 引擎截断，返回 has_more: true。
```

Agent 如果认为结果不够，应该换关键词重新搜索，而不是要求更多结果。

---

## 四、导航设计

### 4.1 `diff_mem_list` 的聚合机制

当某个层级子节点过多时：

```
/projects/ 下有 200 个子节点
→ 引擎自动按前缀聚合：

diff_mem_list("/projects/") 返回:
{
  "path": "/projects",
  "children": [
    {"path": "/projects/a-*", "type": "group", "count": 60},
    {"path": "/projects/b-*", "type": "group", "count": 45},
    {"path": "/projects/c-*", "type": "group", "count": 30},
    ...
  ],
  "has_more": false,
  "note": "结果已按前缀聚合，共 200 个子节点"
}
```

Agent 如果想看某个分组内的节点：
```
→ diff_mem_search(keywords="alpha")  直接搜索
→ 不能 list("/projects/a-*")         聚合节点不是真实路径
```

**设计理由**：
- 不暴露聚合子节点为可导航路径，避免 Agent 走死路
- 聚合纯粹是为了"减少可见数量"，真正的缩小手段是 search

### 4.2 导航与搜索的关系

```
list: 当你不知道要找什么，想"浏览"时用
search: 当你有线索（标签/关键词），想"定位"时用
```

两者互补，不是替代关系。

---

## 五、Deep Load 的分页

`diff_mem_deep_load` 加载 Body 事件流，事件可能很多：

```
节点 /projects/alpha/backend 有 300 条事件
Agent 调用 deep_load(path, "all")
```

**引擎行为**：
```
单次最多返回 500 条。
超过 500 → 返回前 500 条，has_more: true
```

Phase 2 增加 offset/limit 分页参数：
```json
{
  "path": "/projects/alpha/backend",
  "offset": 500,
  "limit": 100
}
```

---

## 六、检索性能目标

| 操作 | 目标延迟 | 数据规模 |
|------|---------|---------|
| `list`（< 50 子节点） | < 5ms | 10K 节点 |
| `list`（> 50，需聚合） | < 10ms | 100K 节点 |
| `search(tags)` | < 5ms | 100K 节点 |
| `search(keywords)` | < 20ms | 100K 节点 |
| `show` | < 5ms | - |
| `deep_load`（recent） | < 10ms | - |
| `deep_load`（all） | < 50ms | 单节点 500 条事件 |

---

## 七、一个完整场景

```
用户: "上次和张三聊的那个 API 方案定了吗？"

Agent 内部推理:
  关键词: 张三、API、方案
  行动:
    1. search(tags=["api"]) + search(keywords="张三")
       → 返回: /people/张三, /projects/alpha/backend/api

    2. show("/people/张三")
       → 标题: 张三, 标签: [联系人, 工程师]
       → summary: "Alpha 项目后端工程师，负责 API 设计"
       → 不相关（张三个人档案，不是方案）

    3. show("/projects/alpha/backend/api")
       → 标题: Alpha API 模块
       → status: completed
       → summary: "API 设计已完成，方案在 8 月 25 日经张三确认"
       → 相关！

    4. deep_load("/projects/alpha/backend/api", "recent")
       → 最近事件:
         - [8/25] "张三确认 API 方案 v2，采用 REST 风格"
         - [8/28] "方案进入开发阶段"

    5. 回答用户: "定好了。8 月 25 日张三确认了 API 方案 v2，
       采用 REST 风格，目前已经进入开发阶段。"
```

---

## 八、Phase 2 扩展方向

- **向量索引**：在倒排索引基础上增加向量索引，支持语义搜索
- **跨节点查询**：一次查询返回多个节点的关联结果
- **时间范围查询**：`deep_load` 支持按时间范围过滤
- **全文索引**：对 Body 事件内容建立全文索引（当前只索引 path 和 summary）
