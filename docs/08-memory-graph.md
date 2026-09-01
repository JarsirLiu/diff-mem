# 08. 记忆图谱：引用关系与图算法

> 节点间的引用关系构成一个有向异构图，引擎在此基础上提供图算法能力

---

## 一、引用的本质

Diff-Mem 不是简单的树结构——节点之间的引用关系在树上叠加了一个**有向图**。

```
树结构（父子关系，由 path 决定）：
  /projects/alpha
    ├── /projects/alpha/backend
    └── /projects/alpha/frontend

引用关系（语义关联，由 link 建立）：
  /projects/alpha/backend ──depends_on──→ /decisions/认证方案
  /decisions/认证方案 ───alternative_to──→ /decisions/认证方案B
  /projects/alpha/frontend ──references──→ /decisions/认证方案
```

树是物理组织（存储、路径），图是语义组织（关系、依赖）。两者独立存在，互相补充。

---

## 二、引用关系类型

```json
{
  "type": "string",
  "enum": [
    "depends_on",      // A 依赖 B（强方向性）
    "alternative_to",  // A 和 B 是替代关系（双向语义，单条边）
    "supersedes",      // A 替代了 B（强方向性，隐含 B 已过时）
    "references"       // A 引用了 B（弱方向性）
  ]
}
```

**方向性语义**：

| 类型 | 方向 | 含义 |
|------|------|------|
| `depends_on` | A → B | A 的存在依赖 B |
| `supersedes` | A → B | A 是 B 的新版本，B 已过时 |
| `alternative_to` | A → B | A 和 B 是替代方案（实际是双向，存为一条边，查询时取两端） |
| `references` | A → B | A 提及或关联 B |

---

## 三、核心图算法

### 3.1 可达性分析（Reachability）

```
问题：从节点 A 出发，能到达哪些节点？
应用：Archive A 之前，需要知道 A 影响到了哪些下游节点
```

**算法：BFS/DFS 遍历有向图**

```
archive(A) 前：
  → 计算 A 的所有可达节点（outbound 遍历）
  → 返回可达节点列表 + 距离
  → 例：A depends_on B, B depends_on C
         → 归档 A 影响到 [B(dist=1), C(dist=2)]
```

**返回格式**：

```json
{
  "success": true,
  "warnings": [
    "归档 /projects/alpha/backend 会影响以下节点："
  ],
  "impacted": [
    {"path": "/decisions/认证方案", "type": "depends_on", "distance": 1},
    {"path": "/decisions/认证方案B", "type": "alternative_to", "distance": 2}
  ],
  "action": "confirm"  // 引擎返回，等待 AI 确认是否继续归档
}
```

**复杂度**：O(V + E) 最坏情况，但通过以下优化实际很快：

1. **距离阈值截断**：只返回距离 ≤ 3 的节点（超过 3 跳的影响通常已可忽略）
2. **活跃度过滤**：只返回活跃节点的可达路径
3. **缓存**：`diff_mem_show` 时返回该节点的"影响半径"预计算结果

---

### 3.2 入站可达分析（Inbound Reachability）

```
问题：哪些节点最终指向 A？
应用：删除 A 之前，需要知道谁依赖 A；恢复 A 时，需要通知谁
```

**算法：反向 BFS（从 A 沿 inbound 边反向遍历）**

```
delete(A) 前：
  → 计算所有能到达 A 的节点
  → 返回完整依赖链
  → 例：C depends_on B, B depends_on A
         → 删除 A 影响 C（通过 B 间接依赖）
```

**与可达性的区别**：
- `outbound` 遍历 = "我影响了谁"（归档时的视角）
- `inbound` 遍历 = "谁依赖我"（删除时的视角）

**引擎行为差异**：

| 操作 | 分析方向 | 引擎行为 |
|------|---------|---------|
| `archive(path)` | outbound 可达 | 警告但不阻止 |
| `delete(path)` | inbound 可达 | 有活跃入站依赖 → 拒绝删除 |

**为什么 delete 比 archive 更严格**：
- archive 可恢复，误归档有补救
- delete 不可逆，必须确保没有活跃依赖

---

### 3.3 环检测（Cycle Detection）

```
问题：引用关系中存在环吗？
风险：A depends_on B, B depends_on A → 逻辑矛盾
```

**算法：DFS + 三色标记**

```
WHITE: 未访问
GRAY:  当前 DFS 路径上
BLACK: 已完全处理

遍历中遇到 GRAY 节点 → 发现环
```

**引擎规则**：
- `depends_on` 和 `supersedes` 不能形成环（引擎在 link 时检测，发现环则拒绝）
- `references` 可以形成环（A 引用 B，B 引用 A 是合法的）
- `alternative_to` 不能形成环（替代关系不能循环替代）

**检测时机**：
- `diff_mem_link` 调用时立即检测
- 如果加入新边会形成非法环 → 拒绝操作，返回环路径

```json
{
  "success": false,
  "error": {
    "code": "CYCLE_DETECTED",
    "message": "建立该引用会形成环：A depends_on B → B depends_on C → C depends_on A",
    "cycle": ["/A", "/B", "/C"]
  }
}
```

---

### 3.4 连通分量（Connected Components）

```
问题：记忆图谱中存在孤立的记忆簇吗？
应用：发现"信息孤岛"——与主记忆网络不相连的节点集合
```

**算法：并查集（Union-Find）**

```
维护：parent[] + rank[]，支持 find 和 union
操作：
  - link(A, B) → union(A, B)
  - 查询：diff_mem_components → 返回所有连通分量
```

**用途**：

1. **孤岛检测**：`diff_mem_orphans` 返回孤立节点列表（未与任何节点建立引用关系的节点），提示 AI 考虑建立引用
2. **归档优先级**：孤立节点可以优先归档（没有任何语义关联）
3. **图谱健康度**：连通分量的大小分布反映记忆组织的成熟度

---

### 3.5 最短路径（Shortest Path）

```
问题：两个节点之间的最短语义距离？
应用：检索增强——搜索命中 A 后，自动关联与 A 距离近的节点
```

**算法：BFS（无权图）或 Dijkstra（有权图）**

```
search(query="登录 bug") 命中 /tasks/修复登录bug
  → BFS 遍历引用图，距离 ≤ 2 的节点：
    /tasks/修复登录bug (dist=0)
    /decisions/登录方案    (dist=1, referenced_by)
    /projects/alpha/auth  (dist=2, depends_on)
  → 返回时附带关联节点
```

**返回格式**：

```json
{
  "primary_results": [...],
  "related": [
    {"path": "/decisions/登录方案", "distance": 1, "via": "references"},
    {"path": "/projects/alpha/auth", "distance": 2, "via": "depends_on → references"}
  ]
}
```

---

## 四、图数据结构设计

### 4.1 核心存储结构（Go）

```go
type Graph struct {
    mu    sync.RWMutex

    // 邻接表
    outbound map[string]map[string]EdgeType  // from → to → type
    inbound  map[string]map[string]EdgeType  // to → from → type

    // 活跃入站计数（用于 O(1) 归档检查）
    activeInbound map[string]int

    // 并查集（用于连通分量）
    parent map[string]string
    rank   map[string]int

    // 节点活跃度
    nodeActive map[string]bool
}
```

### 4.2 操作复杂度汇总

| 操作 | 复杂度 | 说明 |
|------|--------|------|
| `link(from, to)` | O(1) | 插入两条邻接表 + union |
| `unlink(from, to)` | O(1) | 删除两条邻接表 |
| `archive(path)` 检查 | O(1) | 查 `activeInboundCount` |
| `archive(path)` 执行 | O(out_edges) | 遍历出站边，更新入站计数 |
| 环检测 | O(V + E) | 最坏情况，但单条边检测时是局部 DFS |
| 可达性（outbound） | O(V + E) | BFS，带距离截断 |
| 入站可达（inbound） | O(V + E) | 反向 BFS |
| 连通分量查询 | O(1) per node | find 操作 |

---

## 五、图感知的检索增强

引用图最大的价值不在归档检查，而在**检索时利用图结构扩大召回**。

### 5.1 图增强搜索

```
普通 search:
  keywords="登录" → 命中 /tasks/修复登录bug
  → 返回 1 条

图增强 search:
  keywords="登录" → 命中 /tasks/修复登录bug
  → BFS 展开距离 ≤ 2 → 发现 /decisions/登录方案、/projects/alpha/auth
  → 返回 1 条主结果 + 2 条关联结果
```

**引擎实现**：

```go
func (g *Graph) SearchEnhanced(query string, maxDistance int) SearchResult {
    // 第1步：普通关键词匹配
    primary := keywordIndex.Match(query)

    // 第2步：对每个主结果做 BFS
    related := make(map[string]RelatedNode)
    for _, node := range primary {
        bfs := BFS(g.outbound, node, maxDistance)
        for _, r := range bfs {
            if g.nodeActive[r.path] {
                related[r.path] = r
            }
        }
    }

    // 第3步：合并去重，按距离排序
    return SearchResult{
        Primary: primary,
        Related: related,
    }
}
```

### 5.2 图遍历深度控制

```
maxDistance 参数：
  1 = 只展开直接引用（最保守，速度最快）
  2 = 展开二跳（推荐默认值）
  3 = 展开三跳（信息量大但可能返回过多）
```

**默认 maxDistance = 2**，Agent 可通过参数调整。

---

## 六、图谱维护与清理

### 6.1 断链清理

归档节点留下的"死引用"需要处理：

```
归档 A 时：
  → 遍历 A 的 outbound，对每个 to 做 activeInboundCount--
  → 遍历 A 的 inbound，从这些节点的邻接表中移除指向 A 的边
  → 注意：不删除引用记录，只从"活跃图"中移除

恢复 A 时：
  → 遍历 A 的 outbound，对每个 to 做 activeInboundCount++
  → 遍历 A 的 inbound，恢复到邻接表
```

**关键**：归档节点的引用关系仍保留在存储中（用于审计和恢复），只是不计入活跃图。

### 6.2 图谱快照

引擎定期生成图谱快照（如每天一次），记录：
- 活跃节点数
- 活跃边数
- 连通分量数
- 各分量大小
- 孤立节点数

用于监控记忆系统的健康度，辅助 Agent 做归档决策。

---

## 七、论文创新点：图感知的记忆检索

这个引用图设计本身可以成为论文的核心创新之一：

```
现有工作的问题：
  RAG/Mem0 的记忆检索是"扁平的"——
  每个记忆片段独立检索，没有利用记忆间的语义关联

Diff-Mem 的贡献：
  提出图增强检索（Graph-Augmented Retrieval）——
  在关键词检索命中后，利用引用图做 BFS 展开，
  自动召回语义关联的记忆节点

理论贡献：
  证明图增强检索在给定最大跳数 k 时，
  召回率随 k 增长，但 Token 消耗增长有界
  （因为引用图是稀疏图，BFS 展开的节点数远小于全量）
```

**这个点可以和 EPC（实体保持约束）一起构成论文的两个方法论贡献。**

---

## 八、总结

| 图操作 | 用途 | 关键算法 |
|--------|------|---------|
| 可达性分析 | 归档前影响评估 | BFS |
| 入站可达 | 删除前依赖检查 | 反向 BFS |
| 环检测 | 防止逻辑矛盾 | DFS 三色 |
| 连通分量 | 孤岛检测 | 并查集 |
| 最短路径 | 检索增强关联 | BFS |
| 入站计数 | 归档 O(1) 检查 | 增量维护 |

**引用图不是装饰，是 Diff-Mem 从"存储系统"变成"智能记忆系统"的核心能力。**
