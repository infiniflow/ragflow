# PR #17780 Python → Go 对齐修复建议

## Review 范围与结论

本次只检查以下两点：

1. OceanBase/SeekDB Go document engine 是否与现有 Python 实现保持功能语义一致。
2. Go 实现是否能够原地复用 Python 已创建的底层表和数据。

静态源码对照确认，chunk、memory、metadata 的表名、字段、类型、默认值、普通索引、全文索引、向量索引，以及 JSON/ARRAY/VECTOR 编码整体兼容，原则上不需要进行 schema 或数据迁移。

当前仍有 4 个功能对齐问题需要修复，其中 2 个 P1、2 个 P2。

## 1. [P1] 删除尚未写入消息的 memory 会失败

### 涉及代码

- Go：`internal/service/memory.go`，`MemoryService.DeleteMemory`
- Go：`internal/engine/oceanbase/schema.go`，`Engine.DropChunkStore`
- Go：`internal/service/memory_message_service.go`，memory 表的懒创建逻辑
- Python：`api/apps/services/memory_api_service.py`，`delete_memory`
- Python：`common/doc_store/ob_conn_base.py`，`OBConnectionBase.delete`

### 当前差异

Go 的 memory 物理表只在写入第一条消息时创建，但 `DeleteMemory` 会在删除关系库中的 memory 记录之前无条件调用 `DropChunkStore`。`DropChunkStore` 随后直接执行：

```sql
DELETE FROM memory_<tenant_id> WHERE memory_id = ?
```

如果该 tenant 从未写入过 memory 消息，物理表还不存在，OceanBase/SeekDB 会返回 table not found，导致整个删除操作失败，关系库中的 memory 记录也不会被删除。

Python 会先调用 `MessageService.has_index`。表不存在时只删除关系库记录并正常返回；底层 `OBConnectionBase.delete` 也将表不存在视为删除 0 行。

### 建议修复

在 `Engine.DropChunkStore` 内统一保证 shared chunk/memory table 不存在时返回 `nil`，不要把该判断散落到 service 层。建议在执行 dataset-scoped `DELETE` 前调用现有 `tableExists`：

- 表不存在：直接返回 `nil`。
- 表存在：按 `kb_id` 或 `memory_id` 删除当前 dataset 的行。
- 不要删除共享表，也不要影响同 tenant 下其他 memory/KB 的数据。

这样可以同时覆盖 service 调用和未来其他调用方，并与 Python 的 no-op 删除语义一致。

### 最小回归测试

1. 创建 memory，不写入任何消息，删除成功。
2. tenant 的共享 memory 表不存在时，`DropChunkStore(..., memoryID)` 返回 `nil`。
3. 共享表存在但目标 memory 没有数据时，删除成功并返回 no-op。
4. 共享表中存在多个 memory 的消息时，只删除目标 `memory_id`，保留其他 memory 的行。

## 2. [P1] metadata 的 `≠` / `not in` 下推改变多值字段语义

### 涉及代码

- Go：`internal/engine/oceanbase/metadata.go`，`buildMetaPushdownPredicate`
- Go：`internal/engine/oceanbase/metadata.go`，`FilterDocIdsByMetaPushdown`
- Python：`api/db/services/doc_metadata_service.py`，metadata flatten 逻辑
- Python：`common/metadata_utils.py`，`filter_out`
- 可参考：`common/metadata_es_filter.py` 和 Go Elasticsearch 的 `IsPushdownSupported`

### 当前差异

Python 会把多值 metadata 展开为多个 value bucket，然后逐值执行条件。例如：

```json
{"tags": ["a", "b"]}
```

对于 `tags ≠ "a"`，Python 会因为 bucket `"b" != "a"` 而保留该文档。

Go OceanBase 当前将其下推为：

```sql
NOT JSON_CONTAINS(JSON_EXTRACT(meta_fields, '$.tags'), '"a"')
```

因为数组中包含 `"a"`，Go 会排除该文档。`not in` 存在同样问题。该下推结果会被调用方视为最终结果，不会再进行内存过滤，因此会静默漏召回文档。

现有 Python/Go Elasticsearch 和 Infinity 路径已经把 `≠`、`not in` 视为 multi-value unsafe negative operators，并主动拒绝下推。

### 建议修复

优先采用与现有引擎一致的策略：

1. 在 OceanBase metadata pushdown 支持检查中拒绝 `≠` 和 `not in`。
2. 让 `buildMetaPushdownPredicate` 返回 unsupported error，或者在进入构造器前返回 unsupported。
3. `FilterDocIdsByMetaPushdown` 收到 unsupported 后返回 `nil`，触发现有内存过滤 fallback。

除非使用 `JSON_TABLE` 等方式完整复现 Python 的逐 bucket 语义，否则不要用 `NOT JSON_CONTAINS` 近似实现。

### 最小回归测试

至少覆盖以下 metadata：

```json
{"tags": ["a", "b"]}
```

1. `tags ≠ "a"`：拒绝下推，fallback 结果包含该文档。
2. `tags not in ["a"]`：拒绝下推，fallback 结果包含该文档。
3. `tags = "a"` 和 `tags in ["a"]`：仍可下推并命中该文档。
4. 同时验证单值字段及 `and`/`or` 组合没有回归。

## 3. [P2] Highlight 与 Python 的匹配规则不一致

### 涉及代码

- Go：`internal/engine/oceanbase/helpers.go`，`Engine.GetHighlight`
- Python：`rag/utils/ob_conn.py`，`highlight` / `get_highlight`
- 可参考：`internal/engine/elasticsearch/chunk.go`，现有 Go highlight 实现

### 当前差异

Go OceanBase 当前使用区分大小写的 `strings.ReplaceAll`，并允许匹配英文单词内部；Python 对英文使用大小写不敏感的单词边界匹配，并对中文使用分词结果。

确定的差异包括：

- keyword 为 `Apple`、正文为 `apple`：Python 可以高亮，Go 无法高亮。
- keyword 为 `cat`、正文包含 `concatenate`：Python 不会把单词内部当成命中，Go 会误高亮。
- 中文内容没有利用 chunk 中已有的 `content_ltks`，可能与 Python 的分词高亮范围不同。

### 建议修复

不要继续扩展 `strings.ReplaceAll`。建议抽取或复用现有 Go Elasticsearch 的纯文本 highlight 逻辑，形成一个 engine-common helper，避免 OceanBase 和 Elasticsearch 分别维护两套近似实现。

需要保持的 Python 语义：

1. 英文匹配大小写不敏感。
2. 英文按单词边界匹配，不匹配单词内部子串。
3. 中文/非英文优先利用 `content_ltks` 或等价分词结果。
4. 未命中时不生成 highlight 项。
5. 保留文档中的原始大小写，而不是使用 keyword 文本替换原文。

### 最小回归测试

1. `Apple` 可以高亮正文中的 `apple`，并保留原文大小写。
2. `cat` 不高亮 `concatenate`。
3. 多个英文关键词及标点边界。
4. 中文关键词与 `content_ltks` 的匹配。
5. 完全未命中时结果中没有该 chunk ID。

## 4. [P2] Aggregation 错误拆分标量字符串

### 涉及代码

- Go：`internal/engine/oceanbase/helpers.go`，`Engine.GetAggregation`
- Python：`rag/utils/ob_conn.py`，`get_aggregation`
- Go 调用方：`internal/service/nlp/retrieval.go`

### 当前差异

Python 的规则是：

- 字段值为 list：分别统计每个元素。
- 字段值为非空 scalar string：将整个字符串作为一个 bucket。

Go 当前会默认按逗号拆分所有 scalar string。例如：

```text
docnm_kwd = "report,final.pdf"
```

Python 返回一个 bucket `report,final.pdf`，Go 则错误返回 `report` 和 `final.pdf` 两个 bucket。该结果会直接进入 retrieval 的 `Aggregation`。

### 建议修复

严格复现 Python 行为：

1. slice/array 值逐元素统计。
2. scalar string 去除首尾空白后作为一个完整 bucket。
3. 不要按逗号或 `###` 猜测并拆分 scalar string；数组字段应在存储解码阶段恢复为 slice。

如果计划统一所有 Go engine 的 aggregation 行为，建议抽取共享 helper，但不要在本 PR 中保留一套与 Python 不一致的 OceanBase 特例。

### 最小回归测试

1. `docnm_kwd = "report,final.pdf"` 只产生一个 bucket。
2. `tag_kwd = ["rag", "database"]` 产生两个 bucket。
3. 多个 chunk 中相同值的 count 正确累加。
4. 空字符串、空数组和缺失字段不产生 bucket。

## 存储兼容性回归建议

现有 `TestLegacyStorageRoundTrip` 由 Go 创建表、Go 写入、Go 读取，不能完整证明 Python 与 Go 双向兼容。建议补充至少一组真实 OceanBase/SeekDB integration case：

1. 使用与 Python 完全相同的 legacy DDL 创建 chunk、memory、metadata 表。
2. 写入 Python 格式的 ARRAY、JSON、VECTOR、`extra` 和 metadata denormalization 数据。
3. 使用 Go 完成读取、搜索、更新和删除。
4. 使用 Go 写入数据后，再由 Python 连接读取并核对字段值。
5. 同时覆盖无 `ES_INDEX_PREFIX` 和设置 `ES_INDEX_PREFIX` 的 memory 表名。

验收标准是不修改已有表结构、不迁移已有数据，Python 与 Go 能读取彼此写入的数据，并对同一查询返回等价结果。
