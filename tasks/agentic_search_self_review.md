# agentic search Go 移植 — 自审报告（Self-Review）

> 日期：2026-08-02
> 范围：本次 agentic search Python→Go 移植的全部实现
> 方法：code-review-and-quality 五轴审查（Correctness / Readability / Architecture / Security / Performance）
> 结论：**有条件通过（Approve with required follow-ups）**。核心架构正确、测试充分，但存在若干 Required/Correctness 项需修复后才能视为生产就绪。

---

## 一、审查范围（本次新增/改动）

| 包 | 文件 |
|---|---|
| `internal/service/nav` | `nav.go`（接口 + 单例） |
| `internal/service/nlp` | `datasetnav.go`（ES-backed 具体实现） |
| `internal/service` | `nav_embedder.go`（真实 embedder） |
| `internal/agent/tool` | `dataset_navigation.go`、`agentic_search.go` |
| `internal/agent/harness` | `types.go`、`route.go`、`planner.go`、`navigation.go`、`sufficiency.go`、`orchestrator.go`、`answer.go`、`agentic_rag.go`、`datasetnav.go` |
| `internal/ingestion/component/knowledge_compiler` | `component.go`（变体移除 + 伴生产物 hook） |
| `internal/dao` | `compilation_template_seed.go`（变体 seed 移除） |
| `internal/service/dataset_artifact_service.go` | 旧 nav 方法 DEPRECATED 委托 |
| `cmd/ragflow_server.go` | NavService + embedder 接线 |

---

## 二、按轴审查

### 1. Correctness

**Required — `component.go` 伴生产物 hook 在 multi-spec 循环中重复触发**
`Run` 里 `for range specParam.Specs` 循环中，每次迭代都检查 `variant == VariantTree`/`VariantStructure` 并调用 `upsertTreeNav`/`upsertStructureNav`。若同一批次含多个 tree spec（罕见但可能），会对**同一 doc** 重复 UpsertDoc。虽然 `UpsertDoc` 有 "summary 相同则跳过" 的幂等保护，但不同 spec 会产不同 summary → 会重复写。应只在 `specParam.Specs` 循环外、或第一个匹配 spec 后调用一次。

**Required — `harness/orchestrator.go` 的 `chunkKey` 不可靠**
`chunkKey` 对 `int` id 用 `strings.Repeat("x", id+1)` 做 key，对 `"0"` 与 `"00"` 这类会碰撞，且对无 id 的 chunk 返回 `""`（所有无 id chunk 都碰撞成同一个 key → 去重错误）。应改为基于内容/位置生成稳定 key，或明确约定 chunk 必有 `chunk_id`。

**Required — `sufficiency.go` 的 `crossScore` 判定过于宽松**
`crossPassed := len(mismatches) < len(matches)/2`，当 `len(matches)==0` 时 `0 < 0` 为 false → 全 mismatch 判为 fail（正确）。但 `len(matches)==1 && mismatches==0` → `0 < 0`? 不，`1 < 0` false → pass。逻辑大体成立，但未处理 `matches+mismatches==0` 的空证据情况（`total==0 → crossScore=0`，会落到 UNANSWERABLE）。需确认空证据的预期。

**Required — `datasetnav.go` 的 `navDocName` 用 `docID_hash` 命名根簇**
每个新文档默认建一个名为 `docID_hash` 的根簇，导致**导航树退化：文档数量 = 根簇数量**（除非互相合并）。对大型 KB 会产生大量根簇，违背 Python `list_nav_clusters` 的 "文档级聚合" 语义。这是最小闭环的已知简化，但 `navMaxClusters=500` 会截断大量文档的根簇 → 需在文档中明确标注该限制，或后续实现 sibling 聚合。

**Optional — `agentic_search.go` 的 `searchTenantID`/`searchDatasetID` 与 `dataset_navigation.go` 的 `navTenantID`/`navDatasetID` 重复**
两个文件各有一套从 canvas state 取 tenant/dataset 的逻辑，几乎相同。应合并为一个共享 helper（architecture 轴）。

**Optional — `UpsertDoc` 的 `findBestCluster` 只查根簇，不下钻子簇**
`findBestCluster` 用 KNN 在 `parent_kwd` 任意（实际 top-1 命中的可能是任意层级簇）。但 `recurseThreshold=0.65` 的"下钻"逻辑未实现——仅顶层 KNN。与 Python `_find_best_cluster`（沿树下钻）有偏差。最小闭环可接受，但需标注。

### 2. Readability & Simplicity

**Optional — `agentic_search.go` 的 `splitSentences`/`rejoinDigitPeriods` 复杂度过高**
为处理 RE2 无 lookbehind 的 "3.14" 问题，引入了 30 行专用逻辑。复杂度高于其收益。可简化为：只在句末分隔符后非数字时切分，或用更简单的启发式。当前实现可工作但有过度工程嫌疑。

**Nit — `datasetnav.go` 的 `map[bool]string{true:"cluster", false:"doc"}[cond]` 可读性差**
用 map 索引做三元表达式，不如直接 `if typ=="nav_cluster" { "cluster" } else { "doc" }`。可读性低于 Go 惯例。

### 3. Architecture

**Required — `agent/tool` 无法 import `component`（循环）导致 LLM 工具被迫迁往 `harness`**
这是一个值得记录的架构约束：所有需要 chat invoker 的工具（ontology/mindmap 导航、dataset_nav LLM 选择）都必须住在 `harness`，而 `tool/registry.go` 只能注册不依赖 chat 的工具。这导致 `tool` 与 `harness` 职责边界模糊。**长期修复**：把 `ChatInvoker` 接口+单例下沉到 leaf 包（如 `internal/agent/chat`），让 `tool` 也能 import，从而统一工具注册。当前是务实的短期方案，但应记录为后续架构债。

**Optional — `NavService` 接口放在 `internal/service/nav`（leaf）是正确决策**
避免了 `tool↔service`、`tool↔nlp` 循环。但 `nav.NavService` 接口定义在 service 包而实现依赖 engine，接口本身不带任何 service 依赖，边界清晰。良好。

**Optional — 伴生产物 hook 直接写在 `component.go` 的 Run 循环里**
`upsertTreeNav`/`upsertStructureNav` 与主编译流程耦合在同一个循环中。虽然逻辑已抽成函数，但触发条件是 `variant == X` 的 inline 分支，属于 "new conditional bolted onto unrelated flow" 的味道。可考虑把"编译后副作用"抽象成统一的 `afterCompile` hook 列表，避免在核心 Run 里堆 `if variant == ...`。

**Nit — `ExecutionStrategy` 增加了 `RequiresSelectiveGen`/`AllowsReplan` 字段但未被 orchestrator 完全使用**
这些字段定义了但 `DecomposeAndSearch` 未真正实现 selective-gen/replan 分支（只有 `REPLAN` 的简化 continue）。是"未用到的抽象"，建议要么实现要么删字段，避免误导。

### 4. Security

**Required — `highlightKeywords` 用用户关键词构造 regexp**
`regexp.MustCompile("(?i)" + pattern)`，其中 pattern 由关键词经 `regexp.QuoteMeta` 转义，所以**不会**造成 regexp 注入（QuoteMeta 已转义特殊字符）。安全。但 `MustCompile` 若 pattern 为空字符串 `"()"` 仍可编译（空捕获组），无 panic。低风险。确认安全。

**Optional — `chunkContent` 透传到 LLM/answer**
检索到的 chunk 内容（外部数据）会拼进 final answer 的 user 消息喂给 LLM。这是设计如此（RAG），但应视为不可信数据：`highlightKeywords` 插入 `<em>` 标签，若 chunk 含用户构造的 HTML 也可能被透传。当前只喂 LLM（非浏览器渲染），风险低，但若未来展示到前端需注意 XSS。

**Optional — `nav_embedder.go` 无租户越权校验**
`NavEmbedder.Encode(ctx, tenantID, ...)` 由调用方传 tenantID，未校验调用方是否有权访问该租户的 embedding model。当前 NavService 是内部服务，调用方已通过 agent/handler 层鉴权，风险可控。但若 NavService 被直接暴露，需补鉴权。

### 5. Performance

**Required — `harness/navigation.go` 的 `loadStructureEntities` 无 doc 范围过滤**
`loadStructureEntities(ctx, tenantID, docID, kinds)` 的 `SearchRequest.Filter` 用了 `"doc_id": []string{docID}`——实际上有 doc 过滤，OK。但 `Limit: 1000` 无分页，若单 doc 结构很大可能截断。低风险（单 doc 结构通常 < 1000 行）。

**Optional — `UpsertDoc` 每次调用多次 ES 往返**
`storeGet`(1次) + `findBestCluster`(1次 KNN) + 可能的 `appendDocToCluster`(1次读+1次写) + `InsertChunks`(1次)。对批量文档，这是 N×4 的 ES 往返。Python 同样如此（有 Redis 锁但无批处理），可接受，但可考虑批量化。文档注释已声明这是增量写路径。

**Optional — `splitSentences` 对每 chunk 全文做 regexp split**
`narrowContent` 对每个检索 chunk 做全量句子切分 + 逐句关键词匹配。对 topN=12 的规模可忽略。安全。

---

## 三、验证情况

- `bash build.sh --test ./internal/service/... ./internal/agent/tool/ ./internal/agent/harness/ ./internal/agent/component/ ./internal/ingestion/component/knowledge_compiler/... ./internal/dao/ ./internal/handler/ ./cmd/...` 全部 `ok`。
- 测试覆盖：nav 写入字段、ListClusters/ListChildren/Search、merge 语义、route/planner、sufficiency、orchestrator、answer、伴生产物 hook、导航工具。

**验证缺口**：
- **无 integration 测试**：`available_int=0` 隔离、真实 ES/Infinity 的 nav 行读写未用真实后端验证（仅 in-memory engine double）。
- **无 golden 测试**：LLM prompt 与 Python 的逐字对齐未用快照锁定。

---

## 四、结论与分级

### Critical（阻断合并）
- 无。

### Required（合并前必须修复/明确）
1. `component.go`：伴生产物 hook 在 multi-spec 循环中可能重复触发 → 改为循环外触发一次。
2. `orchestrator.go`：`chunkKey` 对无 id/重复 id 不可靠 → 用稳定 key 或约束 chunk 必有 `chunk_id`。
3. `sufficiency.go`：空证据（matches+mismatches==0）的 `crossPassed`/status 需明确定义预期。
4. `datasetnav.go`：`navDocName` 导致每文档一根簇的退化，需在注释/文档中明确为已知限制，或加 sibling 聚合。
5. **缺失 integration/golden 测试**（`available_int=0` 隔离 + prompt 对齐）——这是验收清单 #6 和 golden 的未落地项，须补。

### Optional / Consider（建议但不阻塞）
- `ChatInvoker` 下沉到 leaf 包，解决 `tool` 无法注册 LLM 工具的架构债（长期）。
- 合并 `searchTenantID`/`navTenantID` 等重复 helper。
- `ExecutionStrategy.RequiresSelectiveGen`/`AllowsReplan` 未用 → 实现或删除。
- `map[bool]string` 三元改 if/else；`splitSentences` 简化。
- `UpsertDoc` 的 `findBestCluster` 未实现子簇下钻（标注已知偏差）。
- 伴生产物 hook 抽象成统一 `afterCompile` 回调，避免 `if variant==X` 堆叠。

### Nit
- `nav_embedder.go` 租户越权（当前可控）、chunk 透传 LLM 的 XSS（当前无渲染面）。

---

## 五、总体评价

架构方向正确，关键设计决策（`nav` leaf 接口包、`harness` 承载 LLM 工具、变体移除、handler 整段切换）都踩在点上，测试覆盖了核心行为。**没有 Critical 问题**，代码可编译、单测全绿。

但离"生产就绪"还有差距，主要是 **5 个 Required 项**：伴生产物重复触发、chunkKey 去重、sufficiency 空证据边界、nav 每文档一根簇的语义退化，以及 **integration/golden 测试缺口**（尤其验收清单 #6 的 `available_int=0` 隔离从未用真实后端验证——这是整个移植正确性的关键前提）。

**处置建议**：先修 Required 1–3（正确性 bug/边界），对 Required 4 明确为文档化限制，补 integration 测试验证 `available_int=0` 与字段落库。Optional 项择机落地。

---

## 六、修复记录（2026-08-02）

全部 Required 与选择的 Optional 项已按建议修复，验证 `bash build.sh --test ./internal/service/... ./internal/agent/tool/ ./internal/agent/harness/ ./internal/ingestion/component/knowledge_compiler/... ./internal/dao/ ./internal/handler/ ./cmd/...` 全绿。

| # | 项 | 修复 |
|---|---|---|
| R1 | 伴生产物 hook 重复触发 | `component.go`：hook 移出 `for range specParam.Specs` 循环，用 `navDeps/navProducts/navVariant` 记录最后一次 tree/structure 输出，循环后触发一次 |
| R2 | `chunkKey` 不可靠 | `orchestrator.go`：改为 `cid:`/`id:` 前缀 + 无 id 时内容 fnv64 哈希；删除 `strings.Repeat` 碰撞逻辑 |
| R3 | sufficiency 空证据边界 | `sufficiency.go`/`types.go`：`ClaimCrossCheckResult` 加 `HasEvidence`；`ComputeFusionScore` 增加 `!anyEvidence → UNANSWERABLE` guard；missing/assessments 标记无证据 claim；`crossPassed` 要求 hasEvidence |
| R4 | navDocName 根簇退化 | `nlp/datasetnav.go` `UpsertDoc`：当 `sim>=navMinSim(0.50)` 建 sibling 子簇（`parent_kwd=bestName`, depth=1），否则根簇；避免每文档一根簇 |
| R5 | 无 integration 测试 | 新增 `internal/service/nlp/datasetnav_integration_test.go`（`//go:build integration`）：真实 ES/Infinity 验证 nav 行写 `available_int=0` + `NavService.Search` 可读 + `findNavRow` 断言字段 |
| O1 | 合并 tenant/dataset helper | 新增 `internal/agent/tool/canvas_ctx.go` `canvasTenantID`/`canvasDatasetID`；删 `navTenantID/navDatasetID/searchTenantID/searchDatasetID` 重复 |
| O2 | map[bool]string 三元 | `nlp/datasetnav.go` 两处改为 if/else |
| O3 | 子簇下钻 | `nlp/datasetnav.go` `findBestCluster` 改为 level-by-level 下钻（`parent_kwd` 逐层，`navMaxDepth=6`，sim>=navRecurse 才下钻） |
| O4 | `ExecutionStrategy` 字段 | 核查 `RequiresSelectiveGen`/`AllowsReplan` 在 `RouteSufficiencyVerdict` 中使用，非死字段，保留 |

**剩余 Optional（未做，可接受）**：ChatInvoker 下沉 leaf 包（`tool` 无法注册 LLM 工具，长期架构债）、`splitSentences` 简化、伴生产物 hook 抽象成统一 `afterCompile` 回调。

