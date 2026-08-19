# Go 与 Python Wiki 编译差异及 Go 实现边界

## 结论

Python Wiki 是知识库级生成器；Go Wiki 与 ingestion pipeline 集成，是文档级生成器。Go 不应整体照搬 Python 的库级生成边界，因为一个知识库中的文档可能来自不同 pipeline，并使用不同 Wiki 模板、mode 和模型。

Go 的目标流程是：

```text
文档 ingestion
  -> 按文档 pipeline/template 执行 MAP -> REDUCE -> PLAN -> REFINE
  -> 写入不可搜索的文档 page/section
  -> 发布 doc_completed

知识库 consumer
  -> 读取本次文档 page
  -> 跨文档页面合并
  -> 写入可搜索的 dataset page
  -> 从完整 merged page 集合重建 Wiki 图
```

版本化 MAP 是文档抽取缓存，不是让 consumer 重新生成全库页面的输入。

## 为什么必须由文档生成页面

- Wiki 模板组本身只能包含一个 Wiki 模板，但同一知识库的不同文档可以使用不同 pipeline 和 Wiki 模板。
- 文档 pipeline 才能确定该文档实际使用的模板配置、mode、Compiler `llm_id` 和 embedding model。
- 如果 consumer 从全库 MAP 重新写页面，就必须错误地选择某一个模板或模型代表全部文档。
- 文档页面已经是统一产品契约，consumer 可以在不了解生成模板细节的情况下合并页面。

因此，多模板不是把 PLAN/REFINE 移到知识库级的理由，恰恰是保留文档级 PLAN/REFINE 的理由。

## 文档级 Compiler

文档 Compiler 负责完整生成：

1. 读取当前文档的有效原始 chunks，排除编译产物。
2. 以 `(tenant, kb, doc, chunk, content_hash, template_fingerprint, llm_fingerprint)` 查询不可变 MAP 缓存。
3. 只对 cache miss 调用 MAP LLM；内容改回历史版本时复用旧结果。
4. 对当前文档的全部 MAP 结果执行 REDUCE。
5. entity mode 生成一实体一页；topic mode 执行 PLAN。
6. REFINE 生成完整 page/section，并批量计算 embedding。
7. 后续 tokenizer/indexer 成功写入后才发布 `doc_completed`。

MAP 历史记录可以保留，但不应作为 `wiki_map_extract` 页面产品再次进入普通文档 chunk 流。

## 知识库级 consumer

consumer 只做跨文档聚合：

1. `doc_completed`：读取该文档最新 page 产品。
2. entity 页面：按稳定页面身份合并正文、实体成员和来源。
3. topic 页面：embedding 只缩小已有页面候选，页面合并逻辑决定更新已有页还是保留新页。
4. `doc_deleted`：删除文档产品并从 merged page 中移除其来源；来源为空时删除页面。
5. 写入 dataset-scope merged pages。
6. 从全部 merged pages 重建 `wiki_entity` / `wiki_relation` 图。

consumer 不再执行以下操作：

- 从活动 MAP 快照调用 LLM 重写全库 entity/topic 页面。
- 选择一个知识库级 Wiki 模板替代各文档模板。
- 用 canonical active generation 过滤普通 merged pages。

## Python 中仍值得复用的能力

- chunk content hash 和历史 MAP 版本复用。
- 禁用/删除来源的精确 provenance。
- embedding 只做候选召回，LLM 决定语义合并。
- 页面合并时同时处理新增与撤回证据，避免正文与来源不一致。
- alteration 基于当前 chunk/hash，而不是仅比较文档更新时间。

这些能力应放入文档生成或页面合并内部，不应改变“文档生成、库级合并”的职责边界。

## 增量触发规则

| 变化 | 正确处理 |
|---|---|
| 文档新增或重新解析 | 完整运行该文档 Wiki Compiler，成功后发布 completed |
| 文档删除 | 发布 deleted，consumer 移除来源并重建图 |
| chunk 内容、启用状态、增加或删除发生变化 | 必须先重新生成该文档页面，再发布 completed |
| 未变化文档 | 不重写文档页面；MAP cache 可直接复用 |

直接修改 chunk 时，不能只发布 completed：consumer 会读到旧文档页面。也不能简单重新解析源文件，因为那会覆盖用户手工修改的 chunk。该路径需要一个“从当前活动 chunks 仅重跑 Compiler 及下游索引”的任务入口。

## 一致性要求

- `doc_completed` 必须在文档 page/section 写入成功后发布。
- consumer 沿用 MySQL backlog、claim token、rewrite fence 和知识库写锁。
- LLM/KNN 在锁外运行，最终写入和图投影在 fence 校验后执行。
- 文档页面与 merged 页面必须保留 `source_doc_ids`、`source_chunk_ids`、slug、topic、entity names 和 outlinks。
- merged page 的正文和 provenance 必须表达同一组证据；不能只替换正文却无限 union 历史来源。

## 验收场景

1. 开启 MAP cache 后，文档仍输出带向量的 page/section，而不是只输出 MAP。
2. 两个文档使用不同 Wiki 模板时，各自按模板生成，库级 consumer 可以合并统一页面产品。
3. 修改文档并重新解析后，consumer 读取新页面，不从 MAP 再次全库生成。
4. 删除文档后，只有无剩余来源的 merged page 被删除，图同步更新。
5. 历史 MAP 缓存存在时，普通 merged page 仍可被读取和投影；当前 Go 路径不依赖额外的 canonical marker。
6. chunk 内容改回历史版本时 MAP 命中缓存，但 PLAN/REFINE 仍基于当前文档状态运行。
7. chunk API 直接增删改或禁用时，先触发 compile-only 文档 Wiki 任务，再触发库级合并。
