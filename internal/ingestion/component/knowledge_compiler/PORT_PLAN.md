# 将 `rag/advanced_rag/knowlege_compile` 移植到 Go 的计划

> 目标：在 `internal/ingestion/component` 下新增一个 pipeline 组件 `KnowledgeCompiler`，
> 把 Python 版「知识编译」全部变体（wiki / raptor / mind_map / structure / dataset_nav）
> 收敛到同一个组件，按 `variant` 参数分派。

**评审缺口索引**（code review 后补，正文以「缺口 X」标注）：
- 缺口 #1 历史去重决议（wiki 组件内 ES KNN + `HistoricalCandidates` 可选覆盖）→ §4 / 风险 #2
- 缺口 A 容量护栏（防 OOM）→ `Param.Guardrails` / §4 Outputs / M1 单测
- 缺口 C Raptor 验证门禁（NMI/ARI + 回退）→ M6 / 风险 #1
- 缺口 D 输出合约前置冻结 → §4.1
- 缺口 E golden 对齐必选门禁 → §6
- 缺口 F 变体命名主名 + alias → §4 Inputs

---

## 1. 范围与现状

### Python 源（`rag/advanced_rag/knowlege_compile/`）
| 文件 | 职责 | 关键导出 |
|------|------|----------|
| `_common.py` | 共享底座：stable id、embed 包装、tokenize、chunk 批处理引擎、bulk dedup（exact+embed+LLM）、ES I/O。<br>**注意**：Go 版删除「从 ES 拉源 chunk」与「组件内写 ES」，但保留 **wiki 变体历史去重的 ES KNN 读路径**（见 §1.1）；输入仍来自内存 inputs，产物以 chunks 形式合并进上游分块流返回 | `stable_row_id`, `encode`, `build_chunk_batches`, `run_chunked_pipeline`, `bulk_dedup_items`（内存去重 + wiki 历史候选 ES KNN） |
| `structure.py` | 文档级结构编译（list/set/hypergraph）+ 本地/ES 去重 + 紧凑图 JSON | `compile_structure_from_text`, `merge_compiled_structures`, `rebuild_structure_graph_json` |
| `wiki.py` | 工件管线 MAP→REDUCE→PLAN→REFINE（最大，~3600 行） | `wiki_map_from_chunks`, `wiki_reduce_from_extracts`, `wiki_plan_from_reduction`, `wiki_refine_from_plan` |
| `raptor.py` | RAPTOR 摘要树：classic + Psi 两种 builder，GMM/AHC/UMAP 聚类 | `RecursiveAbstractiveProcessing4TreeOrganizedRetrieval` |
| `mind_map_extractor.py` | 基于 `markdown_to_json` 的心智图抽取 | `MindMapExtractor` |
| `dataset_nav.py` | 数据集级导航的增量聚类（nav_cluster / nav_doc 树 + Redis 锁） | `upsert_dataset_nav_doc`, `remove_dataset_nav_doc` |
| `runner.py` | 非 tree 模板的批量编排（LLMCallPool、flush、synthesis 阶段） | `run_structure_compile_over_batches` |

### 1.1 输入/输出/存储模型变更（关键，Go 版以内存为主 + wiki 历史去重读 ES）

Python 版 `knowlege_compile` 在运行时**全程依赖 ES**：先连 ES 把已切好的源 chunks 拉出来
（`_common.py` 的 `es_search` / `MatchDenseExpr` 取 `doc_id` 对应的 chunk 集合），
编译过程中的去重/合并又**回查 ES 里已写入的编译产物**做 KNN 候选，最后把产物写回 ES。

Go 版是 **pipeline 组件，以内存为主**，并在 wiki 历史去重阶段读 ES KNN：

1. **输入来自内存**：`KnowledgeCompiler` 的 `Inputs()` 直接消费上游 `chunker`/`parser` 输出的
   `chunks`（每项带 `id` + `text`/`content_with_weight` 等）。删去「按 `doc_id` 去 ES 拉源 chunk」。
2. **编译产物驻留内存**：所有编译产物（structure 图 / wiki page / raptor summary / mindmap tree /
  datasetnav 节点）在 `Invoke` 期间先存于内存结构中，**组件不负责写 ES**。
3. **落库由 pipeline 调用者负责**：产物通过 `Outputs()` 返回给下游，由 pipeline 的
   调用方（组合根 / 后续 writer 组件）统一决定何时、如何写入 ES。
4. **去重分两层**：
  - 运行内去重：在内存里已累积的编译产物集合中做向量相似检索（暴力余弦 / 小顶堆 top-k）；
  - 历史去重（wiki 变体）：对 document 级 graph（entity/relationship）并行发起 ES KNN 候选检索，
    作用域受 `tenant_id` + `dataset_id` + `variant` 过滤约束，命中后在组件内直接 merge。

> 结论：`KnowledgeCompiler` 组件保留 `common/memstore.go`（运行内去重），
> 并为 wiki 变体新增**只读 ES KNN 候选检索**依赖（不在组件内写 ES）。
> `Deps` 包含 `Chat` / `Embed` / `Tokenizer`，以及 `HistoricalKNN`（或 `DocEngine.Search` 只读封装，见 §3/§4）。

### 移植原则（遵循 AGENTS.md）
- 收敛为**单一组件** `KnowledgeCompiler`，用 `variant` 参数分派，不保留 Python 的多文件分叉。
- 复用现有 `runtime.Component` 注册模式（参考 `component/extractor.go`）。
- 组件包 **不得** 反向 import `internal/service`；模型/引擎解析器在 `internal/ingestion/task`（组合根）注入，沿用 `embedder.go` 的 `DefaultEmbedderResolver` 模式。
- 优先用 Go 已有能力，缺失的才新增；不引入与现有 runtime 重复的兼容层。

---

## 2. 模块映射（Python → Go）

```
rag/advanced_rag/knowlege_compile/
├── _common.py          → knowledge_compiler/common/      (id, embed, tokenize, batching, dedup engine, in-mem store)
├── structure.py        → knowledge_compiler/structure/   (compile + merge + chain + graph json)
├── wiki.py             → knowledge_compiler/wiki/        (map / reduce / plan / refine)
├── raptor.py           → knowledge_compiler/raptor/      (classic + psi builders + clustering)
├── mind_map_extractor  → knowledge_compiler/mindmap/     (markdown→json tree)
└── dataset_nav.py      → knowledge_compiler/datasetnav/  (incremental nav clustering + redis lock)
```

`runner.py` 的编排逻辑（LLMCallPool / flush / synthesis）下沉为
`knowledge_compiler/common` 里的并发原语（信号量 + 优先级队列 + 批累积器），
由 `KnowledgeCompiler.Invoke` 直接驱动，不再单独保留一个 runner 文件。

---

## 3. Go 基础设施依赖：现状与缺口

### ✅ 已具备
- **Embedding**：`componentpkg.Embedder` 接口 + `EmbedderResolver` + `DefaultEmbedderResolver`
  （`internal/ingestion/task/embedder.go` 已在 `init()` 注入）。
- **Tokenizer**：`internal/tokenizer`（C++ binding，含 `tokenize` / `fine_grained_tokenize` 等价能力）。
  另需一个 `num_tokens_from_string` 等价函数（基于 tiktoken 或 RAG 分词器，放在 `common` 包）。
- **DocEngine / 历史 KNN Reader（只读）**：组件不写 ES，但 wiki 变体会在历史去重阶段读取 ES KNN 候选
  （见 §1.1）。建议以 `HistoricalKNN` 接口封装 `Search` 能力，避免组件直接耦合全量 `DocEngine`。
  输入 chunks 仍来自内存 inputs，写入 ES 与普通 chunk 走同一落库路径。
- **Chat 工厂**：`models.NewModelFactory().CreateModelDriver(...)` + `models.NewEinoChatModel`
  （`extractor.go:einoExtractorChatInvoker` 已示范）。
- **xxhash**：`github.com/cespare/xxhash/v2`（已在 go.mod，被 `chunk_builder` 等使用）。
- **组件注册**：`runtime.MustRegister(name, CategoryIngestion, ctor, Metadata{Inputs,Outputs})`。

### ⚠️ 缺口（需新建，按风险排序）
1. **JSON-mode chat helper（gen_json 等价物）** —— 最高频依赖。
   封装 `einoExtractorChatInvoker`：请求时带 `response_format=json`（或 JSON schema），
   返回解析后的 `map[string]any` / `[]any`，带 code-fence 剥离、重试、超时。
   建议新增 `knowledge_compiler/common/jsonchat.go`，复用 `extractor` 的 driver 解析与 invoker 注入 seam。
2. **内存产物存储 + 内存向量检索（`common/memstore.go`）** —— 承担运行内去重（见 §1.1）。提供：
   - `Add(item)` / `Upsert(id, item)` / `Delete(id)`：维护本次 pipeline 内的编译产物集合；
   - `TopK(vec []float32, k int, threshold float64) []Hit`：内存余弦 top-k，供去重/合并做候选查找。
     **实现建议（见下方①）**：内部把已存产物向量维护为一个 `[][]float32` + 预计算的 L2 norm
     （embedder 若已归一化则可直接用点积）；`TopK` 对 query 做一次 matVec 取点积再阈值过滤 +
     部分 top-k。单次 pipeline 规模不大，朴素循环也够，但走矩阵形态（gonum `mat.Dense.Mul`
     或手写点积）更一致、更易与 RAPTOR Psi 的全对 `M·Mᵀ` 共用同一套向量运算。
  - `Snapshot() []Item`：`Invoke` 结束时把全部产物交给 `Outputs()`，由 pipeline 调用者落库。
3. **历史去重 KNN 检索器（wiki 专用）** —— 新增 `common/historical_knn.go`（或等价位置）：
  - `TopKHistory(ctx, qvec, k, threshold, scope) []Hit`，`scope` 至少含 `tenant_id/dataset_id/variant`；
  - 仅用于读取历史候选；命中后在组件内执行 merge（仍不在组件内写 ES）。
3. **聚类数值库（RAPTOR classic builder）** —— Python 用 `sklearn` 的
   `GaussianMixture`(BIC 选簇)、`AgglomerativeClustering`(ward + dendrogram gap)、`umap.UMAP`。
   **最终决策（M6 前已拍板）：纯 Go + `gonum`，不引入 `pa-m/sklearn`，不用 CGO/C++ 内核。**

   **放弃 `pa-m/sklearn`**：它只是 sklearn API 的 Go 模仿版，不成熟，尤其 GMM/UMAP 支持残缺，
   多一个需审计的第三方 ML 依赖得不偿失。
   **不用 CGO/C++ 内核（`libcluster.so` 之类）**：本仓库 `build.sh` 已在 juggling CGO 原生库
   （office_oxide / pdfium / pdf_oxide / lld），再加一个自维护 C++ 数值内核会显著加重构建与长期维护负担；
   而本场景数值热度根本不需要它（见下）。
   **只加 `gonum.org/v1/gonum`（纯 Go、无 CGO，不影响 `build.sh` 的 CGO 链路）到 go.mod。**

   **关键事实（已核对 raptor.py 源码）**：
   - `clustering()` 在分支**之前**无条件跑 `umap.UMAP(metric="cosine")` 降到 ≤12 维（`raptor.py:326-333`），
     所以**即便只支持 AHC 也仍需降维器**；Go 用 **PCA（gonum `mat.SVD`，SVDThin）** 替代 UMAP。
     对文本 embedding（BGE/E5/...）PCA 通常保留大部分聚类结构，且比「cosine-UMAP→Ward」更契合 Ward 的欧氏几何。
   - **Psi builder 完全不聚类**（仅余弦矩阵 + union-find，`raptor.py:425-432`），零 GMM/AHC/UMAP 依赖。
   - **GMM 是 `covariance_type="diag"` + `reg_covar=1e-4`（`raptor.py:261,351`），不是 full 协方差**。
     因此 EM 只需逐维均值/方差，无需 Cholesky / 全协方差行列式 / 逆矩阵 —— 实现量约 **200-300 行**，
     远低于 full 协方差的 500-800 行；BIC 参数 `p = K·d(均值) + K·d(对角协方差) + (K-1)(权重)`，可直接算。
     GMM 还用 `predict_proba` + `threshold`(默认 0.1) 软分配（取 prob>阈值 的首簇，否则 argmax）。
   - **Ward AHC**：维护 `{centroid, size, SSE}` 的 struct（cache locality 好），距离矩阵用扁平 `[]float64`
     连续内存（避免 `[][]float64` 反复分配）；典型 N=几千~几万、降维后 D≤12，O(n²) 距离矩阵完全可接受。
     保存 merge history 即得到 linkage 矩阵 `Z=[left,right,distance,size]`，**dendrogram gap = 排序后 heights 的差分 argmax**。

   **交付分解（M6）**：
   - Psi builder：仅余弦矩阵（复用 ① 的 memstore 向量运算），零聚类依赖。
   - classic(AHC + PCA)：自写 Ward 链接 + gap 检测 + `_adjust_tree_nodes`(质心重分配，k-means 式)；PCA 走 gonum。
   - GMM/BIC + `predict_proba` 软分配：**留 `ClusteringMethod` 可插拔接口，后补**（纯 Go diagonal-GMM EM，已确认成本低）；
     不在 M6 阻塞项。

   **接口骨架（现在就定义，避免将来改 `raptor.Run` 签名）** —— M6 落地前先把边界定死：

   ```go
   // ClusteringMethod 标识聚类后端；M6 只交付 AHC，GMM 留 TODO。
   type ClusteringMethod string

   const (
       ClusteringAHC ClusteringMethod = "AHC"        // 默认：Ward + gap 检测 + _adjust_tree_nodes
       ClusteringGMM ClusteringMethod = "GMM"        // TODO(M6+): diagonal-GMM EM + BIC + predict_proba 软分配
   )

   // ClusterSpec 聚类配置，对应 Python RaptorConfig 的聚类相关字段。
   type ClusterSpec struct {
       Method      ClusteringMethod
       Threshold   float64 // AHC: dendrogram gap 阈值；GMM: predict_proba 软分配阈值(默认 0.1)
       MinClusters int     // _get_optimal_clusters 下界
       MaxClusters int     // _get_optimal_clusters 上界(默认 20)
   }

   // Clusterer 聚类后端接口；AHC/Psi 实现各自满足。
   // embeddings: 降维后矩阵 (n×d, d≤12)；返回每点簇标签 + 簇质心（供 _adjust_tree_nodes）。
   type Clusterer interface {
       Fit(embeddings [][]float64, spec ClusterSpec) (labels []int, centroids [][]float64, err error)
   }

   // Run 签名以 ClusterSpec 传入，后端按 Method 分派；GMM 未实现时返回明确 ErrNotImplemented。
   func Run(ctx context.Context, in *Inputs, spec ClusterSpec, deps Deps) (*Outputs, error)
   ```

   注：Psi builder 不走 `Clusterer`（仅余弦矩阵 + union-find），但 `Run` 仍以 `ClusterSpec` 统一入口，
   内部按 `Method`/`builder` 分派到 Psi 或 classic 路径，保证签名单一、将来加 GMM 不破 `Run`。
4. **`markdown_to_json`（mind_map）的 markdown 来源澄清** —— 注意它**不是文档级也不是 chunk 级
   的源 markdown**，而是 **LLM 的回答 markdown**（`mind_map_extractor.py:168`
   `markdown_to_json.dictify(response)`，`response` 是 `MIND_MAP_EXTRACTION_PROMPT` 产出的思维导图大纲；
   输入 `sections` 只是喂给 LLM 的源文本，`dictify` 作用在模型输出上）。
   因此**不能复用 Go Parser 组件（`parser.go`/`parser_dispatch.go`）的输出**——那是源文档解析结果，与本题无关。
   Go 端用 `goldmark` 解析** LLM 回复的 markdown 大纲**，按 Python `dictify` 语义（标题层级→嵌套 dict、
   列表项→数组）自实现轻量 AST 遍历转换即可；无需引入等价重库。计划措辞应改为「LLM 回复 markdown→tree」。
5. **Redis 分布式锁** —— `dataset_nav` 用 `RedisDistributedLock`。
   `internal/engine/redis` 已有 `RedisClient`，新增一个 `spin_acquire/release`（SET NX + TTL）封装即可。
6. **多语言 config localize** —— Python 的 `_struct_localize` 取 `cfg["en"]` 等。
   Go 端 `common` 加一个 `localize(value any, lang string) string` 处理 `string` / `[]string` / `map[string]string`。
7. **`split_chunks` token budget（注意含义）** —— 它**不读 ES/磁盘、也不切分单个 chunk**，
   而是把**内存中**的 chunks **按 LLM 上下文窗口做 token-budget 打包成 batch**
   （`generator.split_chunks:795` 注释「Do not split a single chunk, even if it exceeds max_length」，
   是 `build_chunk_batches`/`run_chunked_pipeline` 引擎里的预算装箱步骤）。既然 chunks 已在内存，
   仍需要它来尊重 `chat_mdl.max_length` 的窗口约束。**建议改名** `split_chunks.go` → `batch_packing.go`
   （或并入 `batch.go`），明确其语义是「内存 chunks → LLM 批次」，避免误读成「从外部切分加载」。

---

## 4. 组件接口设计

```go
const componentNameKnowledgeCompiler = "KnowledgeCompiler"

// Param 由 DSL params 构造；缺失键走 Defaults()。
type Param struct {
    Variant          string // "structure" | "wiki" | "raptor" | "mindmap" | "datasetnav"
    LLMID            string
    EmbeddingModel   string
    Language         string // "en" / "zh" / ...
    TemplateIDs      []string // structure/wiki 用；group_ids 在 Invoke 时解析
    GroupIDs         []string
    SimilarityThreshold float64 // 去重阈值，默认 0.99(structure) / 0.9(artifact)
    MaxWorkers       int
    // 变体特有：raptor 的 max_cluster / tree_builder / clustering_method；
    //          wiki 的 synthesis example；mindmap 的 prompt 覆盖等
    Extra map[string]any

    // 历史去重开关：wiki 变体在 document graph 阶段并行读取 ES KNN 候选。
    EnableHistoricalDedup bool

    // 容量护栏（缺口 A，防 OOM/GC 抖动）：限制单次 Invoke 产物规模，见 §4 Outputs。
    Guardrails CapacityGuardrails
}

// CapacityGuardrails 单次 Invoke 产物容量上限；任一超限即触发对应处置（error / flush）。
type CapacityGuardrails struct {
    MaxItems       int         // 产物条目上限（默认按变体给安全值，如 structure 5e4、raptor 按树规模）
    MaxVectorBytes int64       // 向量总字节上限（= Σ len(vec)*4）
    MaxOutputBytes int64       // 全部产物序列化字节上限
    OnExceed       ExceedAction // "error" | "flush"
}

// ExceedAction 超限处置：error=返回 ErrCapacityExceeded；flush=经 ChunkedSink 分段交付。
type ExceedAction string
```

- **变体命名口径（统一主名 + alias，缺口 F）**：组件内部 `Param.Variant` 与 `Run` 分派键一律用
  **`structure` / `wiki` / `raptor` / `mindmap` / `datasetnav`**（无下划线，Go 风格）。
  DSL 模板若填旧名 `mind_map` / `dataset_nav`，在 `Param` 构造处做 **alias 映射 + 弃用日志**
  （`mind_map→mindmap`、`dataset_nav→datasetnav`），其余未知值返回明确 `ErrUnknownVariant`。
  里程碑描述里的 `mind_map`/`dataset_nav` 仅为人读别名，代码与模板一律用主名。

- `Inputs()`：返回 `chunks`(**直接来自上游 chunker/parser 的输出**，每项带 `id`+`text`/`content_with_weight`，
  见 §1.1，**不再从 ES 读源 chunks**)、`llm_id`(可选覆盖)、`embedding_model`(可选覆盖)、变体特有键。
  - `HistoricalCandidates []Candidate`(**可选覆盖路径**)：用于测试或离线模式；若未提供且
    `EnableHistoricalDedup=true`，wiki 变体从 ES 并行检索历史候选（KNN）。
    命中在组件内直接 merge（merge 统计仅留存于组件内部）。
  - `Sink ChunkedSink`(**可选**)：当 `CapacityGuardrails.OnExceed=="flush"` 时，组件边产边回调
    分段交付，避免 `Outputs()` 一次性持有全部产物（缺口 A）。
- `Outputs()`：**回传 `chunks`**（与上游 chunker 输出 schema 完全一致）——
  各变体产物（structure 图行 / wiki page / raptor summary / mindmap tree /
  datasetnav 节点）转换为 `schema.ChunkDoc`（对齐 `conf/infinity_mapping.json`，以 `compile_kwd`
  区分普通分块），与上游输入 `chunks` 合并后作为 `Outputs()` 的 `chunks` 返回；运行内去重/历史合并
  统计不再外显到输出 surface（仅在组件内部 `common.Outputs` 中留存）。
  **容量护栏**：若 `Guardrails.OnExceed=="error"` 且超限，`Invoke` 返回 `ErrCapacityExceeded`；
  若 `=="flush"`，产物经 `ChunkedSink` 已分段交付。持久化与任何普通 chunk 一致，由调用方决定。
- `Invoke(ctx, inputs)`：解析 `variant` → 调 `structure.Run` / `wiki.Run` / `raptor.Run` /
  `mindmap.Run` / `datasetnav.Run`。每个子包暴露 `Run(ctx, deps, param, inputs) (map[string]any, error)`，
  共享 `common.Deps{ Chat ChatInvoker; Embed Embedder; Tokenizer; HistoricalKNN; ... }`。
  各 `Run` 内部用 `common.MemStore` 做运行内去重；wiki 变体额外并行调用 `HistoricalKNN` 做历史候选标记，
  结束时 `Snapshot()` 进 outputs。
- `init()`：`runtime.MustRegister(componentNameKnowledgeCompiler, runtime.CategoryIngestion, NewKnowledgeCompilerComponent, Metadata{...})`。

### 依赖注入 seam（沿用 embedder 模式）
`KnowledgeCompiler` 不直接构造模型，而是持有一个 `common.DepsResolver`：
在 `internal/ingestion/task` 的 `init()` 里把生产 resolver 注入
（`GetChatModel(tenantID, llmID)`、`DefaultEmbedderResolver`、`HistoricalKNNResolver`；
datasetnav 若需 Redis 锁再单独注入 `RedisClient`，见 §5 M8）。
测试时替换 resolver 注入 mock。组件仅执行历史候选读取，不在组件内写 ES；落库仍由 pipeline 调用者处理。

### 4.1 输出合约（chunk 对齐，缺口 D 演化）

组件**不落库、也不再通过独立的 writer seam 写 ES**：编译产物在 `Invoke` 期间驻留内存，
结束时转换为 `schema.ChunkDoc`（对齐 `conf/infinity_mapping.json`，以 `compile_kwd` 区分普通
分块），与上游输入 `chunks` 合并后作为 `Outputs()` 的 `chunks` 返回，schema 与上游 `chunker`
完全一致。行结构冻结项（M1 即与组件输出 schema 同步）：

- **行结构 schema**：每条产物即一个 `schema.ChunkDoc`，含 `id`(xxhash 稳定)、`doc_id`/`tenant_id`、
  `text`/`content_with_weight`、`compile_kwd`(=`variant`)、`parent_kwd`(树形变体)、`q_<dim>_vec`(向量)、
  以及 `kc_*` 前缀保留原始 `meta`(kind/level/name/size)。
- **幂等键**：`(tenant_id, doc_id, variant, id)` 唯一；若需落库，调用方以 upsert 语义写入，重复 id 覆盖不新增。
- **历史去重职责**：wiki 变体在组件内执行**并行历史候选检索 + 直接 merge**（ES KNN 只读），merge 统计仅留存
  于组件内部 `common.Outputs`，不再外显到输出 surface。
- **错误语义**：容量超限(`ErrCapacityExceeded`)、未知变体(`ErrUnknownVariant`)、LLM/embed 失败均透传。

---

## 5. 分阶段实施

- **M1 脚手架 + common 底座**
  `knowledge_compiler/common/`：`id.go`(xxhash 稳定 id)、`embed.go`(Embedder 包装)、
  `tokenize.go`(tokenizer + num_tokens)、`batch.go`(`build_chunk_batches` + `run_chunked_pipeline`)、
  `memstore.go`(内存产物存储 + 内存向量检索 `Add`/`Upsert`/`Delete`/`TopK`/`Snapshot`)、
  `localize.go`、`batch_packing.go`(内存 chunks → LLM token-budget 批次，原 `split_chunks`)。
  单测：mock Embedder + `memstore` 的 `TopK` 精度/阈值断言；
  **新增容量护栏单测**（缺口 A）：注入超大 chunks 集，断言 `CapacityGuardrails` 的 `error`/`flush`
  两条路径（`ErrCapacityExceeded` 与 `ChunkedSink` 分段回调次数），作为大文档内存压力的最小防护。

- **M2 JSON chat helper + 并发原语**
  `common/jsonchat.go`（gen_json 等价：JSON 模式请求 + 解析 + 重试 + 超时）、
  `common/pool.go`（LLMCallPool 等价：优先级信号量）。
  单测：注入 canned JSON 响应。

- **M3 structure 变体（编译 + 内存去重）**
  `structure/compile.go`：`compile_structure_from_text`（list/set/hypergraph 提示词渲染、
  hypergraph 的 node→edge 两阶段、产物行构造）、`merge.go`：`merge_compiled_structures`
  的本地去重（余弦候选 + LLM merge-pair + 别名重写 relation 端点 + 紧凑图 JSON），全程走 `common.MemStore`。返回 docs 供断言。

- **M4 structure 内存合并闭环**
  `merge.go` 接 `memstore.go`：**内存 `TopK` 候选查找**（替代 ES KNN）、分组 LLM 判定、
  批量合并、`rebuild_structure_graph_json`，最终 `Snapshot()` 全部产物进 `Outputs()`。
  单测用 `memstore` 双跑验证 inserted/updated/duplicates_dropped 计数（无需 DocEngine）。

- **M5 wiki 变体**
  `wiki/`：`map_from_chunks` → `reduce_from_extracts` → `plan_from_reduction` →
  `refine_from_plan`，含 5 个 `WIKI_*_COMPILE_KWD` 与 synthesis 阶段（example + compile_kwd 覆盖）。
  在 document 级 graph（entity/relationship）产出后，启用并行历史去重：内存 `TopK` + ES KNN 候选，
  命中后组件内直接 merge 并写入统计（`historical_merged`）。

- **M6 raptor 变体**
  `raptor/`：先交付 **Psi builder（仅余弦矩阵，零聚类依赖）** + **classic(AHC + PCA 降维)**；
  AHC 仍需降维器（见缺口 #3：`clustering()` 无条件先 UMAP，Go 用 **gonum PCA** 替代）。
  聚类后端：Ward AHC + gap 检测 + `_adjust_tree_nodes` 全部**纯 Go + gonum** 自实现；
  GMM/BIC 留 `ClusteringMethod` 可插拔接口与 TODO（纯 Go diagonal-GMM，后补，不阻塞 M6）。
  `summarize_texts` 走 `jsonchat`/普通 chat + embed。
  **验证门禁（缺口 C，替代原「保真度可接受」弱约束）**：M6 落地前先跑 Python `raptor.py` 在固定样本集上
  产出一个**基线快照**（簇标签、树高、叶覆盖、summary 文本），Go 版须对齐：
  - 指标：与基线对比 **NMI/ARI**（簇一致性）、**平均/最大树高**、**叶节点覆盖率**(≥基线 95%)；
  - **离线召回**：固定 query 集的 top-k hit 率回退不超过设定阈值（如 5%）；
  - **门禁失败回退**：classic(AHC) 任一指标不达标即**禁用 classic、仅启用 Psi**（Psi 零聚类依赖、风险更低），
    并告警，不阻塞发布。
  （开放问题 3：需确认是否存在可固化的 Python/Go 聚类一致性回归样本集；若无，先用合成 embedding 构造。）

- **M7 mindmap 变体**
  `mindmap/`：goldmark 解析 + `dictify` 语义转换 + 多段合并（`_merge`/`_be_children`）。

- **M8 datasetnav 变体**
  `datasetnav/`：Redis 锁封装 + 分层 KNN 下降 + LLM merge/summary + 增量 split/rebalance。
  单测用 miniredis 或内存锁。

- **M9 注册 + 模板 + 集成测试**
  在 `internal/ingestion/pipeline/template/` 新增一个使用 `KnowledgeCompiler` 的
  pipeline DSL（或一个参数化模板按 variant 切换），下游接普通 chunk 落库路径（组合根 / ES 写入）
  （消费 `KnowledgeCompiler` 的 outputs 产物）；`builtin_registry` 自动加载；
  编写端到端用例：断言组件 outputs 产物正确，落库交由 pipeline 层验证。

---

## 6. 测试与验证

- 每个子包单测：`mockEmbedder`、`mockChatInvoker`（返回固定 JSON）、`common.MemStore`（真实内存实现）。
- 包级运行（遵循 AGENTS.md，必须用 build.sh 以拿到 CGO/原生库）：
  ```bash
  bash build.sh --test ./internal/ingestion/component/knowledge_compiler/...
  ```
- 行为对齐（**structure + wiki 为 CI 必选门禁；raptor/mindmap/datasetnav 为可选 golden**）：
  用同一组 chunks 跑 Python 与 Go，对比 `compile_structure_from_text` 产物行数量与 `merge` 后
  inserted/updated 计数；wiki 额外对比 `reduce`/`refine` 关键阶段产物形状。未达标**阻断合并**。
- 不默认跑全量 `go test`；只跑本组件子树。

---

## 7. 风险与决策

1. **聚类保真度（最高风险）**：sklearn GMM/UMAP 在 Go 无直接等价。
   决策已定：**纯 Go + `gonum`，不引入 `pa-m/sklearn`、不用 CGO/C++ 内核**。
   - AHC 路径**仍依赖降维**（Python 分支前无条件跑 UMAP，`raptor.py:326`），Go 用 **gonum PCA** 替代；
     PCA 输出的欧氏空间更契合 Ward，保真度可接受，需在 M6 单测固化聚类数/阈值。
   - GMM 是 `covariance_type="diag"`（`raptor.py:261,351`），diagonal-GMM EM+BIC 约 200-300 行，
     纯 Go 可行，留作可插拔后端后补，不阻塞 M6。
   - Psi builder 仅用余弦矩阵，不在此风险内。
   结论：M6 先交付 **AHC + Psi（AHC 走 PCA 降维；Psi 仅依赖余弦）**，GMM/UMAP 作为可插拔聚类后端后续补。
   **验证闭环见缺口 C**：M6 须以 Python 基线快照做 NMI/ARI/树高/叶覆盖/离线召回门禁，失败则回退 Psi-only。
2. **历史去重一致性**：Python 用 ES `MatchDenseExpr`（近似 KNN）在全量已存产物上查候选；
  Go 版改为「运行内 memstore 精确余弦 + wiki 历史 ES KNN 并行候选」。关键点：
  - 运行内去重：`MemStore.TopK` 精确余弦；
  - 历史去重（wiki）：`HistoricalKNN.TopKHistory`（ES 近似 KNN）按 `tenant_id/dataset_id/variant` 过滤；
  - 历史命中后组件内直接 merge，落库幂等兜底由调用方负责。
  结论：不再将其定义为“仅本次运行去重”的 breaking change；行为更接近 Python，但仍保留
  `enable_historical_dedup` 开关用于灰度与回退。
3. **依赖方向**：组件包不得 import `internal/service`；所有 resolver 在 `internal/ingestion/task` 注入。
4. **并发语义**：Python 的 `asyncio.Semaphore` + 取消检查 → Go 用 `context.Context` + `errgroup`/信号量，
   取消用 `cancel_check` 包成 `ctx.Err()` 检查。
5. **多语言/模板字段**：config 的 `en`/`zh` 多语言与 `entity`/`relation` 新模板形状需完整映射，
   避免只覆盖旧 `output.entities/relations` 形状。

---

## 8. 建议目录树

```
internal/ingestion/component/knowledge_compiler/
├── PORT_PLAN.md
├── component.go            // KnowledgeCompilerComponent + Param + Invoke + init 注册
├── deps.go                // Deps / DepsResolver 接口与注入 seam
├── common/
│   ├── id.go              // stable_row_id (xxhash)
│   ├── embed.go           // Embedder 包装
│   ├── tokenize.go        // tokenizer + num_tokens + localize
│   ├── batch.go           // build_chunk_batches + run_chunked_pipeline
│   ├── batch_packing.go   // 内存 chunks → LLM token-budget 批次 (原 split_chunks)
│   ├── memstore.go        // 内存产物存储 + 内存向量检索 (Add/Upsert/Delete/TopK/Snapshot；不碰 ES)
│   ├── historical_knn.go  // wiki 历史候选检索接口/实现（只读 ES KNN）
│   ├── jsonchat.go        // gen_json 等价
│   └── pool.go            // LLMCallPool 等价
├── structure/  (compile.go, merge.go, prompt.go, graph.go, chain.go)
├── wiki/      (map.go, reduce.go, plan.go, refine.go)
├── raptor/    (raptor.go, clustering.go, psi.go)
├── mindmap/   (mindmap.go, dictify.go)
└── datasetnav/(nav.go, lock.go, split.go)
```

---

## 9. 交付顺序建议（最小可用路径）

`M1 → M2 → M3 → M4`（structure 变体端到端可用）
→ `M9` 注册 + 模板（让 pipeline 能跑 structure）
→ 之后再按 `M5/M6/M7/M8` 逐个补齐 wiki / raptor / mindmap / datasetnav，
每补一个变体就扩展 `Param.Variant` 与对应 `Run`，保持单一组件、单一注册入口。

**发布策略（回答开放问题 2）**：推荐**分阶段发布**——M9 先只上线 `structure` 变体并默认启用；
`wiki`/`raptor`/`mindmap`/`datasetnav` 经 feature flag 逐步放开，其中 `raptor` 的 classic(AHC)
受 M6 验证门禁约束（不达标则仅启用 Psi），避免低质量摘要树进入召回。
