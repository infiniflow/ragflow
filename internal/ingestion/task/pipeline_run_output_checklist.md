# pipeline.run 输出字段清单

## 1. 问题背景

当前 Go 侧 `RunDataflow` 的行为以 Python 重构版
[`rag/svr/task_executor_refactor/dataflow_service.py`](/home/shenyushi/cc-workspace/ragflow/rag/svr/task_executor_refactor/dataflow_service.py:90)
为准。

这个 Python 版本里，`run_dataflow()` 在处理普通正文 chunk 时：

- 会把 `text` 搬到 `content_with_weight`
- 不会为普通正文额外补 `content_ltks / content_sm_ltks`
- 只有在 chunk 自带 `summary` 时，才会在 `run_dataflow()` 里补 `content_ltks / content_sm_ltks`

因此，如果 `pipeline.run` 的 terminal `Tokenizer` 输出里缺少
`content_ltks / content_sm_ltks`，问题应归因于 `pipeline.run` 输出不符合契约，
而不是 `RunDataflow` 这层缺少兜底。

## 2. 契约来源

本清单以 Python 端 `Tokenizer` 组件实现为准：

- [`rag/flow/tokenizer/tokenizer.py`](/home/shenyushi/cc-workspace/ragflow/rag/flow/tokenizer/tokenizer.py:132)
- 当前验证路径是 terminal 组件 `Tokenizer:LegalReadersDecide`
- 当前模板验证时使用 `search_method = ["full_text"]`
- 当前真实样本路径是：
  - 真实 MySQL
  - 真实 MinIO
  - 真实 `pipeline.run`
  - 输入文件内容为两段纯文本：
    - `Alpha paragraph.`
    - `Beta paragraph.`

## 3. 当前路径下 pipeline.run 应输出哪些字段

这里说的是 terminal `Tokenizer` 输出的 **每个 chunk** 应至少带哪些字段。

### 必备字段

`text: string`
- 原始正文内容。
- 这是 `RunDataflow` 后续生成 `content_with_weight` 的来源。

`chunk_order_int: int`
- chunk 顺序号。
- 用于保持切分后的稳定顺序。

`title_tks: string`
- 文档名的粗粒度 token。
- Python 中由 `from_upstream.name` 去掉扩展名后再做 `rag_tokenizer.tokenize(...)`。

`title_sm_tks: string`
- 文档名的细粒度 token。
- Python 中由 `rag_tokenizer.fine_grained_tokenize(title_tks)` 得到。

`content_ltks: string`
- 正文的粗粒度 token。
- 当前这条路径里，如果 chunk 有 `summary`，取 `summary`；
  否则取 `text`。

`content_sm_ltks: string`
- 正文的细粒度 token。
- Python 中由 `rag_tokenizer.fine_grained_tokenize(content_ltks)` 得到。

### 条件字段

`question_kwd: list[str]`
- 当 chunk 有 `questions` 时输出。
- 是按换行拆开的问句列表。

`question_tks: string`
- 当 chunk 有 `questions` 时输出。
- 是 `questions` 的 token 化结果。

`important_kwd: list[str]`
- 当 chunk 有 `keywords` 时输出。
- Python 当前 `Tokenizer` 里是按英文逗号拆分。

`important_tks: string`
- 当 chunk 有 `keywords` 时输出。
- 是 `keywords` 的 token 化结果。

### embedding 打开时才要求的字段

`q_<dim>_vec: list[float]`
- embedding 向量。
- 例如 `q_1024_vec`。

顶层 `embedding_token_consumption: int`
- embedding 推理消耗的 token 数。
- 只有 `search_method` 包含 `embedding` 时才要求。

## 4. 顶层输出格式要求

terminal `Tokenizer` 顶层输出应至少有：

`output_format: "chunks"`
- 表示 terminal 输出已经是 chunk 列表。

`chunks: list[dict]`
- chunk 列表。

此外运行时还可能带有：

- `_created_time`
- `_elapsed_time`
- `__cpn_id__`

这些属于运行时辅助字段，不是这次问题的关键。

## 5. 当前真实 pipeline.run 实测输出

基于真实 integration 链路，当前 terminal payload 顶层实际有：

- `output_format`
- `chunks`
- `__cpn_id__`
- `_created_time`
- `_elapsed_time`

当前每个 chunk 实际有：

- `chunk_order_int`
- `ck_type`
- `doc_type_kwd`
- `text`
- `tk_nums`

也就是说，当前 terminal `Tokenizer` 输出看起来更像是把上游 chunker 结果直接透传了，
没有把 Python `Tokenizer` 在 `full_text` 分支里本应补充的搜索字段带出来。

## 6. 当前缺失字段清单

按 Python `Tokenizer` 契约，当前样本里明确缺失：

`title_tks`
- 缺失

`title_sm_tks`
- 缺失

`content_ltks`
- 缺失

`content_sm_ltks`
- 缺失

## 7. 缺失字段的影响

`title_tks / title_sm_tks`
- 会影响基于标题的全文检索和召回质量。

`content_ltks / content_sm_ltks`
- 这是当前最关键的问题。
- Python `run_dataflow()` 不会为普通正文兜底补这两个字段。
- 因此它们一旦在 `pipeline.run` 输出中缺失，后续索引侧就会拿不到正文 token 字段。

## 8. 当前结论

当前可以明确得出的结论是：

1. Go 侧 `RunDataflow` 不应该增强补齐逻辑。
2. 测试预期不应该放宽。
3. 因此问题应归因于 `pipeline.run` terminal `Tokenizer` 输出不符合 Python 契约。

## 9. 待同事接入/修复后的核对项

- terminal `Tokenizer` 输出顶层仍为 `output_format = "chunks"`
- 每个 chunk 至少带：
  - `text`
  - `chunk_order_int`
  - `title_tks`
  - `title_sm_tks`
  - `content_ltks`
  - `content_sm_ltks`
- 若开启 embedding，再额外核对：
  - `q_<dim>_vec`
  - `embedding_token_consumption`
