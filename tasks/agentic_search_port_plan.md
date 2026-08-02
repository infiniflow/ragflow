# 将 Python 版 agentic search 移植到 Golang 的计划书

> 状态：**计划书全部项已实现（代码 + 测试全绿）；无 defer 项**
> 日期：2026-08-02
> 范围：聚焦"提供给 agent 使用的 tool"，以及 `dataset_nav.py` 的双重角色（`datasetnav.go` 定位有偏差，应转移并完全改写为 ES-backed 读写服务层；agent 侧直接基于 eino SDK）。

## 0.1 实现状态（2026-08-02）

| 项 | 状态 | 交付物 |
|---|---|---|
| **P0** datasetnav 读写服务层 | ✅ **已实现** | `internal/service/nav`（接口 leaf 包）+ `internal/service/nlp/datasetnav.go`（ES-backed 实现）；`cmd/ragflow_server.go` 接线 |
| **P0** 移除独立 `datasetnav` 变体 | ✅ **已实现** | `component.go`/`types.go`/`compilation_template_seed.go` 移除 + 删孤立包 + 锁死测试 |
| **P0** handler 切换 | ✅ **已实现** | `dataset_artifact_service.go` 四方法 DEPRECATED 委托 `nav.GetNavService()` |
| **P0c** tree 编译伴生产物 | ✅ **已实现** | `component.go` `upsertTreeNav`（root summary → UpsertDoc，写失败不冒泡）+ 测试 |
| **P1** `dataset_navigation_by_tree` 工具 | ✅ **已实现（最小闭环版）** | `internal/agent/tool/dataset_navigation.go`（`einotool.BaseTool`，一层下钻 + 去重 ≤8），注册 registry |
| **P2** `RetrievalRequest.DocScope` | ✅ **已实现** | `tool.RetrievalRequest.DocScope` → `nlp.RetrievalRequest.DocIDs`（nlp 已有 `filters["doc_id"]`） |
| **P0c-2** structure(page_index) 伴生产物 | ✅ **已实现** | `component.go` `upsertStructureNav`（解析 graph 产品 entities → 摘要 → UpsertDoc）+ `pageIndexSummary` + 测试 |
| **P1b** LLM 两轮选择 | ✅ **已实现** | `internal/agent/harness/datasetnav.go`（`NavigateDatasetByTree`：cluster-select → BFS 下钻 → doc-select，两轮 `askNavSelect`）+ 测试 |
| **P3a** hybrid/vector/bm25 工具 | ✅ **已实现** | `internal/agent/tool/agentic_search.go`（`AgenticSearchTool`，weight 0.3/1.0/0.0 选模式）+ 关键词窄化 + 测试，注册 registry |
| **P3b** ontology/mindmap 导航 | ✅ **已实现** | `internal/agent/harness/navigation.go`（`NavigateStructure`，读取 compiled structure + LLM 选实体 + 拉取源 chunk）+ 测试 |
| **P4a** harness route+planner 节点 | ✅ **已实现** | `internal/agent/harness`：`types.go`(RouteDecision/ClaimTarget/WorkflowPlan/ExecutionStrategy/THINKING_MODES) + `route.go`(RouteNode) + `planner.go`(PlannerNode) + 测试 |
| **P4b** orchestrator + sufficiency + answer | ✅ **已实现** | `orchestrator.go`(DirectSearch/DecomposeAndSearch) + `sufficiency.go`(CrossCheckClaim/ComputeFusionScore/RouteSufficiencyVerdict) + `answer.go`(FormalizeAnswer) + `agentic_rag.go`(RunAgenticRAG 全图) + 测试 |
| **P6** 真实 embedder 接线 | ✅ **已实现** | `internal/service/nav_embedder.go`（`NewNavEmbedder(modelSvc, name)` 解析 tenant 模型）+ `cmd/ragflow_server.go` 接线 |

> 验证命令（全绿）：`bash build.sh --test ./internal/service/... ./internal/agent/tool/ ./internal/agent/harness/ ./internal/agent/component/ ./internal/ingestion/component/knowledge_compiler/... ./internal/dao/ ./internal/handler/ ./cmd/...`
> 新增：`internal/service/nav/nav.go`、`internal/service/nlp/datasetnav.go`(+test)、`internal/service/nav_embedder.go`、`internal/agent/tool/dataset_navigation.go`、`internal/agent/tool/agentic_search.go`(+test)、`internal/agent/harness/{types,route,planner,navigation,sufficiency,orchestrator,answer,agentic_rag}.go`(+tests)、registry 注册行。
> 删除：`internal/ingestion/component/knowledge_compiler/datasetnav/{datasetnav.go,datasetnav_test.go}`。

---

## 0. TL;DR

Python 侧存在**两套不同的"agentic search"**：

1. **RAGFlow 新的 agentic-RAG 图**（`rag/advanced_rag/`，`harness/`）—— 供 "Agent/Advanced" 对话模式使用的搜索图，注册了大量 agent tools（`hybrid_search` / `vector_search` / `bm25_search` / `ontology_navigate` / `mindmap_navigate` / `dataset_navigation_by_tree` / `graph_explore` / `wiki_query` / inspector 系列等），由 `Pipeline` + `TOOL_REGISTRY` 调度。
2. **数据集导航（dataset_nav）**：既在 ingestion 写路径上构建 KB 级 nav 树（`upsert_dataset_nav_doc` / `remove_dataset_nav_doc`），又在 agentic 图中被 `dataset_navigation_by_tree` 这个 **router tool** 读取（走 REST `list_nav_clusters` / `list_nav_children`），返回命中的 `doc_id` 集，再作为下游检索工具的 `doc_scope`。

**当前 Go 侧的状态（独立变体属错误架构）：**

- Go 的 `internal/ingestion/component/knowledge_compiler/datasetnav/datasetnav.go` 作为 **KnowledgeCompiler 一个 variant**，只在**单次 Invoke 内存中**建树并产出 `Product`。这与 Python 的 `dataset_nav.py`（**ES-backed 跨运行增量 upsert/remove + read，且不是独立编译 kind，而是 tree/page_index 编译后的伴生产物**）语义**不符**。Go 的 `common/types.go:192` 注释也承认 `datasetnav` "has no Python kind"——它只是为单测驱动的占位变体，不是真实编译路径。
  - **缺**：`upsert_dataset_nav_doc` 的 ES 增量语义（read-modify-write + Redis 锁）、`search_dataset_nav()`（读 seam，KNN 检索 nav 树）、REST `list_nav_clusters` / `list_nav_children` / `delete_nav_node`、agent 侧 `dataset_navigation_by_tree` router tool、以及"在 tree/structure 编译完成后触发"的伴生产物接入点。
- Go agent 侧**基于 eino SDK**：工具即 `einotool.BaseTool`（`internal/agent/tool/`），agent 用 `eino/flow/agent/react`，编排用 `eino/compose` Graph（`internal/agent/canvas/`）。目前只有基础 `Retrieval` tool，**没有** agentic 图的任何节点/工具。

> 结论：**移除独立 `datasetnav` 变体**（Python 无此编译 kind），把其算法迁移为 `internal/service/datasetnav` 的 ES-backed 读写服务，并作为 **tree/structure(page_index) 编译完成后的伴生产物**接入（对齐 Python `compiler.py:475`/`runner.py:189`）；agentic search 的 agent 侧**直接基于 eino SDK** 落地（工具注册 + `eino/compose` Graph），不另建编排框架。这正是用户的两条异议。

---

## 1. Python 侧代码清单与角色

### 1.1 agentic-RAG 图（对话/Agent 模式）

| 文件 | 角色 |
|---|---|
| `rag/advanced_rag/agentic_rag.py` | `RAGTools`：LLM 封装、`formalize`/`extract_keywords`/`retrieve`/`web_retrieve`/`structured_retrieve`/`judge_sufficiency`/`gen_followups`/`fetch_full_document`，以及两个绑定工具 `rag`（跑整个图）和 `summarize_document`。 |
| `rag/advanced_rag/agentic_rag_graph.py` | LangGraph 4 节点：`formalize_question → route → pre_search → planner → orchestrator_loop → formalize_answer`；`run_agentic_rag()` 驱动并流式产 token。 |
| `rag/advanced_rag/tree_structured_query_decomposition_retrieval.py` | 旧版树状查询分解（`TreeStructuredQueryDecompositionRetrieval`，深度递归检索）。 |
| `harness/pipeline.py` | `Pipeline`：统一工具执行器，`_DOC_SCOPE_CONSUMERS` 让 router 产出的 `doc_scope` 自动喂给下游工具，维护 `trace`、把结果并入共享引用池 `kbinfos`。 |
| `harness/route.py` | `route_node`：问题分类，产出 `RouteDecision`（`question_type`、`requires_decomposition`、`execution_strategy`）。 |
| `harness/planner.py` | `planner_node`：按问题类型把问题分解为 `ClaimTarget[]`（factual/comparative/procedural/exploratory）。 |
| `harness/orchestrator/` | 按 thinking mode 分发：`direct.py`(direct_search) / `decompose.py`(decompose_and_search) / `agentic.py`(agentic_research, 两层循环)。 |
| `harness/config.py` | thinking mode 配置（low/medium/high/ultra）。 |
| `harness/types.py` | `ToolResult` / `RouteDecision` / `WorkflowPlan` / `ClaimTarget` 等 TypedDict。 |
| `harness/agent.py` | agent 主循环（工具调用 + 结果回填）。 |
| `harness/sufficiency.py` | 充分性判定。 |
| `harness/tools/registry.py` | `TOOL_REGISTRY` + `register_tool` + schema builder。 |
| `harness/tools/search.py` | 搜索工具：`hybrid_search`/`vector_search`/`bm25_search`/`web_search`/`structured_query` + keyword 窄化 + compiled 扩展。 |
| `harness/tools/navigation.py` | **导航工具**：`ontology_navigate`、`mindmap_navigate`、**`dataset_navigation_by_tree`**、`graph_explore`。 |
| `harness/tools/exploration.py` | `graph_explore`/`wiki_query`。 |
| `harness/tools/inspector.py` | `open_context`/`compare_sources`/`grep_within`/`request_adjacent`。 |
| `harness/prompts/` | 各节点 prompt 模板。 |

### 1.2 dataset_nav.py 的双重角色（重点）

`rag/advanced_rag/knowlege_compile/dataset_nav.py`（**注意目录名拼写 `knowlege`，Python 源码如此**）：

- **写路径（ES-backed 增量 upsert，非一次性建树；且不是独立编译 kind，而是 tree/page_index 编译后的伴生产物）**：
  - `upsert_dataset_nav_doc()`（行 563）：对**单篇文档**做增量放置——先 `_store_get` 查已有 nav_doc（同 desc 则跳过），再 KNN 找最近簇（`_find_best_cluster`，`_MERGE_THRESHOLD=0.80` 合并 / `_MIN_SIM=0.50` 建兄弟簇 / 否则建根簇；`_maybe_split_cluster` 2-means），最后 `_store_upsert` 写回 **ES 已存在的 nav 行**。全程持 per-`kb_id` Redis 锁，是**跨运行的读-改-写**，不是一次把整批 chunk 建成树。
  - `remove_dataset_nav_doc()`（行 797）/`remove_dataset_nav_doc_sync()`（行 1125，同步包装）：删除文档并级联清理空簇。
  - **调用路径（关键）**：`upsert_dataset_nav_doc` 有两条调用点，都在**别的编译模板成功产出文档摘要之后**调用，**没有独立的 `dataset_nav` 编译 kind**：
    1. **tree 路径**：`compiler.py:475` 与 `chunk_post_processor.py:1005`——在 `compile_kwd="tree"` 图 upsert 成功后，把 `tree` 对象作为输入调 `upsert_dataset_nav_doc`。
    2. **page_index 路径**：`runner.py:189` 的 `_upsert_dataset_nav_from_page_index`（行 600 调用）——从 `rebuild_structure_graph_json("page_index")` 重建的图提取摘要（`_page_index_graph_summary`），拼接文本后调 `upsert_dataset_nav_doc(summary_text)`。
  - 即：**nav 的输入本质是"文档摘要文本"**（tree 对象或 page_index summaries），nav 自己负责 embedding + KNN 放置 + 落 ES，不依赖"整个编译管线"。删除侧 `document_api.py`/`agent_api.py`/`dataset_api_service.py`/`document_service.py` 都调 `remove_dataset_nav_doc_sync`。
- **读路径（agentic search 的文档路由）**：
  - `search_dataset_nav()`（行 1013）：**官方 read seam**。`available_int=0` 的行对普通 retriever 不可见，此函数按 query 向量 KNN（或文本打分兜底 `_nav_text_score`）返回 `nav_doc`/`nav_cluster` 命中的 `doc_id`/`doc_ids`，供调用方做 scoped chunk 检索。
  - **注意**：当前 agentic 图里的 `dataset_navigation_by_tree` 并没有调用 `search_dataset_nav()`，而是走 `api/apps/services/dataset_api_service.py` 的 `list_nav_clusters`/`list_nav_children` REST 接口做 LLM 引导的树遍历。`search_dataset_nav` 目前实际上"无人调用"（是预留的语义路由 seam）。

### 1.3 dataset 导航的 REST/服务层（Go 缺失）

`api/apps/services/dataset_api_service.py`：
- `list_nav_clusters`（行 2823）、`list_nav_children`（行 2833）、`delete_nav`（行 2848）、`delete_nav_node`（行 2877，调用 `_remove_dataset_nav_doc_locked`）。
- REST：`api/apps/restful_apis/dataset_api.py` `list_dataset_nav`(958)/`list_dataset_nav_children`(980)/`delete_dataset_nav`(1003)/`delete_dataset_nav_node`(1025)。

---

## 2. Go 侧现状盘点

### 2.1 datasetnav.go（定位有偏差，需转移 + 完全改写）

`internal/ingestion/component/knowledge_compiler/datasetnav/datasetnav.go`：
- 作为 **KnowledgeCompiler 一个 variant**（`VariantDatasetnav`，`_COMPILE_KWD="dataset_nav"`）。
- 在**单次 Invoke 内存**中构建 `navTree`（`navCluster`/`navDoc`），产出 `common.Product`（`nav_cluster`/`nav_doc` + root 概览）。
- 阈值：`mergeThresholdDefault=0.80` / `recurseThreshold=0.65` / `minSim=0.50` / `maxFanout=64` / `maxDocsPerCluster=50`，与 Python 对齐。
- LLM 辅助：`llmMerge`/`llmCreateSummary`/`readableClusterName`/`fallbackTitle`/`navDocName`，与 Python prompt 一致。
- 锁：`datasetnavLock` 接口（生产走 Redis）。
- **定位偏差（用户指出）**：Python 的 `dataset_nav.py` 是**写读并存**的模块——写路径是**基于 ES 已存在 nav 树行的跨运行增量 upsert/remove**（`upsert_dataset_nav_doc` 先 `_store_get` 查已有 nav_doc → 决定 merge/split/新建 → `_store_upsert`，全程 ES 读改写 + Redis 锁），读路径是 `search_dataset_nav`（KNN/text 检索 nav 树返回 doc_id）。而 Go 这个 variant **只在单次 Invoke 内存里建树，不碰 ES**——这与 Python 的 ES-backed 增量语义**不符**，`datasetnav.go` 自己注释也承认"cross-run incremental upsert/removal is the caller's concern"。
- **结论**：Go 的 `datasetnav` 变体本身就没有 Python 对应的编译 kind（`common/types.go:192` 注释明说它 "has no Python kind"，只是为了内部单测驱动加的分支）。Python 的 `dataset_nav` 只是 tree/page_index 编译后的**伴生产物**。因此 Go **不应保留独立 `datasetnav` 变体**，而应：
  - 把该变体从 `component.go` 的 `Run`/`applyVariantColumns`/`variantCompileKWD` 及 `common/types.go` 的 `KindToVariant` 中**移除**；
  - 把 `datasetnav.go` 的算法逻辑（merge/split/summary/prompt）迁移到 **tree 和 structure(page_index) 编译完成后的增量 upsert 服务**（`internal/service/datasetnav`），对齐 Python `compiler.py:475` / `runner.py:189` 的两条调用路径；
  - 该服务既承载增量写（对齐 Python upsert/remove），又承载 agentic search 需要的读（search/list）。

### 2.2 Go agent 工具系统（基于 eino SDK）

> **关键事实**：Go 侧 agent 系统**本身就是 eino SDK 驱动的**，不是另起炉灶的自研框架。移植 agentic search **应直接基于 eino SDK**（`eino/compose` Graph、`eino/flow/agent/react`、`eino/components/tool`），复用 Go 已有的 eino 接线，而不是绕开它重写编排层。
>
> 依据：
> - `go.mod` 已依赖 `github.com/cloudwego/eino v0.9.12` + `eino-contrib/jsonschema`。
> - `internal/agent/component/agent.go` 用 `eino/flow/agent/react` 实现 ReAct agent；agent 可见工具就是 `einotool.BaseTool`（`github.com/cloudwego/eino/components/tool`）。
> - `internal/agent/canvas/` 与 `internal/agent/runtime/state.go` 用 `eino/compose` 构建 Graph。
> - `internal/agent/tool/registry.go`（`Factory`/`BuildByName`/`BuildAll`）把工具注册为 eino tool，返回 `einotool.BaseTool`。

现有工具注册点：
- `internal/agent/tool/retrieval.go`：`RetrievalTool`（`search_my_dateset`），实现 `einotool.BaseTool`，走 `RetrievalService` 接口。
- `internal/agent/tool/retrieval_service.go`：`RetrievalService` 接口 + `Set/GetRetrievalService`，`RetrievalChunk`/`RetrievalRequest`。默认 stub。
- `internal/agent/tool/retrieval_nlp.go`：`NLPRetrievalAdapter` 桥接到 `nlp.RetrievalService`。
- `internal/agent/tool/registry.go`：`Factory`/`BuildByName`/`BuildAll`，注册了大量外部工具（akshare/arxiv/google/…）。
- `internal/agent/tool/tool2component.go`：`ToolInvoker`/`ToolComponent`，让 tool 可作 Canvas component（供 Canvas DSL 引用）。
- `internal/agent/component/`：canvas 组件（Retrieval 组件在 component 层）。
- `internal/agent/canvas/`：基于 `eino/compose` 的 canvas 运行时。
- **尚无**：agentic-search 的编排图（route → planner → orchestrator 的 `eino/compose` Graph）、`dataset_navigation_by_tree` 等导航工具。

**移植立场**：新增的导航/搜索工具一律实现 `einotool.BaseTool` 并注册进 `registry.go`；若需复刻 Python `harness/` 的 route/planner/orchestrator 流程，直接用 `eino/compose` 建 Graph（可参考 `internal/agent/canvas/compile.go`），不引入新的编排框架。

### 2.3 Go 检索底层

- `internal/service/nlp/`：`RetrievalService`（`Search`），混合检索实现。
- `internal/engine/`：ES/Infinity 后端，`DocEngine.Search`（`enginetypes.SearchRequest` 支持 `Filter`/`MatchDenseExpr`/`MatchTextExpr`），在 `knowledge_compiler_wiring.go` 的 `kcWikiPageStore` 已示范如何用 `SearchRequest` 做带 filter 的 KNN。
- `internal/dao/`：gorm DAO（`CompilationTemplate` 等），但**无 nav_cluster/nav_doc 相关 DAO**。

---

## 3. 目标范围与优先级（建议分阶段）

核心原则（采纳用户两条异议 + 本次核实结论）：
1. **移除独立 `datasetnav` 变体，改为 tree/structure 编译后的 ES-backed 增量 upsert 伴生产物服务**（`internal/service/datasetnav`）：Python 无 `dataset_nav` 编译 kind（`types.go:192` 注释佐证），nav 只是 tree(page_index) 编译成功后的伴生产物。不再保留"单次内存建树"的 variant 定位。
2. **agentic search 的 agent 侧直接基于 eino SDK**：新工具实现 `einotool.BaseTool` 并注册；编排（route/planner/orchestrator）用 `eino/compose` Graph。不引入新的编排框架。

| 阶段 | 交付 | 依赖 |
|---|---|---|
| **P0** | **移除独立 `datasetnav` 变体**，新建 `internal/service/datasetnav` ES-backed 读写服务层，并接入 Go 的 tree/structure(page_index) 编译完成点（伴生产物）：(a) 写：`upsert_dataset_nav_doc`/`remove_dataset_nav_doc` 的增量语义（read-modify-write + per-kb Redis 锁 + split/merge + `available_int=0`）；(b) 读：`Search`(search_dataset_nav) + `ListClusters`(list_nav_clusters) + `ListChildren`(list_nav_children) + `DeleteNode`(delete_nav_node)。 | Go 已有 engine.Search/Insert（KNN + upsert） |
| **P1** | agent **router 工具** `dataset_navigation_by_tree`（`einotool.BaseTool`），返回命中的 `doc_id` 集。 | P0 的读能力 |
| **P2** | `RetrievalService` 扩展以支持 `doc_scope` 过滤（Python `_DOC_SCOPE_CONSUMERS` 语义）。 | P1 |
| **P3** | 其余导航/搜索工具（eino tool）：`ontology_navigate` / `mindmap_navigate` / `graph_explore` / `hybrid_search` / `vector_search` / `bm25_search` 等。 | P0–P2 |
| **P4**（可选/大） | `harness/` 编排图（route → planner → orchestrator → formalize_answer）用 `eino/compose` Graph Go 化。 | P3 |

> 建议先只做 **P0 + P1**：把 datasetnav 改写为 ES-backed 读写服务层（补上 Python 增量写语义 + agentic read），再把 agent 可调用的 `dataset_navigation_by_tree` 工具落地。这是"提供给 agent 使用的 tool" + "dataset_nav 角色"最小闭环。

---

## 4. 详细设计（P0 + P1）

### 4.1 datasetnav 读写服务层（P0）——移除独立变体，改为伴生产物

**移除 `internal/ingestion/component/knowledge_compiler/datasetnav/` 的独立 variant 定位**，把其算法逻辑（merge/split/summary/readable name、LLM prompt）迁移到 `internal/service/datasetnav` 的 ES-backed 读写服务层。agent 工具层在 `internal/agent/tool`，本服务层供 agent tool 与 REST 共用；**接入 Go 的 tree 与 structure(page_index) 编译完成点作为伴生产物**，落地为对 ES nav 行的增量读改写（`available_int=0`），而非单次内存建树。

定义接口（写 + 读，与 Python `dataset_nav.py` 对应）：

```go
type NavService interface {
    // 写 —— ES-backed 增量 upsert（对齐 Python upsert_dataset_nav_doc）
    UpsertDoc(ctx context.Context, tenantID, kbID, docID string, summary string,
        embd []float32, chat ChatInvoker) error
    // 写 —— 删除文档并级联清理空簇（对齐 remove_dataset_nav_doc）
    RemoveDoc(ctx context.Context, tenantID, kbID, docID string) error
    // 读 —— 对齐 search_dataset_nav()
    Search(ctx context.Context, tenantID, kbID, query string, embd []float32, topK int) ([]NavHit, error)
    // 读 —— 对齐 list_nav_clusters / list_nav_children
    ListClusters(ctx context.Context, tenantID, kbID string, page, pageSize int) ([]NavClusterNode, error)
    ListChildren(ctx context.Context, tenantID, kbID, nodeName string, page, pageSize int) ([]NavChildNode, error)
    // 读+删 —— 对齐 delete_nav_node（需导航锁）
    DeleteNode(ctx context.Context, tenantID, kbID, docID string) error
}

type NavHit struct {
    Type        string   // "nav_doc" | "nav_cluster"
    DocID       string
    DocIDs      []string
    Name        string
    Description string
    Score       float64
}
```

实现要点（复用 `internal/engine`：`SearchRequest` + Insert/Update/Delete，参考 `kcWikiPageStore`）：
- **触发点**：在 Go 的 tree 编译完成点（对齐 `compiler.py:475`）与 structure/page_index 编译完成点（对齐 `runner.py:189`，提取 page_index 图摘要）调用 `UpsertDoc`——即伴生产物，而非独立编译。
- **写 upsert 步骤**（对齐 Python `upsert_dataset_nav_doc`）：
  1. `_store_get` 查已有 nav_doc（`doc_id=docID`），description 相同则 return；否则先 `_remove_dataset_nav_doc_locked`。
  2. KNN 找最近簇（`_find_best_cluster`，`recurseThreshold=0.65` 下钻）：
     - `sim≥0.80` → merge 进簇（LLM merge desc + 追加 `doc_ids_kwd` + 重算簇向量）→ `_maybe_split_cluster`（fanout>64 / docs>50 → 2-means）；
     - `sim≥0.50` → 建新兄弟簇；
     - 否则 → 建根簇。
  3. 全程持 per-kb Redis 锁（`datasetnavLock`，复用现有 seam），写 `_store_upsert`（`available_int=0` + `compile_kwd=dataset_nav` + `parent_kwd`/`depth_int`/`doc_ids_kwd`/`doc_count_int`/`type_kwd`）。
- **行定位**：nav 行 `compile_kwd="dataset_nav"`，`available_int=0`（普通 retriever 默认 filter `available_int=1`，因此天然不可见）。读写都须显式 `Filter["available_int"]=0` + `compile_kwd`。
- **`Search`（search_dataset_nav）**：query embedding → `MatchDenseExpr` 于 `q_{dim}_vec`，`Filter`=`compile_kwd:dataset_nav`+`kb_id`；无 embedding 则退化为文本打分扫描（`_nav_text_score`）。
- **`ListClusters`**：`depth_int=0` 且 `type_kwd=nav_cluster` 的根簇。
- **`ListChildren`**：`parent_kwd=nodeName`，按 `type_kwd` 区分 doc（→`doc_id`）与 cluster（→`name` 供下钻）。
- **`DeleteNode`/`RemoveDoc`**：per-kb 锁下删 nav_doc、从父簇 `doc_ids_kwd` 移除、空簇级联清理——与 Python `_remove_dataset_nav_doc_locked`/`_cleanup_empty_cluster` 对齐。

依赖注入：`SetNavService`/`GetNavService`（同 `RetrievalService`），生产在 `cmd/server_main.go` 引导注册；`UpsertDoc` 的 `ChatInvoker` 可复用 `internal/agent/component` 的默认 chat invoker 或注入。

### 4.2 agent router 工具 `dataset_navigation_by_tree`（P1，eino tool）

新增 `internal/agent/tool/dataset_navigation.go`，实现 `einotool.BaseTool`（`InvokableTool`，同 `RetrievalTool`），并注册进 `internal/agent/tool/registry.go`：

- `Info()` 返回与 Python `_navigate_schema` 一致的 schema：参数 `topic`（必填）、`keywords`、`doc_scope`。
- `InvokableRun`：调 `NavService.ListClusters` → LLM 两轮选择（`_ask_nav_select` 的 prompt，用 eino ChatModel / 现有 chat seam）：
  1. 列出各绑定 KB 的根簇，问 LLM 哪些相关；
  2. 对选中簇 `ListChildren` BFS 下钻到文档叶子（depth 上限 6、叶子上限 300）；
  3. 再问 LLM 哪些文档值得读；
  4. 返回去重 `doc_id` 列表（上限 `_NAV_MAX_DOCS=8`）。
- 结果形态：扩展工具返回协议以携带 `docs`（Python `_normalize` 支持 `list[str]` 即 docs）。Go 侧可给 `RetrievalRequest`/工具结果新增 `Docs []string` 字段或独立 RouterResult。

### 4.3 `doc_scope` 贯通（P2）

- 给 `RetrievalRequest` 增加 `DocScope []string`。
- `NLPRetrievalAdapter` 把 `DocScope` 映射到检索请求的 `doc_ids` filter。
- 编排层（若做）模仿 `Pipeline._DOC_SCOPE_CONSUMERS`：router 产出的 `_routed_docs` 自动注入后续搜索/导航工具的 `doc_scope`。

### 4.4 P0 具体实现设计（Go 代码级）

以下为 P0 的落地方案，基于已核实的 Go 现有接线。

#### 4.4.1 新包 `internal/service/datasetnav`

```
internal/service/datasetnav/
  service.go      // NavService 接口 + Set/Get 单例 + 生产实现（基于 engine.Get()）
  nav_service_test.go   // 用 engine 的 in-memory test double + 假 LLM/Embedder
```

依赖注入：`SetNavService`/`GetNavService`（照抄 `tool.SetRetrievalService` 的 `sync.RWMutex` 单例模式）。生产在 `cmd/server_main.go` 引导注册：

```go
datasetnav.SetNavService(datasetnav.New(
    engine.Get(),                 // 复用全局 DocEngine 单例（internal/engine/global.go:74）
    deps.Embed,                   // 复用 common.Deps 的 Embedder（tree/wiki 已注入）
    datasetnavLockFactory,        // per-kb Redis 锁（复用 deps.Redis seam）
    chatInvoker,                  // LLM：merge desc / 根簇名 / 可读名
))
```

#### 4.4.2 读路径实现（复用 `nlp.RetrievalService` 模式）

- **底层查询**：直接构造 `types.SearchRequest`（照抄 `nlp/retrieval.go:568 Search` 与 `600` 的模式）：
  - `IndexNames: buildIndexNames([]string{tenantID})`（同 nlp 的 `buildIndexNames`），`KbIDs: []string{kbID}`。
  - **`Filter` 必须覆盖**：`available_int=0`（覆盖 nlp 默认 1，见 retrieval.go:573-575）+ `compile_kwd=dataset_nav` + `type_kwd`（按需）。
  - 向量检索：用 `nlp.RetrievalService.GetVector`（retrieval.go:782）等价逻辑——`models.EmbeddingModel` → `GetVector(query)` → `MatchDenseExpr{VectorColumnName: "q_"+dim+"_vec"}`。也可直接复用 `nlp.RetrievalService`（若它支持覆盖 `available_int` filter）或新写一个小查询器。
- **`ListClusters`**（`list_nav_clusters`）：`Filter{depth_int:0, type_kwd:nav_cluster, compile_kwd:dataset_nav}`，`SelectFields` 取 `title_kwd`/`content_with_weight`/`doc_count_int`/`kb_id`。
- **`ListChildren`**（`list_nav_children`）：`Filter{parent_kwd:nodeName, compile_kwd:dataset_nav}`，按 `type_kwd` 分 doc(`doc_id`)/cluster(`title_kwd`)。
- **`Search`**（`search_dataset_nav`）：query embedding → `MatchDenseExpr`，`Filter{available_int:0, compile_kwd:dataset_nav}`，topK 返回 `nav_doc`(→`doc_id`)/`nav_cluster`(→`doc_ids_kwd`)。
- **字段名对照**（ES 列 ← Python/Go 值）：`type_kwd`(nav_cluster|nav_doc) ← Meta.type、`title_kwd`(name) ← Meta.name、`depth_int` ← Meta.depth、`doc_count_int` ← Meta.size、`doc_ids_kwd` ← Meta.doc_ids、`parent_kwd` ← ComponentID/ParentID。

#### 4.4.3 写路径实现（ES-backed 增量 upsert）

- 复用原 `datasetnav.go` 已对齐的算法（`mergeThreshold=0.80`/`recurse=0.65`/`minSim=0.50`/`maxFanout=64`/`maxDocsPerCluster=50`、`llmMerge`/`llmCreateSummary`/`readableClusterName`/`fallbackTitle`/`navDocName` 的 Python prompt），但把"内存建树"换成"对 ES 行的 read-modify-write"：
  1. `storeGet(docID)`：`Filter{doc_id:docID, compile_kwd:dataset_nav}` 查已存在 nav_doc；desc 相同 → return。
  2. `findBestCluster`：KNN 最近簇（`recurseThreshold=0.65` 沿 `parent_kwd` 下钻）；`sim≥0.80` merge（LLM 合并 desc + `doc_ids_kwd` append + 重算簇向量 + `doc_count_int`）→ `maybeSplit`（fanout>64/docs>50 2-means）；`sim≥0.50` 新兄弟簇；否则根簇。
  3. `storeUpsert`：写 `compile_kwd=dataset_nav` + `available_int=0` + `type_kwd`/`parent_kwd`/`depth_int`/`doc_ids_kwd`/`doc_count_int` + `content_with_weight`(payload: description/type) + `q_<dim>_vec`。
  4. 全程持 per-kb Redis 锁（迁移 `datasetnavLock` seam）。
- `RemoveDoc`：per-kb 锁 → 删 nav_doc → 从父簇 `doc_ids_kwd` 移除 → 空簇级联清理（对齐 `_remove_dataset_nav_doc_locked`/`_cleanup_empty_cluster`）。

#### 4.4.4 伴生产物接入点（tree / structure(page_index) 编译完成后触发）

- **移除** `component.go:153-154` 的 `case common.VariantDatasetnav`、`:267` 的 `variantCompileKWD` 的 `dataset_nav` 映射、`:524` 的 `applyVariantColumns` 的 `VariantDatasetnav` 分支、`common/types.go` 的 `KindToVariant` 的 `datasetnav` 分支。
- **tree 接入**：在 `internal/ingestion/component/knowledge_compiler/tree/tree.go` 的 `Run`（树编译产出后）末尾，若 `specParam.Variant==VariantTree`，用 `tree` 产物的 root/level 摘要调用 `datasetnav.UpsertDoc`（对齐 Python `compiler.py:475` 把 `tree` 对象传入）。tree 产物已是 `Products`，取其中根/叶子摘要文本。
- **structure(page_index) 接入**：`component.go:146` 的 `VariantStructure` 分支内，在 `structure.Run` 产出后，若 DSL 含 page_index（结构图重建），提取 page_index 图摘要 → `datasetnav.UpsertDoc`（对齐 Python `runner.py:189` `_upsert_dataset_nav_from_page_index`）。
- 两种接入点都**不产生新的独立 Product**，而是旁路调用 `NavService` 直接写 ES（nav 行是 ES compile 行，不是 chunk 流里的 Product）。

#### 4.4.5 测试策略（AGENTS.md 分层）

- **unit（无 tag）**：`engine` 的 in-memory test double（`internal/engine` 已有）+ 假 `Embedder`/`ChatInvoker`/`RedisLock`。断言：`ListClusters`/`ListChildren` 过滤正确、`Search` 返回 doc_id、`UpsertDoc` 增量 merge/split/新簇、`RemoveDoc` 级联清理、`available_int` 恒为 0。
- **integration**（`-tags integration`）：接真实 ES/Infinity 验证 `compile_kwd=dataset_nav` 行的读写与普通检索的隔离（available_int=0 不可见）。
- **golden**：复用 `tree/raptor_test.go` 的 `signedEmbedder` 思路，锁定 LLM prompt 与字段映射。

#### 4.4.6 最小闭环（第一阶段收敛目标，评审第 1 点）

**把第一阶段收敛成一个可独立验证的最小闭环，暂缓 merge/split/list/delete/LLM 两轮选择等全量能力**：

> 目标：**编译完成后能把 nav 行写入 ES；agent 工具能读 nav 行返回 `doc_id` 列表。**

- **写（最小）**：`UpsertDoc` 先做"确定性放置"的降级实现——`storeGet`(查已有) → KNN 找最近簇 → 若 `sim≥mergeThreshold(0.80)` 则并入（追加 `doc_ids_kwd`、更新 `doc_count_int`、`available_int=0`），否则**直接建一个独立的 nav_doc/根簇**（**先不实现** LLM merge desc、`maybeSplit` 2-means、`remove` 级联清理、`list`/`delete`）。
- **读（最小）**：`Search`(query KNN → 返回 doc_id/doc_ids) + `ListClusters`/`ListChildren`（为 agent 工具提供遍历入口）三个读方法先落地。
- **agent 工具（最小）**：`dataset_navigation_by_tree` 先做"一层下钻 + 返回 doc_id 去重列表"，**先不实现** depth>1 的 BFS 与 LLM 两轮选择（两轮选择可后置为增强版）。
- **验收**：`bash build.sh --test ./internal/service/datasetnav/...` + `./internal/agent/tool/`，用 in-memory engine double 断言：`UpsertDoc` 写入行含 `compile_kwd=dataset_nav`/`available_int=0`；`Search`/`ListChildren` 返回正确的 doc_id；工具 `InvokableRun` 返回去重 doc_id 列表。
- 明确不做（一期边界外）：merge 描述重写、split/rebalance、删除级联、LLM 树遍历选择、`doc_scope` 贯通。

---

## 5. 关键对齐/风险点

1. **available_int=0 语义**：nav 行对普通检索不可见，Go 读写都必须显式处理，否则查不到/写错。Go `engine` 默认检索 filter 是 `available_int=1`，读写路径都须覆盖为 `available_int=0` + `compile_kwd=dataset_nav`。
2. **`q_{dim}_vec` 动态列**：embedding 维度运行时才知道，读写 SearchRequest/Insert 的向量列名必须由实际长度动态构造（`kcWikiPageStore` 已示范）。
3. **分布式锁**：upsert/remove/delete 共享同一把 per-kb Redis 锁；Go 现有 `datasetnavLock` seam 需从原 variant 迁移到新的 `NavService` 实现中，生产由 `deps.Redis` 注入。
4. **`dataset_navigation_by_tree` 的 LLM 选择 prompt**：必须逐字对齐 Python `_NAV_SELECT_SYSTEM`，且解析 `{"relevant": [index,...]}` 用 index（非 id）。
5. **ES 行字段一致性**：新的 ES-backed 写路径必须写全 Python nav 行字段：`type_kwd`(`nav_cluster`/`nav_doc`)、`parent_kwd`、`depth_int`、`doc_ids_kwd`、`doc_count_int`、`available_int=0`、`compile_kwd=dataset_nav`、`q_<dim>_vec`、`content_with_weight`(payload: description/type)。读路径按这些字段查询。这是写/读能否对接的关键。
6. **目录拼写**：Python 是 `knowlege_compile`（漏个 d），Go 是 `knowledge_compiler`；计划/注释不要被这个差异误导。
7. **`dataset_nav` 是独立 ES compile 行，但不是独立编译 kind；Go 应移除独立变体、改造成伴生产物 + 补 `available_int=0`**（已核实）：
   - Python：`dataset_nav.py:47` `_COMPILE_KWD="dataset_nav"`，nav 行写 `compile_kwd=dataset_nav` + `available_int=0` + `type_kwd`/`parent_kwd`/`depth_int`/`doc_ids_kwd`/`doc_count_int`。ES schema `conf/infinity_mapping.json` 已定义 `doc_ids_kwd`(61)/`doc_count_int`(76)/`depth_int`(77)/`parent_kwd`(78)。REST 遍历按 `compile_kwd=dataset_nav`+`parent_kwd`。写入口 = **tree(`compiler.py:475`/`chunk_post_processor.py:1005`) 与 page_index(`runner.py:189`) 编译成功后的伴生产物**——**没有 `dataset_nav` 这个编译 kind**（Go `common/types.go:192` 注释也承认它 "has no Python kind"）。
   - **因此 Go 应移除独立 `datasetnav` 变体**：删 `component.go` 的 `Run`/`applyVariantColumns`/`variantCompileKWD` 三处 case 与 `common/types.go` 的 `KindToVariant` 分支；把算法迁到 `internal/service/datasetnav` 增量 upsert 服务，并在 Go 的 tree/structure(page_index) 编译完成后的写入点调用（对齐 Python 两条路径）。
   - **`available_int` 缺陷**：Go 现有 `applyVariantColumns` 的 `VariantDatasetnav` 分支已正确落库 `type_kwd`/`title_kwd`/`depth_int`/`doc_count_int`/`doc_ids_kwd`，`compile_kwd`/`parent_kwd` 已设；但**落库从不写 `available_int` → nav 行默认为 1，会被普通 retriever 误检索（污染）**。新服务必须显式写 `available_int=0`（Python 用它隐藏 nav 行）。

---

## 6. 建议落地步骤（P0+P1）

1. **ES schema 已核实**：`conf/infinity_mapping.json` 对 `compile_kwd=dataset_nav` 的 nav 行已定义 `doc_ids_kwd`(61)/`doc_count_int`(76)/`depth_int`(77)/`parent_kwd`(78)/`type_kwd`/`available_int`/`q_<dim>_vec`。Python 侧 dataset_nav 作为独立 compile 行写入（`_COMPILE_KWD="dataset_nav"`）。
2. **P0a**：把 `internal/ingestion/component/knowledge_compiler/datasetnav/` 的整体逻辑迁移到 `internal/service/datasetnav`，改为基于 `engine.Get()` 的 ES-backed 读写实现（`UpsertDoc`/`RemoveDoc`/`Search`/`ListClusters`/`ListChildren`/`DeleteNode`），保留算法常量与 LLM prompt，见 4.4.1–4.4.3。
3. **P0b**：加 `SetNavService`/`GetNavService` 注入，在 `cmd/server_main.go` 引导注册（注入 `engine.Get()` + `deps.Embed` + Redis 锁 + chat invoker）。
4. **P0c**：**移除独立 `datasetnav` 变体**（`component.go` 的 `Run`(153)/`applyVariantColumns`(524)/`variantCompileKWD`(267) 三处 case、`common/types.go` 的 `KindToVariant` 分支）——Python 无此 kind；在 Go 的 **tree 编译完成点**（`tree/tree.go` Run 末尾）与 **structure(page_index) 编译完成点**（`component.go:146` 分支）调用新的 ES-backed 增量 upsert 服务（对齐 Python `compiler.py:475` / `runner.py:189`），并显式写 `available_int=0`，见 4.4.4。
5. **P1**：新增 `internal/agent/tool/dataset_navigation.go`（`dataset_navigation_by_tree`，`einotool.BaseTool`），复用 LLM chat seam；在 `internal/agent/tool/registry.go` 注册。
6. **测试**：按 4.4.5 分层——unit 用 `internal/engine` 的 in-memory test double + 假 LLM/Embedder 断言 upsert 增量/两轮选择/去重；integration 接真实 ES 验证 `available_int=0` 隔离；golden 复用 `signedEmbedder` 锁定 prompt/字段映射。
7. **P2（可选）**：`RetrievalRequest.DocScope` + adapter 映射 + 编排层的 `_DOC_SCOPE_CONSUMERS` 等价逻辑。

---

## 7. 评审补强项（已吸收，2026-08-02）

以下 4 项评审补强已全部纳入本计划，并核对了代码现状。

### 7.1 先定"最小闭环"，再扩展

见 4.4.6。第一阶段收敛为：**编译后能写 nav 行 + agent 工具能返回 doc_id 列表**。merge/split/list/delete/LLM 两轮选择全量能力后置。目标：一期可独立验证、可回退。

### 7.2 把接口边界提前定清（输入/输出契约 + 依赖 + fallback）

`NavService` 契约必须明确，避免"服务层"与"工具层"耦合过深：

| 契约项 | 定义 |
|---|---|
| **依赖** | `engine.Get()`（读/写 ES）、`deps.Embed`（embedding，query + doc summary）、per-kb Redis 锁（写路径）、chat invoker（仅 merge/summary/两轮选择，**最小闭环可传 nil 跳过**）。 |
| **LLM 依赖** | 写路径 merge/root 名/readable 名：需要 LLM；**最小闭环阶段省略**（确定性放置，不用 LLM）。读路径 `Search`/`List`：**不依赖 LLM**。 |
| **embedding 依赖** | 必须。`Search` 的 query 向量 + `UpsertDoc` 的 summary 向量。embed 失败 → 返回明确错误，调用方按 §7.3 fallback。 |
| **失败 fallback** | (a) `Search` 无 embedding → 文本打分兜底（对齐 `_nav_text_score`）；(b) 写失败 → 记录日志并继续编译（不阻塞主流程，见 §7.3）；(c) `ListClusters/Children` 查无 → 返回空集非错误。 |
| **返回格式** | `NavHit{Type, DocID, DocIDs, Name, Description, Score}`；工具层 `dataset_navigation_by_tree` 返回 `DocIDs []string`（去重，上限 8）。工具结果**不透传 NavService 内部结构**，仅暴露 `doc_id` 列表。 |

**边界纪律**：`internal/agent/tool` 只依赖 `NavService` 接口，不依赖其 engine/锁实现；`internal/service/datasetnav` 不反向 import `internal/agent`。

### 7.3 把"失败策略"写进方案（生产可维护性优先）

已核实并采纳：

- **写失败（编译完成后 nav upsert 失败）**：**记录日志并继续**，不阻塞整条编译管线。nav 是副产物，其失败不应让文档编译/索引失败。在 tree/structure 编译完成点调用 `UpsertDoc` 时包一层 `if err := nav.UpsertDoc(...); err != nil { common.Warn(...) }`（绝不 return err 阻断主流程）。
- **读失败（agent 工具调 Search/List 失败）**：工具返回空 doc_id 列表 + 明确错误信息，让 LLM agent 感知"导航不可用"，并允许回退到普通 `hybrid_search`（Python 侧同样有 fallback 语义）。
- **embedding/LLM 失败**：见 §7.2 表格。写路径 embed 失败直接 skip 该 doc 的 nav（记日志）；不做半写。
- **锁失败**：per-kb 锁获取失败（超时）→ 跳过本次 upsert（记日志），避免死等阻塞编译。

> 这些策略比算法细节更影响生产可维护性，必须写进代码注释与测试（unit 断言"写失败不冒泡"）。

### 7.4 模板/variant 的移除要与测试一起落地（防兼容裂缝）

已核实：`datasetnav` 的注册点**不止** `component.go`/`common/types.go`，还漏了两个文件：

| 注册点 | 位置 | 处置 |
|---|---|---|
| `component.go:153` `case VariantDatasetnav` | Run 分派 | 移除 |
| `component.go:267` `variantCompileKWD[VariantDatasetnav]="dataset_nav"` | compile_kwd 映射 | 移除 |
| `component.go:524` `applyVariantColumns` case | 落库字段 | 移除 |
| `common/types.go:25,204-205` `VariantDatasetnav` + `KindToVariant` | kind→variant | 移除 |
| **`dao/compilation_template_seed.go:46`** `{Kind:"datasetnav", Name:"Dataset nav..."}` | **模板 seed** | **移除**（否则旧 kind 还在 catalog，模板可被选但无对应编译，= 裂缝） |
| `ingestion_pipeline_knowledge_compiler.json` | DSL 模板引用 | 核对是否引用 datasetnav，若有则移除 |
| `component.go:3` 注释 / `deps.go:90,104` `Redis`(仅 datasetnav) | 注释/依赖 | 清理注释；Redis 依赖迁移到 NavService 而非组件 |

**同步测试**：
- 移除后补一条 unit：`common.KindToVariant("datasetnav")` 返回 `ErrUnknownVariant`（锁死移除）。
- 移除后补一条 unit：`compilation_template_seed.go` 的 `builtinCompilationTemplateKinds` 不再含 `datasetnav`（锁死 seed 移除）。
- `dataset_artifact_service.go`（现有读路径服务，字段与 Python 树结构不符，见下）补一条显式标注，避免后续误用。

**额外发现的兼容裂缝（已确认，处置见 §7.5）**：`internal/service/dataset_artifact_service.go` 的 `ListNavClusters`/`ListNavChildren`/`DeleteNav`/`DeleteNavNode` 用的是 `nav_cluster_kwd`/`nav_child_kwd`/`count_int` 字段，而 Python `dataset_api_service.py:2823-2845`（`list_nav_clusters`/`list_nav_children`）用的是 `compile_kwd=dataset_nav` + `type_kwd`/`parent_kwd`/`doc_count_int`/`doc_id`。**两者字段不一致**——Go 现有 `DatasetArtifactService` 读不到 Python `dataset_nav.py` 写的树行。

### 7.5 裂缝处置建议：废弃并替换为 `NavService`（勿修字段）

**结论：废弃 `DatasetArtifactService` 的 nav 四个方法，统一由新的 `NavService` 提供；不要"修正字段"原地修**。理由：

1. **Go `nav_cluster_kwd`/`nav_child_kwd`/`count_int` 在 Python 里根本不存在**（Python 侧 grep 全库无此三字段）。Go 这套 nav 读路径是**凭空造的错误字段**，读不到任何 Python 写入的 nav 树行，属死代码（仅 `handler/dataset_artifact.go` 挂接，无实际数据源）。
2. **Python nav 行的唯一真实字段集已确认**（`_NAV_FIELDS`，`dataset_api_service.py:2736-2746`）：`name`/`type_kwd`/`content_with_weight`/`doc_count_int`/`doc_ids_kwd`/`doc_id`/`depth_int`/`parent_kwd`，且 `available_int=0`，读路径必须 `docStoreConn.search` 直接读（绕过 `available_int=1`）。
3. `ListNavClusters` 根判定 = `parent_kwd=root`（非 `nav_cluster_kwd`）；`ListNavChildren` 子判定 = `parent_kwd=name`（非 `nav_cluster_kwd`）；层级完全靠 `parent_kwd` 串起来——与 Go 现有实现（用 `nav_cluster_kwd` 平铺）**语义不同**，原地改等于重写，不如新建 `NavService`。

**落地步骤（强约束：旧入口"整段切，不修字段"，见下方红线）**：
1. 新 `NavService`（`internal/service/datasetnav`）实现 Python 语义：`ListClusters`=`compile_kwd+type_kwd=nav_cluster+parent_kwd=root`；`ListChildren`=`compile_kwd+parent_kwd=name`；`Search`=query KNN；`DeleteNode`=子树遍历删除（对齐 Python `delete_nav_node`，先取子树再删）。
2. `DatasetArtifactService` 的 `ListNavClusters`/`ListNavChildren`/`DeleteNav`/`DeleteNavNode` 标 `Deprecated`，**整段改为委托 `NavService`**（一行委托即可，**禁止**在其内部对 `nav_cluster_kwd`/`count_int` 做任何字段级修补），或直接移除该方法体 + 更新 `handler/dataset_artifact.go` 相应 4 个 handler 指向 `NavService`。
3. `handler/dataset_artifact.go` 的 `ListNavigation`/`DeleteNavigation`/`DeleteNavigationNode`/`ListNavigationChildren`（行 328/342/356/370）**改调 `NavService`**，保持 REST 路径不变（Python REST `dataset_api.py` 对应 `list_dataset_nav`/`list_dataset_nav_children`/`delete_dataset_nav`/`delete_dataset_nav_node` 语义不变）。
4. 移除 `DatasetArtifactService` 内 `CompileKwdDatasetNav` 常量（或改为引用 `NavService` 的常量）。

> **🚫 红线（防新旧语义混用）**：
> - **旧 handler 里不做任何小修补**——不"顺手"把 `nav_cluster_kwd` 改成 `parent_kwd`、不"顺手"给 `count_int` 兜底。要么整段切到 `NavService`，要么整段删掉；不存在"局部改动旧方法"的选项。
> - **新 `NavService` 是唯一 nav 读写入口**：agent tool、REST handler、tree/structure 编译完成点，一律只调 `NavService`；`DatasetArtifactService` 不得再被新代码 import 用于 nav。
> - **同一次变更里完成切换**：`handler` 切换与 `DatasetArtifactService` 废弃在**同一个 PR/变更**内落地，不留"新旧并存的过渡窗口"，避免有人在这段时间里继续改旧方法。
> - **删除优于废弃**：只要没有外部调用方依赖 `DatasetArtifactService` 的 nav 方法，就直接删除方法（删代码 > 标 Deprecated），符合 AGENTS.md"prefer deletion over shims"。

---

### 7.6 最小闭环接口签名（P0+P1 契约）

```go
// internal/service/datasetnav/service.go
package datasetnav

const CompileKwd = "dataset_nav"
const RootParent = "root"

// NavService —— ES-backed 读写服务。最小闭环只实现黑体三个 + ListClusters/ListChildren。
type NavService interface {
    // UpsertDoc：编译完成后把单篇文档摘要放入 nav 树（最小闭环 = 确定性放置，无 LLM）。
    UpsertDoc(ctx context.Context, in UpsertDocInput) error
    // RemoveDoc：删除文档并清理（一期可只删该 doc 行，级联清理后置）。
    RemoveDoc(ctx context.Context, tenantID, kbID, docID string) error

    // Search：query KNN → 返回命中的 doc_id/doc_ids（agent 路由）。
    Search(ctx context.Context, tenantID, kbID, query string, embd []float32, topK int) ([]NavHit, error)
    // ListClusters：parent_kwd=root 的 nav_cluster 列表（list_nav_clusters）。
    ListClusters(ctx context.Context, tenantID, kbID string, page, pageSize int) ([]NavNode, int64, error)
    // ListChildren：parent_kwd=name 的直接子节点（list_nav_children）。
    ListChildren(ctx context.Context, tenantID, kbID, name string, page, pageSize int) ([]NavNode, int64, error)
}

type UpsertDocInput struct {
    TenantID string
    KbID     string
    DocID    string
    Summary  string   // 文档摘要文本（tree 产物或 page_index summaries）
    Embedd   []float32 // 可选：外部已算好则传入；否则服务内部用 deps.Embed 计算
}

type NavNode struct { // 对齐 Python _nav_item
    Name        string // name
    Description string // content_with_weight.payload.description
    DocCount    int    // doc_count_int（cluster）或 1（leaf）
    Type        string // "cluster" | "doc"
    DocID       string // leaf 的 doc_id；cluster 为空
    HasChildren bool   // is_cluster
}

type NavHit struct {
    Type    string   // "nav_doc" | "nav_cluster"
    DocID   string
    DocIDs  []string
    Name    string
    Score   float64
}

// 单例注入（照抄 tool.SetRetrievalService 模式）
func SetNavService(s NavService)
func GetNavService() NavService

// 写路径依赖注入（New 时给出）；读路径只用 engine.Get()
func New(deps Deps) NavService
type Deps struct {
    Engine    engine.DocEngine // 或复用 engine.Get() 单例
    Embed     func(ctx context.Context, texts []string) ([][]float32, error)
    RedisLock LockFactory      // per-kb Redis 锁（UpsertDoc/RemoveDoc 用）
    Chat      ChatInvoker      // 可选；最小闭环传 nil（跳过 merge/summary）
}
```

```go
// internal/agent/tool/dataset_navigation.go —— P1 工具（最小闭环 = 一层下钻）
type DatasetNavigationByTree struct{ nav datasetnav.NavService }
// Info(): schema {topic:string required, keywords:string, doc_scope:string}
// InvokableRun:
//   1. kbID := scopeToKbID(in.DocScope)          // 绑定 KB
//   2. roots,_ := nav.ListClusters(...,1,1000)   // 根簇
//   3. children := 每个 root 的 ListChildren(...,1,1000)  // 一层下钻
//   4. 收集 leaves 的 DocID + clusters 的 doc 后代（去重，上限 8）
//   5. 返回 {docs: dedupDocIDs}
// 注：LLM 两轮选择（_ask_nav_select）在一期暂缓，直接返回相关根簇的直接子文档。
```

**一期验收清单（test checklist，逐条可勾选；unit + in-memory engine double + 假 Embed/Chat）**：

> 原则：每一条都必须**真实断言**（断言返回字段/落库行字段），不留"看起来做了但没验证"的模糊地带。以 `go test` 能跑绿为准。

| # | 验收项 | 断言方式（伪码） |
|---|---|---|
| 1 | **写行字段**：`UpsertDoc` 成功后落库行**同时带** `compile_kwd=dataset_nav` 与 `available_int=0` | `row, _ := engine.GetChunk(id); assert row.CompileKwd=="dataset_nav"; assert row.Available==0` |
| 2 | 写行完整字段：`parent_kwd`/`depth_int`/`doc_count_int`/`doc_ids_kwd`/`type_kwd`/`name` 均正确 | `assert row.ParentKwd==root\|clusterName; row.DepthInt>=0; row.DocCountInt>=1; len(row.DocIDsKwd)>=1` |
| 3 | `ListClusters` 只返回 `parent_kwd=root` 的 cluster | `clusters,_ := nav.ListClusters(...); for c := range clusters { assert c.Name != "" && c.Type=="cluster" }`（且 filter 确实带 parent_kwd=root） |
| 4 | `ListChildren("X")` 只返回 `parent_kwd=X` 的直接子节点 | `children,_ := nav.ListChildren(...,"X",...); for ch := range children { assert ch.ParentKwd=="X" }` |
| 5 | `Search` 返回正确 `doc_id`/`doc_ids_kwd`（query 命中对应 nav_doc/nav_cluster） | `hits,_ := nav.Search(...); assert len(hits)>=1; assert hits[0].DocID in expectedDocs` |
| 6 | 普通检索隔离：nav 行 `available_int=0` 对默认 `SearchRequest`（available_int=1）**不可见** | 用 `nlp` 默认 filter 查同一行 → `assert noHit`（防污染） |
| 7 | **工具去重**：`DatasetNavigationByTree.InvokableRun` 返回**去重** doc_id 列表且长度 ≤8 | `docs,_ := tool.InvokableRun(...); assert unique(docs).len==len(docs); assert len(docs)<=8` |
| 8 | 写失败不冒泡：`UpsertDoc` 出错时，编译完成点只记日志、不返回 err | `err := safeUpsert(ctx,...); assert err==nil && logWarned`（见 §7.3） |
| 9 | 变体移除锁死：`KindToVariant("datasetnav")` 返回 `ErrUnknownVariant`；seed 不再含 datasetnav | `_, err := common.KindToVariant("datasetnav"); assert errors.Is(err, common.ErrUnknownVariant)`；`assert !slices.Contains(seedKinds,"datasetnav")` |
| 10 | 旧 handler 切换：`ListNavigation`/`ListNavigationChildren` 返回的数据来自 `NavService`（非旧字段） | handler 层集成断言：`ListNavigation` 返回 item 的 Name 来自 `parent_kwd=root` 行 |

---

## 8. 参考文件索引

## 8. 参考文件索引

- Python agentic 图：`rag/advanced_rag/agentic_rag.py`、`agentic_rag_graph.py`、`harness/{pipeline,route,planner,config,types,agent,sufficiency}.py`、`harness/orchestrator/{direct,decompose,agentic}.py`、`harness/tools/{registry,search,navigation,exploration,inspector}.py`、`harness/prompts/`
- Python dataset_nav（写+读）：`rag/advanced_rag/knowlege_compile/dataset_nav.py`、`runner.py`、`rag/svr/task_executor_refactor/chunk_post_processor.py`、`rag/flow/compiler/compiler.py`
- Python nav 服务/REST：`api/apps/services/dataset_api_service.py`（`list_nav_clusters`:2823、`list_nav_children`:2833、`delete_nav`:2848、`delete_nav_node`:2877，字段 `compile_kwd=dataset_nav`+`type_kwd`/`parent_kwd`/`doc_count_int`/`doc_id`，`_NAV_COMPILE_KWD="dataset_nav"`:2741、`_NAV_ROOT_PARENT="root"`:2742）、`api/apps/restful_apis/dataset_api.py`
- Go 现有（待迁移/改写）：`internal/ingestion/component/knowledge_compiler/datasetnav/datasetnav.go`、`component.go`(Run:153/variantCompileKWD:267/applyVariantColumns:524)、`common/types.go`(:25,:204)
- **Go 现有 nav 读路径（字段不符，见 §7.4）**：`internal/service/dataset_artifact_service.go`（`CompileKwdDatasetNav`(:35)、`ListNavClusters`:612、`DeleteNav`:632、`DeleteNavNode`:659、`ListNavChildren`:694，用 `nav_cluster_kwd`/`nav_child_kwd`/`count_int` 字段）、`internal/handler/dataset_artifact.go`、`cmd/ragflow_server.go`
- **Go 模板 seed（待移除）**：`internal/dao/compilation_template_seed.go:46`（`{Kind:"datasetnav"}`）、`internal/dao/compilation_template_seed_test.go`
- Go eino agent 层（复用）：`internal/agent/component/agent.go`（react）、`internal/agent/canvas/compile.go`（compose Graph）、`internal/agent/tool/{retrieval.go,retrieval_service.go,retrieval_nlp.go,registry.go,tool2component.go}`
- Go 检索底层：`internal/engine`、`internal/engine/types`、`internal/service/nlp`（`retrieval.go:568 Search`、`:600 SearchRequest`、`:782 GetVector` 模式）、`internal/engine/global.go:74 Get()` 单例
- Go ES 读写 wiring（SearchRequest/Insert 用法示范）：`internal/ingestion/task/knowledge_compiler_wiring.go`、`internal/service/nlp/retrieval.go`
- Go 伴生产物接入点：`internal/ingestion/component/knowledge_compiler/tree/tree.go`（tree.Run 末尾）、`component.go:146 VariantStructure` 分支（page_index 摘要）
