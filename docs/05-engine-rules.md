# 05. 引擎层校验规则全集

> 引擎在所有 Tool 调用执行前必须通过的校验清单

---

## 一、校验分层

```
Tool Call 进入引擎
    ↓
第1层：Schema 校验（框架层，自动）
    ↓ 通过
第2层：路径校验（引擎层）
    ↓ 通过
第3层：操作语义校验（引擎层）
    ↓ 通过
第4层：频率/配额校验（引擎层）
    ↓ 通过
执行
```

---

## 二、路径校验（所有操作通用）

### 2.1 路径格式

| 校验项 | 规则 | 示例 |
|--------|------|------|
| 非空 | 不能为空字符串 | `""` → ❌ |
| 以 `/` 开头 | 必须以正斜杠开头 | `projects/alpha` → ❌ |
| 无尾斜杠 | 末尾不能有斜杠 | `/projects/` → ❌ |
| 无空层级 | 不能有连续斜杠 | `/projects//alpha` → ❌ |
| 层级数限制 | 最多 5 层 | `/a/b/c/d/e/f`（6层）→ ❌ |
| 单层长度 | 每层不超过 100 字符 | 超长 → ❌ |
| 总长度 | 不超过 256 字符 | 超长 → ❌ |
| 允许字符 | a-z, 0-9, `-`, `_`, `/`, 中文/日文/韩文 | 其他字符 → ❌ |

### 2.2 路径存在性

| 操作 | 路径必须存在？ | 不存在时的行为 |
|------|-------------|---------------|
| `diff_mem_list` | ✅ | 返回 404 + did-you-mean |
| `diff_mem_show` | ✅ | 返回 404 |
| `diff_mem_deep_load` | ✅ | 返回 404 |
| `diff_mem_append` | ✅ | 返回 404 |
| `diff_mem_update_field` | ✅ | 返回 404 |
| `diff_mem_delete` | ✅ | 返回 404 |
| `diff_mem_archive` | ✅ | 返回 404 |
| `diff_mem_create` | ❌ | 已存在 → 返回冲突 |
| `diff_mem_update_summary` | ✅ | 返回 404 |

### 2.3 路径状态

| 操作 | 节点已归档？ | 归档时的行为 |
|------|-----------|-------------|
| `diff_mem_append` | 不可追加到归档节点 | ❌ 返回 "节点已归档，不能追加事件" |
| `diff_mem_update_field` | 不可修改归档节点 | ❌ 返回 "节点已归档，不能修改字段" |
| `diff_mem_update_summary` | 不可修改归档节点 | ❌ 返回 "节点已归档" |
| `diff_mem_show` | 允许读取归档节点 | ✅ |
| `diff_mem_deep_load` | 允许读取归档节点 | ✅ |
| `diff_mem_list` | 默认不显示归档节点 | 除非 include_archived=true |
| `diff_mem_search` | 默认不搜索归档节点 | 除非 include_archived=true |

---

## 三、操作语义校验

### 3.1 `diff_mem_create`

| 校验项 | 规则 |
|--------|------|
| path 不存在 | 已存在 → 拒绝 |
| 父路径存在 | 如果 path 有多层，所有父路径必须已存在 |
| title 非空 | 不能为空 |
| summary 非空 | 不能为空 |
| summary 长度 | ≤ 500 字符 |
| tags 数量 | ≤ 20 个 |
| reason 非空 | 不能为空 |
| initial_events | 每个 event 不超过 2000 字符 |

### 3.2 `diff_mem_append`

| 校验项 | 规则 |
|--------|------|
| path 存在 | 必须存在 |
| path 未归档 | 归档节点不可追加 |
| event 非空 | 不能为空 |
| event 长度 | ≤ 2000 字符 |
| reason 非空 | 不能为空 |

### 3.3 `diff_mem_update_field`

| 校验项 | 规则 |
|--------|------|
| path 存在 | 必须存在 |
| path 未归档 | 归档节点不可修改 |
| field 非空 | 不能为空 |
| field 格式 | 只能包含 a-z, 0-9, `_` |
| value 非空 | 不能为空字符串 |
| value 长度 | ≤ 5000 字符 |

### 3.4 `diff_mem_update_summary`

| 校验项 | 规则 |
|--------|------|
| path 存在 | 必须存在 |
| path 未归档 | 归档节点不可修改 |
| old_summary 与当前一致 | 提交的 old_summary 必须等于当前存储的 summary |
| new_summary 非空 | 不能为空 |
| new_summary 长度 | ≤ 500 字符 |
| 新旧对比通过 | 引擎抽取实体对比；old 中有实体消失时，reason 必须非空且 ≥10 字符 |
| 频率未超限 | 该节点今日内更新未超过 10 次 |

### 3.5 `diff_mem_delete`

| 校验项 | 规则 |
|--------|------|
| path 存在 | 必须存在 |
| 不能删除根节点 | 不能删除 /projects 这种根层级 |
| 不能删除 meta 节点 | 不能删除引擎的元数据节点 |
| reason 非空 | 不能为空 |
| 子节点处理 | 有子节点时，引擎在删除前记录所有子路径到操作日志 |

### 3.6 `diff_mem_archive`

| 校验项 | 规则 |
|--------|------|
| path 存在 | 必须存在 |
| 不能归档根节点 | 同 delete |
| 不能重复归档 | 已归档节点不能再次归档 |
| reason 非空 | 不能为空 |
| 入站活跃引用检查 | 有活跃节点引用该节点 → 返回警告但不阻止 |

### 3.7 `diff_mem_restore`

| 校验项 | 规则 |
|--------|------|
| path 存在 | 必须存在 |
| 节点已归档 | 只有 archived 节点才能恢复 |
| reason 非空 | 不能为空 |

### 3.8 `diff_mem_link`

| 校验项 | 规则 |
|--------|------|
| from 存在 | 必须存在 |
| to 存在 | 必须存在 |
| from ≠ to | 不能自引用 |
| 关系不存在 | 同类型关系已存在 → 拒绝 |
| type 合法 | 必须在枚举范围内 |
| reason 非空 | 不能为空 |

### 3.9 `diff_mem_unlink`

| 校验项 | 规则 |
|--------|------|
| from 存在 | 必须存在 |
| to 存在 | 必须存在 |
| 关系存在 | 关系不存在 → 返回 404 |

### 3.10 `diff_mem_exec`（事务）

| 校验项 | 规则 |
|--------|------|
| 操作数量 | 1-20 个 |
| 不支持的操作 | list/show/search/deep_load 不能在事务中 |
| 顺序校验 | 所有操作按顺序预校验通过后才开始执行 |
| 原子性 | 任意操作失败 → 全部回滚 |

---

## 四、频率与配额

| 操作 | 频率限制 | 窗口 |
|------|---------|------|
| `diff_mem_create` | ≤ 100 次 | 每小时 |
| `diff_mem_append` | ≤ 500 次 | 每小时 |
| `diff_mem_update_field` | ≤ 200 次 | 每小时 |
| `diff_mem_update_summary` | ≤ 10 次 | 每天/每节点 |
| `diff_mem_delete` | ≤ 20 次 | 每小时 |
| `diff_mem_deep_load` | ≤ 200 次 | 每小时 |
| `diff_mem_search` | ≤ 500 次 | 每小时 |

超限行为：返回错误 `"RATE_LIMITED: 请稍后重试"`，不执行。

---

## 五、错误返回格式

所有校验失败统一返回：

```json
{
  "success": false,
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "human-readable 描述",
    "details": {
      "field": "出问题的字段",
      "reason": "具体原因"
    },
    "suggestion": "did-you-mean 或修复建议"
  }
}
```

错误码枚举：

| Code | 含义 |
|------|------|
| PATH_NOT_FOUND | 路径不存在 |
| PATH_EXISTS | 路径已存在（create 时） |
| PATH_INVALID_FORMAT | 路径格式不合法 |
| PATH_TOO_DEEP | 路径层级过深 |
| NODE_ARCHIVED | 节点已归档 |
| NODE_NOT_FOUND | 同 PATH_NOT_FOUND |
| FIELD_NOT_REGISTERED | 字段未注册（可选警告） |
| ILLEGAL_STATUS_TRANSITION | 状态转换非法 |
| SUMMARY_DRIFT_DETECTED | Summary 关键实体丢失 |
| RATE_LIMITED | 频率超限 |
| TRANSACTION_FAILED | 事务中某步失败 |
| INTERNAL_ERROR | 引擎内部错误 |

---

## 六、幂等性保证

同一个操作重复执行应该安全：

| 操作 | 重复执行行为 |
|------|-------------|
| `diff_mem_append` | 重复追加（不做去重，见 [04.md](./04-state-drift-defense.md)） |
| `diff_mem_update_field` | 覆盖为相同值（等价于无操作，但审计日志仍记录） |
| `diff_mem_create` | 返回冲突错误（不重复创建） |
| `diff_mem_delete` | 第二次返回 404 |
| `diff_mem_archive` | 第二次返回"已归档"错误 |

**事务的幂等性**：`diff_mem_exec` 内部没有幂等保证，由 Agent 自行处理。
