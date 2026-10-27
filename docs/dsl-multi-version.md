# DSL 多版本设计（Ingestion Pipeline / Dataflow Canvas）

> 状态：v5.4（在 v5.3 基础上追加：§1 C5 / §G3 空版本=红色 Missing + Save 报“版本信息缺失”，dataset/file 两层；仍为锁定设计，TDD 实现中）
> 约束：单 PR、严格 TDD、迁移为幂等 Go 函数、Missing 运行时校验不加字段、版本归属校验禁止悬空。

---

## 1. 目标与范围

为 ingestion pipeline（画布类别 `dataflow_canvas`）的 DSL 增加**多版本**能力：

- **A1** DSL 变更在数据库增量写入，版本号即时间（`user_canvas_version` 已存在的基础设施）。
- **A2** 新增版本别名 `alias`（后端仅存储，前端过滤，不涉及后端逻辑）。
- **B1** 画布可查看多版本；**B2/B2.1** 删除版本需弹窗二次确认（告知可能导致 File 解析不可用）；**B3** 手动 Save（用户明确点击保存）→ 生成新版本；自动保存（非手动）→ 仅更新 `user_canvas.dsl` live draft，不生成新版本（v5.2 细化，修正原「改 DSL 即生成版本」）；**B4** 画布修改**不向下传播**到已绑定的 dataset/file（粘性快照）。
- **C1** dataset/file 配置可选版本；**C2** 默认绑定“选择时刻最新具体版本”（锁定非浮动），可手动选；**C3** 切版本 → 参数重置为该版本 default。
- **C4** 版本删除 → 引用方 Missing，file 不可解析；**D1/D2/D3** 非最新显示 `!`，点击仅提示不引导升级；**E1** 列表新→老；**F1** 别名前端过滤。
- **C5** 版本信息为空（`pipeline_version_id` 为空 / 未选版本）→ UI 显示红色 `Missing`（缺失）徽标；用户点击 Save 时校验并报错“版本信息缺失”。dataset 与 file 两层均适用（见 G3）。空版本不再自动回填为最新版本。
- **G1** dataset/file 版本选择 UI：不渲染 `latest` 伪选项，仅列真实版本，选项文案须含 `alias`（如 `alias (title)`）；已绑定即存具体版本 ID，无 latest 概念（呼应 C2）。
- **G2** alias 写入/更新：后端经版本级 `PATCH` 端点（及可选随 manual Save 携带）写入 `user_canvas_version.alias`；前端提供编辑入口（见 §4.4、§8）。
- **G3** 空版本（`pipeline_version_id` 为空 / 未选版本）处理：UI 显示红色 `Missing`（缺失）徽标，与「版本被删」的 Missing 同一种视觉（见 §3 扩展）；用户点击 Save（创建/更新 dataset、或更新 file 绑定）时，后端**拒绝**并校验报错“版本信息缺失”（HTTP 4xx + 文案“版本信息缺失”），不写入、不自动回填为最新版本（呼应 C5）。dataset 与 file 两层均实现此校验（C5 双层要求）。

**不在范围**：agent（画布类别 `agent`）的发布语义、built-in parser（`parse_type=1`）的版本化、Python 端（按 AGENTS.md 正在移除）。

---

## 2. 当前状态核查（已核实，非臆测）

### 2.1 版本化基础设施已存在
- 表 `user_canvas_version`，实体 `entity.UserCanvasVersion`：`ID, UserCanvasID, Title, Description, Release bool, DSL, BaseModel`。**需加 `Alias *string`**。
- `internal/dao/user_canvas_version.go`：
  - `SaveOrReplaceLatestTx`：DSL 变更才建新版本；同 DSL 且 `!latest.Release || opts.Release` 时就地更新最新行（对 dataflow 即同 DSL 重复保存不新建版本）。
  - `ListByCanvasID`：`ORDER BY create_time DESC` —— 满足 E1（新→老）。
  - `GetLatest`：`create_time DESC` 取最新版本。
  - `GetLatestReleased`：仅 `release = true` 的版本。
  - `DeleteTx` / `DeleteByCanvasIDTx` 已存在。

### 2.2 发布动作的真相（关键修正）
- `web/.../index.tsx:350` 无条件挂载 `<PublishConfirmDialog>`，但 `publish-confirm-dialog.tsx` 内部有 `if (isPipeline) { return null; }`（`isPipeline` 由 `useIsPipeline()` 判定 URL 分类为 `DataflowCanvas`）。**结论：发布按钮是 agent 专用，ingestion pipeline（dataflow）没有发布动作。**
- `use-save-graph.ts` 带 `release=true` 的 `saveGraph(...,true)` 仅来自 `onPublish`，而该回调在 pipeline 下永不触发；dataflow 所有保存（含 20s 自动保存）`release` 均为 `undefined`。
- 后端 `SaveOrReplaceLatestTx` 对 dataflow 写入的版本 `Release` 恒为 `false`。
- 因此 **`GetLatestReleased` 对 dataflow 永远返回未找到** → dataflow 无 draft/published 双态，**所有版本都是草稿**。
- **（v5.2 勘误/延伸）** 尽管 dataflow 全部保存 `release` 均为 undefined，但 `SaveOrReplaceLatestTx` 在 DSL 发生变更时会 INSERT 新版本行——即**当前自动保存（20s 去抖）也会生成新版本**。§5.1 将把版本创建收敛到「仅手动 Save」。agent 与 dataflow 共用同一保存入口（`web/src/pages/agent/index.tsx` → `UpdateAgent`），故该修正作用于共享路径。

### 2.3 DSL 加载的两个真相源（版本化要改的核心）
- API 侧：`internal/service/pipeline_params.go` 的 `loadCanvasDSLJSON(ctx, canvasID)` 读 `user_canvas.dsl`（永远最新），被 `LoadPipelineDSL` 与 `ResolveComponentParamsDefaults` 复用。
- 运行时：`internal/ingestion/task/pipeline_executor.go` 的 `loadDSLFromCanvas(ctx, canvasID)`（~L1010）读 `user_canvas.dsl`（永远最新），返回 `(dsl, canvasID, err)`。其类型 `loadDSLFunc` 签名仅 `(ctx, canvasID)`，**破坏性变更需加版本**。

### 2.4 绑定落库路径（写入粘性快照的落点）
- dataset：`internal/service/dataset/crud.go` `CreateDataset`、`internal/service/dataset/update.go` `UpdateDataset`。
  - `service.FromRequest(parseType, parserID, pipelineID)` 构建 `ParserSelection`；`sel.IsBuiltIn()` 时 `req.PipelineID = nil`（清掉）；Pipeline 模式下 `pipelineID` 是 `user_canvas.id`（经 `normalizeDatasetPipelineID` 校验 32 hex）。
  - `update.go` 中 `service.LoadPipelineDSL(ctx, isPipeline, effParserID, effPipelineID)` 加载 DSL；`pipelineChanged := pipelineID != nil && (lockedKB.PipelineID == nil || *pipelineID != *lockedKB.PipelineID)`。
- document（文件级）：`internal/service/document/file2document.go` 写入 `PipelineID`；`document.go` 有 `PipelineID *string`。单个文件可单独改 pipeline（已确认）。

### 2.5 运行时上下文
- `internal/ingestion/task/task_context.go` 的 `TaskContext` 仅有 `PipelineID string`；`resolvePipelineID` 需加 `resolvePipelineVersionID`。
- ad-hoc/调试路径（`pipeline_executor.go` ~L885-916，`input.PipelineID`）也需走版本校验（见 §12 A7）。

---

## 3. 锁定决策

1. **迁移 = 幂等 Go 迁移函数**（`internal/dao/migration.go` 的 `RunMigrations` 集中调用），参考先例 `migrateKnowledgebaseParserConfig`（FindInBatches + 版本标记）。**不**用独立 SQL 脚本。
2. **Missing = 运行时校验**，不额外增加字段、不查“是否最新”。两种成因都归为 Missing：
   - **版本被删 / 悬空**：绑定版本 `pipeline_version_id` 指向的 `user_canvas_version` 不存在（或被删、或跨 canvas）→ Missing。
   - **空版本**：`pipeline_version_id` 为空 / 未选版本（dataset 或 file 创建/更新时未带版本）→ 同样显示红色 `Missing` 徽标（见 G3/C5）；保存时后端校验报错“版本信息缺失”，不写入、不自动回填（见 G3）。
   空版本与「被删版本」是 Missing 的两种成因，前端视觉统一为红色 `Missing`，区别仅在于成因提示文案（被删=提示重选版本；空版本=提示“版本信息缺失”）。
3. **版本归属校验**：禁止悬空版本号。`pipeline_version_id` 必须属于绑定 `pipeline_id` 对应的 `user_canvas`（且类别为 `dataflow_canvas`）；否则视为无效。

### H12 修订（v4 修正）
- 因 dataflow 无发布语义，`!`（非最新）基线改用 **`GetLatest`**（按 `create_time DESC`），**不**用 `GetLatestReleased`。
- `backfillDSLVersionBindings` 对每个绑定了 dataflow-canvas pipeline 的 dataset/file，取 **`GetLatest(canvasID).ID`** 写入 `pipeline_version_id`；`GetLatest` 不存在（脏数据）则跳过 + 日志告警（绝不写悬空 version_id）。

---

## 4. 数据模型变更

### 4.1 `entity.UserCanvasVersion`
新增字段：
```go
Alias *string `gorm:"column:alias" json:"alias,omitempty"`
```

### 4.2 `entity.Dataset` 与 `entity.Document`
各新增可空字段：
```go
PipelineVersionID *string `gorm:"column:pipeline_version_id;size:32;index" json:"pipeline_version_id,omitempty"`
```
（built-in parser 绑定时该字段为 `NULL`，不启用。）

### 4.3 迁移函数（新增 `internal/dao/migration.go`）
- `migrateDSLVersionColumns`：给 `knowledgebase` 与 `document` 表加 `pipeline_version_id` 列（用 `HasColumn` + ALTER，参考 `migrateIngestionTaskPipelineLogID` 的 `Error1060` 容错）。
- `backfillDSLVersionBindings`：幂等（用 `system_settings` 版本标记防重跑）。`FindInBatches` 遍历 `knowledgebase`/`document` 中 `pipeline_id IS NOT NULL AND pipeline_id <> ''` 的行；仅当 `user_canvas` 存在且 `canvas_category = dataflow_canvas` 时，写 `pipeline_version_id = GetLatest(canvasID).ID`；built-in / 类别不符 / 无版本 → 跳过。

---

### 4.4 `alias` 写入/更新路径（v5.2 新增）

- **现状（已核实）**：
  - `entity.UserCanvasVersion.Alias` 已存在（`internal/entity/canvas.go:67`，gorm 列 `alias`）。
  - DAO `SaveOrReplaceLatestTx` 在新建/就地更新两分支均写 `alias`（`internal/dao/user_canvas_version.go:313-315` 更新分支、`:328` 新建分支）；有单测 `TestUserCanvasVersionDAO_SaveOrReplaceLatestTxStoresAlias` 覆盖。
  - handler `publishAgentRequest.Alias *string`（`internal/handler/agent.go:701`）仅经 `POST /publish` 透传；但 `UpdateAgent` 路径调用 `saveOrReplaceVersionOptions(..., nil, nil, release)` 显式传 `nil` alias，故普通保存**不写** alias。
  - 前端**无任何 alias 输入控件**：`publish-confirm-dialog.tsx` 对 dataflow 返回 null（无发布入口），agent 发布也不发 alias；所有前端 alias 命中均为只读过滤/显示。结论：**alias 写入路径后端完备但 UI 不可达，实际恒为 NULL**。
- **写入设计**：
  - 新增 `PATCH /api/v1/agents/{canvas_id}/versions/{version_id}`：body 含 `alias`/`title`/`description`，经 `versionDAO.UpdateTx` 更新该行元数据；**不**触发新版本、**不**改 DSL；需归属校验（`version.UserCanvasID == canvas_id`）。
  - `UpdateAgent` 保存请求**可选**携带 `alias`，随 manual Save 创建/刷新版本时一并写入（兜底路径，复用 `SaveOrReplaceLatestTx` 既有 alias 分支）。
- **前端**：版本历史对话框（`web/src/pages/agent/version-dialog/index.tsx`）增加 alias 编辑输入，提交至上述 PATCH；dataset/file 选择器保持只读过滤（G1/F1）。

## 5. 写入路径：粘性快照（首次默认最新、之后保留）

**语义**：绑定 pipeline 的那一刻，把“当时最新具体版本”的 `ID` 锁进 `pipeline_version_id`，后续画布再改不再影响已绑定方（B4）。

- **dataset**：`crud.go` `CreateDataset` 与 `update.go` `UpdateDataset` 中，当 `sel.IsPipeline()` 且 `pipelineID` 有效时：
  - 若本次是**新绑定 / pipeline_id 变更**（`pipelineChanged`）：`pipeline_version_id = GetLatest(canvasID).ID`（锁定当时最新）。
  - 若 pipeline_id 未变（仅其它字段变更）：**保留**原 `pipeline_version_id`（不随画布漂移）。
  - 归属校验：确认 `canvas_category = dataflow_canvas`，否则报错（决策 §3-3）。
- **document**（文件级）：`file2document.go` 同理，绑定/变更 pipeline 时写 `pipeline_version_id = GetLatest(canvasID).ID`；未变更则保留。
- **手动选版本（C2）**：新增请求字段 `pipeline_version_id`（可选）。传入时校验其属于该 `pipeline_id` 的 canvas，写该值（取代“默认最新”）。
  - **handler 字段白名单（v5.3 补充）**：`internal/handler/dataset.go` 的 `createDatasetAllowedFields` 与 `updateDatasetAllowedFields` 必须放行该字段，否则请求在 handler 层即被拒（code 101 `Extra inputs are not permitted`），service 层永远收不到。该白名单是独立于 service 校验的**第二道闸**，仅显式登记的字段可通过；新增 `pipeline_version_id` 须同时登记到创建与更新两个白名单（PR #21261 已落地，对应 `dataset_allowed_fields_test.go`）。
- **C3 切版本重置参数**：当 `pipeline_version_id` 被显式改为另一版本时，调用 `ResolveComponentParamsDefaults(ctx, parserID, &canvasID, versionID)` 取得该版本 default，将 dataset/document 的 `parser_config`（component params）重置为该 default（范围见 §12 A3）。
- **回落契约（v5.1 新增，必读）**：`pipeline_id`（画布容器）与 `pipeline_version_id`（容器内快照）是「容器⊃快照」关系，`pipeline_version_id` 必须在 `pipeline_id` 的上下文里解析。任何解析都必须遵守下面两条回落链，缺一不可，否则会出现「钉被清空 / 误用最新 / 误用 builtin 默认」三类缺陷：
  - **版本回落链**：`req.PipelineVersionID` → `lockedKB.PipelineVersionID`（已钉版本，不可跳过）→ `GetLatest(canvas)`。请求未带版本时，**必须先回落到已钉版本**，仅当已钉版本也为空才用 latest。绝不允许「请求没带就直接取 latest」——那会让「钉旧版本」的数据集在改配置时被悄悄升级到最新结构。
  - **canvas 上下文回落链**：解析 `pipeline_version_id` 所需的 canvas = `req.PipelineID` → `lockedKB.PipelineID`（已绑画布）→ nil（builtin）。请求省略 `pipeline_id`（只发 `pipeline_version_id`）时，**必须沿用已绑 `PipelineID`**，不得因省略而让上下文变 nil；否则归属校验失效、`pipeline_version_id` 被误清空、C3 重置误用 builtin 默认。
  - **parser_config 重建一律基于钉住版本的 DSL**：无论因何触发（请求带 `parser_config`、切版本、pipeline 变更），重建所用 DSL 必须来自 `pipeline_version_id` 指向的版本；仅当 `pipeline_version_id` 为空时才回落 `GetLatest`。「保留版本号」与「用该版本 DSL 重建」是同一语义，不可割裂——只保留列而用 latest DSL 重建属于未完全实现粘性快照（B4）。
  - **单字段 PATCH 必须沿用已绑值**：update 允许只发 `pipeline_version_id` 省略 `pipeline_id`（或反之）。被省略的字段一律沿用 `lockedKB` 已绑值，不得清空或误判。

---

## 5.1 画布保存语义：手动 Save vs 自动保存（v5.2 新增）

**背景**：agent 与 dataflow 画布共用同一保存入口——`web/src/pages/agent/index.tsx` 的 Save 按钮（`handleSave` → `saveGraph()`）与 `useWatchAgentChange` 的 20s 去抖自动保存，二者皆落到 `service.UpdateAgent` → `SaveOrReplaceLatestTx`。当前只要 DSL 变更即 INSERT 新版本行，故**自动保存也会刷版本**，违背「版本=手动保存里程碑」语义。

**信号**：前端自动保存请求增加 `autosave: true`（手动 Save 不带，或显式 `autosave: false`）。`updateAgent` 请求体（`web/src/services/agent-service.ts`）与 `PUT /api/v1/agents/:canvas_id` 透传该字段；`service.UpdateAgent` 据此分支。

**分支规则（统一作用于 agent 与 dataflow，因共用 `UpdateAgent` 路径）**：
- `autosave == true`：**仅** `canvasDAO.UpdateFieldsTx` 更新 `user_canvas.dsl`（live draft），**不**调用 `SaveOrReplaceLatestTx`，**不**产生新版本。
- `autosave == false/缺省`（手动 Save）：**必须同时发生两件事**——① `canvasDAO.UpdateFieldsTx` 更新 `user_canvas.dsl`（live draft）；② 版本写入 **INSERT 一条新的 `user_canvas_version` 记录**。**v5.2 修正：不再做『同 DSL 就地刷新』去重**——手动 Save 即落一版，即便 DSL 与上一版本完全相同也 INSERT 新行（保证每次手动保存都产生新版本）。实现上 manual Save 应走强制 INSERT 路径（如新增 `SaveNewVersionTx(canvasID, dsl, alias, title, description)` 或给 `SaveOrReplaceLatestVersionOptions` 加 `ForceNew bool`），绕过 `sameDSL` 就地刷新分支；自动保存则不进入该路径。

**agent 发布语义不受影响**：`PublishAgent`（`release=true`）路径独立，仍创建 released 版本，本次不改（与 §1「agent 发布语义不在范围」一致）。

**范围裁定（A9，用户已确认 2026-10-22）**：因 agent/dataflow 共用 `UpdateAgent`，修正必须落在共享路径、对所有画布类别统一生效；**手动 Save 对 agent 与 dataflow 一致：更新 `user_canvas` + INSERT 新 `user_canvas_version`**；agent 的发布语义（Publish）不受影响。agent 手动 Save 仍建版本（行为不变），仅自动保存收敛，属良性改进且不触及 §1 范围。

**粘性快照不受影响**：新建版本只改 `user_canvas_version`，已绑定 dataset/file 的 `pipeline_version_id` 仍为粘性（B4）；live draft 领先于最新版本属预期（手动 Save 后方才落成版本）。

## 5.2 版本上限与保留策略（v5.3 新增）

**动机**：用户可经手动 Save（§5.1）反复创建版本，需有一个**可预期、有界**的存储上限，同时**绝不**在写路径上静默删除用户的旧版本（旧设计曾设想「未发布 ≤ 20 自动裁剪」，经评审否决——裁剪会丢失用户可能需要的旧版本，故改为 keep-all）。

### 5.2.1 每画布硬上限 K=64

- 常量 `MaxCanvasVersions = 64`（位于 `internal/dao/user_canvas_version.go`）。计数针对**单个 `user_canvas_id` 下的全部 `user_canvas_version` 行**，不区分 `release` 状态（发布的与未发布的草稿一并计入）。
- 写版本前 `SaveOrReplaceLatestTx` 先 `COUNT(*)` 该 canvas 现有行数：
  - 当 `existing < MaxCanvasVersions`：允许写入（新增 INSERT 或就地刷新）。
  - 当 `existing >= MaxCanvasVersions`：拒绝本次写入，返回 `ErrCanvasVersionLimitExceeded`（文案：**「版本数已达上限（最多 64 个），请删除旧的不再使用的版本」**），由 `mapAgentError` 映射为 `common.CodeDataError` 透传原话给前端弹窗。
- **计数为「总量」而非「未发布」**：发布版本（agent `PublishAgent` 的 `release=true` 行）同样占名额。达到 64 后，无论手动 Save 新草稿还是发布新版本，都会被上限拦截，必须用户主动删除腾位。
- **隔离性**：上限按 `user_canvas_id` 维度独立计算，画布 A 写满不影响画布 B（见单测 `TestCanvasVersionLimit_PerCanvasIsolation`）。

### 5.2.2 不消耗名额的写入形态

以下两种写入**不计入 / 不触发上限判定**，是「更新既有版本」而非「新增版本」：

- **就地 DSL 刷新**（`ForceNew=false` 且 `sameDSL` 命中）：`SaveOrReplaceLatestTx` 命中同 DSL 分支时只 `UPDATE` 最新一行（如 alias/title/description 元数据），行数不变，不消耗名额（单测 `TestCanvasVersionLimit_RefreshInPlaceAllowedAtCap`）。
- **元数据 PATCH**：服务层 `PatchVersion`（独立 update 路径）仅改 `alias`/`title`/`description` 等字段，不增行，不消耗名额（单测 `TestCanvasVersionLimit_AliasPatchUnaffected`）。

> 上限只在「实际新增行」时起作用。只要不 INSERT 新行，即便已达到 64 也能继续刷新最新版本或改元数据。

### 5.2.3 keep-all：写路径不做任何自动裁剪

- **彻底删除**原 `deleteAllUnpublishedExcessTx` 及其「未发布 ≤ 20 裁剪」逻辑。写版本路径**不再因总数过高而删除任何既有行**。
- 版本行的唯一删除入口是用户**显式**操作：
  - 前端版本历史对话框的删除按钮 → `DeleteVersion`（带归属校验、引用扫描 `CountPipelineVersionReferences`，防止删掉仍被 dataset/file 绑定的版本）。
  - agent 的版本清理（如适用）走显式接口，非写时隐式裁剪。
- 因此「手动 Save 越多越占名额」，名额的回收**只能由用户主动删除**完成；K=64 是系统唯一的硬约束，不存在「写满后自动丢旧版」的隐藏行为（单测 `TestCanvasVersionKeepAllRetainsUnpublished`、`TestCanvasVersionKeepAllRetainsBeyondOldTrim`）。

**与上游 PR 的关系**：上游 #21343 曾引入 keep-all 但本 PR 早期版采用「未发布裁剪到 20」，现经评审统一为「keep-all + K=64 上限」——以本 PR 行为为准；上游 #21352 的 `ApplyVersion` 已采纳并保留。

## 6. 读取路径：版本透传 + Missing 校验

### 6.1 API 侧（`pipeline_params.go`）
- `loadCanvasDSLJSON(ctx, canvasID, versionID *string)`：
  - `versionID` 非空 → 经 `UserCanvasVersionDAO.GetByID` 取版本；**归属校验**：`version.UserCanvasID == canvasID`（否则视作 Missing）。
  - `versionID` 空（脏数据/未迁移）→ 回退 `GetLatest(canvasID)`；若仍无 → 返回 Missing 错误。
  - 读 `version.DSL` 替代 `canvas.DSL`。
  - 仅 `canvas_category = dataflow_canvas` 进入上述版本化路径；agent 类画布（`agent`/`agent_canvas`）按 §1 不在范围，直接读 `canvas.DSL`（live draft），不走版本化、不触发 Missing。
- `LoadPipelineDSL` 与 `ResolveComponentParamsDefaults` 透传 `versionID`（从 dataset/document 的 `pipeline_version_id` 取）。

### 6.2 运行时（`pipeline_executor.go`）
- `TaskContext` 增加 `PipelineVersionID string`；`resolvePipelineID` 增加 `resolvePipelineVersionID`（从 `doc.PipelineID`/`doc.PipelineVersionID` 解析）。
- `loadDSLFunc` 签名加 `versionID`；`loadDSLFromCanvas(ctx, canvasID, versionID)` 改为读 `user_canvas_version.dsl`（归属校验同上）。
- ad-hoc `input.PipelineID` 路径同样带 `versionID`（来源见 §12 A7）。
- **Missing 处理**：版本缺失/不归属 → 解析任务直接 fail（C4），错误信息含“pipeline 版本不存在，请重新选择版本”，file 进入不可解析状态；**不做**“是否最新”判断（决策 §3-2）。

### 6.3 `!` 非最新指示（D1/D2/D3）
- 列表/详情接口返回 dataset/document 时，附带 `pipeline_version_outdated: bool` =（`pipeline_version_id` 非空 且 ≠ `GetLatest(canvasID).ID`）。
- 前端显示 `!`；点击仅弹提示（D3：**不**引导升级、不自动改绑定）。

---

## 7. 服务/API 变更

- `agent.go` `UpdateAgent`（agent 与 dataflow 共用，经 `canvas_category` 区分）：`patch` 含 `dsl` 时调 `SaveOrReplaceLatestTx`（携 `release`，dataflow 恒 false）——**不改**。
- **handler 字段白名单放行 `pipeline_version_id`（v5.3 补充）**：`internal/handler/dataset.go` 的 `createDatasetAllowedFields` / `updateDatasetAllowedFields` 已登记 `pipeline_version_id`。这是“绑定具体版本”得以进入 service 层的前置条件——未登记时 create/update dataset 请求携带该字段会被 handler 直接以 `code 101` 拒绝，service 的粘性快照逻辑无从触发（见 §5 C2 注释）。该改动仅“显式放行目标字段”，未知字段仍按白名单拒绝，不退化成“全放行”。
- `DeleteVersion`：增加引用扫描 —— 查 `knowledgebase`/`document` 中 `pipeline_version_id = 待删版本ID` 的引用数；**允许删除**（B2.1 弹窗已二次确认），引用方因版本缺失自动进入 Missing（§6.2）。仅 dataflow 生效（引用扫描范围见 §12 A4）。
- 版本列表接口（`ListByCanvasID`，已按 create_time DESC 满足 E1）：返回含 `alias` 字段（A2/F1）。
- 版本详情/下载接口已存在（`version-dialog` 仅查看/下载，无删除/载入编辑入口 —— 前端需补 B2 删除按钮与 C2 选版本入口）。

---

- **保存分支（v5.2）**：`UpdateAgent` 按 `autosave` 字段分支（见 §5.1）；自动保存**仅** `UpdateFieldsTx` 更新 live draft、绝不碰 `user_canvas_version`；手动 Save **同时**更新 `user_canvas.dsl` 并 **INSERT 新 `user_canvas_version` 记录**（保证 user_canvas 更新与版本插入两者都发生，不复用就地刷新）。
- **版本 PATCH 端点（v5.2）**：`PATCH /api/v1/agents/{canvas_id}/versions/{version_id}`，body `{alias?, title?, description?}`，经 `versionDAO.UpdateTx` 更新元数据，不新建版本、不改 DSL；需归属校验（`version.UserCanvasID == canvas_id`）；仅对画布本体版本生效。

## 8. 前端变更

- `web/src/pages/agent/index.tsx`：`PublishConfirmDialog` 对 pipeline 返回 null（确认无发布动作，保持现状）。
- `web/src/components/pipeline-version-select/index.tsx`：**移除 `LATEST_VERSION_VALUE='__latest__'` 伪选项（G1，v5.2）**——删除 `:97-99` 的 `<SelectItem value={LATEST_VERSION_VALUE}>` 与 `:32/:43-49` 的 sentinel 映射（`formValueToSelectValue`/`selectValueToFormValue` 不再用 `__latest__`）；选项文案保留并强化 alias 显示（`v.alias ? v.alias + ' (' + v.title + ')' : v.title`）。无预绑定时默认选中最新具体版本（GetLatest），提交发送具体 `pipeline_version_id`，**空值不再表示 latest**。
- dataset 设置页（`web/src/pages/dataset/setting/index.tsx:294`）与文件对话框（`web/src/components/document-pipeline-dialog/index.tsx:193`）复用上述组件，自动生效；提交体 `pipeline_version_id` 恒为具体版本 ID。
- `web/src/pages/agent/index.tsx` + `use-save-graph.ts`：自动保存（20s 去抖）请求增加 `autosave: true`；手动 Save（`handleSave`）不带（v5.2，见 §5.1）。
- `web/src/pages/agent/version-dialog/index.tsx`：增加 alias/title 编辑输入，提交至新 `PATCH` 端点（G2，见 §4.4）；保留删除按钮（B2/B2.1）、`!` 提示（D1/D2/D3）、alias 显示与过滤（F1）。
- dataset 设置页：增加 pipeline 版本选择（C1/C2）与 `!` 指示（D1/D2/D3）。

---

## 9. TDD 测试清单（严格红→绿）
> **对抗性 fixture 强制规则（v5.1 新增，见 §13）：** 本特性的灵魂是 `pipeline_version_id`（钉住）可以 ≠ `GetLatest`（最新）。因此 §9 每一条涉及版本的测试**必须**使用 `pinned != latest` 的 fixture；并且必须为「耦合字段只发其一」的 PATCH 场景各写一条测试。happy-path（pinned==latest 且两字段齐发）**不算覆盖**，因为它永远发现不了 B1/B2 类缺陷。

**后端（unit tier，默认 `bash build.sh --test`）**
- T1 `migrateDSLVersionColumns`：列不存在时加列、已存在时幂等（Error1060 容错）。
- T2 `backfillDSLVersionBindings`：dataflow-canvas 绑定 → 写 `GetLatest().ID`；built-in 跳过；agent-canvas 跳过；`GetLatest` 缺失跳过+日志；重跑幂等（system_settings 标记）。
- T3 `loadCanvasDSLJSON` 带 versionID：取版本 DSL；归属不符 → Missing；空 versionID 回退 GetLatest。
- T4 `LoadPipelineDSL` 透传 versionID。
- T5 `ResolveComponentParamsDefaults` 按 version 取 default。
- T6 dataset `CreateDataset`：Pipeline 模式写 `pipeline_version_id=GetLatest`；BuiltIn 模式字段为 NULL。
- T7 dataset `UpdateDataset`：pipeline 未变更 → 保留原 versionID（B4）；pipeline 变更 → 重写为新 GetLatest；手动 versionID 生效且做归属校验。
- T7f（B2 对应）`UpdateDataset` **单字段 PATCH**：只发 `pipeline_version_id=v-new`、省略 `pipeline_id`，且数据集已钉 `v-old` → 断言 `pipeline_version_id` 变为 `v-new`（**未被清空**），且 `parser_config` 按 `v-new` 默认整体重置（C3/A3），归属校验以已绑 `PipelineID` 为上下文。
- T7g（B1 对应）`UpdateDataset` **钉旧版本 + 只改 parser_config**：数据集钉 `v-old`（≠latest），PATCH 只带 `parser_config`、不带 `pipeline_version_id` → 断言重建所用 DSL 来自 `v-old`（**非 latest**），`parser_config` 结构与 `v-old` 默认一致（粘性快照 B4 不被破坏）。
- T7h（v5.3 白名单）`createDatasetAllowedFields` / `updateDatasetAllowedFields` 含 `pipeline_version_id`：断言 create/update dataset 请求携带该字段不被 handler 白名单以 `code 101` 拒绝（端到端经此闸进入 service）；对抗：未知字段 `__bogus_field__` 在两白名单中仍为 false（白名单是“显式放行”而非“全放行”）。对应 `internal/handler/dataset_allowed_fields_test.go`。
- T8 document 绑定同样写 versionID（粘性）。
- T9 `DeleteVersion` 引用扫描：引用存在时允许删除、引用方 Missing；无引用正常删。
- T10 运行时 `loadDSLFromCanvas` 按 versionID 取 `user_canvas_version.dsl`；版本缺失 → 解析 fail（C4）。
- T11 `TaskContext.resolvePipelineVersionID` 从 doc 解析。
- T12 ad-hoc 路径带 version 校验。
- T13 归属校验：悬空/跨 canvas 的 `pipeline_version_id` → 拒绝/ Missing。
- T14 `!` 指示计算：绑定≠GetLatest → outdated=true；相等 → false。
- T15 alias 存储/返回（A2）。
- T16 built-in pipeline 不受任何版本逻辑影响（验收断言）。
- T17 迁移前存量数据可被 backfill 正确锁定。
- T18 `normalizeDatasetPipelineID` + 类别校验组合。

**前端**
- T19 版本列表新→老（E1）。
- T20 `!` 显示与“仅提示不引导升级”（D3）。
- T21 删除版本二次确认弹窗文案含“可能导致 File 解析不可用”（B2.1）。
- T22 选版本表单可选具体版本 + alias 过滤（C2/F1）。
- T23 dataset/file 版本选择入口与提交体含 `pipeline_version_id`。
- T28 `!` 点击仅提示。
- T29 Missing 状态呈现（禁用解析 + 提示重选）。

**集成/边界（按需 integration tier）**
- T24 删除被引用版本后，相关 file 解析失败且错误可定位。
- T25 画布改 DSL 生成新版本、已绑定方不受影响（B4 端到端）。
- T26 切版本后 component params 重置为该版本 default（C3）。
- T27 迁移在含 built-in + dataflow 混合库上的幂等重跑。
- T30 并发保存同 canvas 不产生孤儿版本。
- T31 alias 纯前端过滤不影响后端版本返回集。

---

- T32（v5.2 保存语义）`UpdateAgent` 带 `autosave:true` + DSL 变更 → 断言 `user_canvas_version` 行数**不变**、`user_canvas.dsl` 已更新为最新 DSL（不产生新版本）。
- T33（v5.2 保存语义）`UpdateAgent` 手动 Save（无 `autosave`）→ 断言**新增一条 `user_canvas_version` 记录且 `user_canvas.dsl` 已更新**（即便 DSL 与上一版本相同也 INSERT 新行，不复用就地刷新；验证 user_canvas 更新与版本插入两者俱发生）。
- T34（v5.2 统一路径）agent 画布自动保存同样不刷版本（与 dataflow 共用 `UpdateAgent`，断言行数不变）。
- T35（v5.2 alias 写入）`PATCH /versions/{id}` 带 `alias` → 断言该行 `alias` 更新、未新建版本、DSL 不变；跨 canvas 改 alias → 归属校验拒绝。
- T36（v5.2 alias 兜底）manual Save 请求携带 `alias` → 断言新建/刷新版本写入该 alias。
- T37（v5.2 前端 latest 去除）dataset/file 版本选择器选项列表**不含** `__latest__` 伪值，且含 alias 文案；提交 `pipeline_version_id` 为具体 ID（前端/组件断言）。
- T38（v5.2 对抗）连续 N 次自动保存（不刷版本）+ 仅 1 次手动 Save → 版本数恰为 1（非 N），且 live draft 反映最后一次自动保存内容；手动 Save 前后 `user_canvas_version` 记录数差恰为 1（确认 INSERT 而非 in-place 更新）。
- T39（v5.4 空版本校验）dataset 创建/更新 Save 时 `pipeline_version_id` 为空（且为 Pipeline 模式）→ 后端**拒绝**并返错误文案“版本信息缺失”（HTTP 4xx），`pipeline_version_id` 不写入、不自动回填为 latest（呼应 C5/G3）；对抗：built-in 模式允许空版本（空版本校验仅作用于 Pipeline 模式）。
- T40（v5.4 空版本校验·file 层）document/file 绑定 Save 时 `pipeline_version_id` 为空（Pipeline 模式）→ 后端同样拒绝并返“版本信息缺失”；dataset 与 file 两层校验行为一致（C5 双层要求）。

## 10. 漏洞审计（v4 状态）

| 编号 | 描述 | 状态 |
|---|---|---|
| V1 | 版本未线程化到运行时（loadDSL 永远读最新） | **已修复设计**：§6 改读 `user_canvas_version.dsl` |
| V2 | 拦截点不全（ad-hoc 路径漏校验） | 设计覆盖，待 T12 落实 |
| V3 | 别名误建版本 | 设计：alias 仅字段，不改 SaveOrReplaceLatestTx 行为 |
| V4 | `!` 与 Missing 不可区分 | 设计区分：`!`=outdated（可解析，提示）；Missing=版本不存在（不可解析） |
| V5 | C3 仅在 pipeline 变更触发 | 设计：C3 显式切版本即触发重置（§5 / T26） |
| V6 | Missing 恢复 | 设计：重新选版本即恢复，无需特殊逻辑 |
| V7 | 文件级 pipeline 绑定可编辑 | 已确认可编辑，document 级同套逻辑 |
| H12 | `!` “最新”口径 | **v4 修正**：dataflow 无发布 → 用 `GetLatest`（create_time），非 `GetLatestReleased` |

---

## 11. 验收清单（核心断言）

1. built-in parser 绑定：不受任何版本逻辑影响，`pipeline_version_id` 恒 NULL。
2. dataflow 绑定默认锁“当时最新具体版本”，画布后续改动不传播到已绑定方（B4）。
3. 删除被引用版本：引用方进入 Missing、file 不可解析；删除前有二次确认弹窗（B2/B2.1）。
4. 非最新绑定显示 `!`，点击仅提示，不自动升级（D3）。
5. 迁移幂等：存量 dataflow 绑定被锁定到 `GetLatest`；重跑安全；built-in/agent 跳过。
6. 悬空/跨 canvas 的 `pipeline_version_id` 被拒绝或判 Missing（归属校验）。
7. 别名仅存、仅前端过滤，后端版本返回集不受其影响。
8. 空版本（Pipeline 模式 `pipeline_version_id` 为空）：UI 红色 `Missing` 徽标；Save 时报错“版本信息缺失”且不写入、不自动回填；dataset 与 file 两层一致（C5/G3/T39/T40）。

---

## 12. 决策裁定（v5 已锁定，无未决项）

- **A1 类别边界（已裁定：强制校验）**：绑定/运行时要求 `user_canvas.canvas_category == "dataflow_canvas"`。依据：`canvas_category` 是 `user_canvas` 真实字段（值 `dataflow_canvas`/`agent_canvas`）；作为归属校验的强化。
- **A2 文件级范围（已裁定：覆盖 document）**：`document` 表同样加 `pipeline_version_id` 并复用全套逻辑。依据：C1 原文“dataset/file”。
- **A3 C3 重置范围（已裁定：整体重置）**：切版本时把 dataset/document 的 `parser_config`（component params）整体重置为该版本 default。依据：用户裁定 + C3 字面“参数重置为该版本 default”。（§5 已按此写）
- **A4 删除语义（已裁定：允许删除→Missing）**：弹窗二次确认后允许删除，引用方进入 Missing。依据：用户裁定 + B2.1 文案“告知风险”。（§7 已按此写）
- **A5 alias 约束（已裁定：不约束）**：自由文本、可空、不要求唯一。依据：A2 原文“仅存储、前端过滤”。
- **A6 Missing 呈现（已裁定：禁用解析+提示重选）**：前端对 Missing（版本被删）禁用解析按钮并提示重新选版本；与 `!`（仍可解析、仅提示）区分。
- **A7 ad-hoc/调试路径（已裁定：同正式解析带 version）**：`pipeline_executor.go` ad-hoc 路径已 reload `doc`（L888），同法透传 `doc.PipelineVersionID`。
- **A8 脏数据回填（已裁定：跳过+日志）**：backfill 遇 canvas 无版本时跳过 + 日志告警，绝不写悬空 version_id。

- **A9 保存语义修正范围（已裁定：作用于共享路径、全类别统一；用户确认 2026-10-22）**：手动/自动保存经 `autosave` 信号区分；版本创建仅限手动 Save，且手动 Save **同时**更新 `user_canvas` 并 **INSERT 新 `user_canvas_version` 记录**（agent 与 dataflow 一致）；agent 发布语义（Publish）不受影响。依据：agent/dataflow 共用 `UpdateAgent`，修正必须落在共享路径；agent 手动 Save 仍建版本，仅自动保存收敛，属良性改进且不触及 §1 范围。

> 设计已完全锁定，进入 §9 TDD 实现阶段。
## 13. 对抗性测试强制规则（v5.1 新增）

TDD 保证「代码满足测试」，不保证「测试满足需求」。本特性 review 暴露的 B1/B2 缺陷，根因不是 TDD 机制失效，而是**测试从需求派生的那一环失职**：fixture 全用了 `pinned == latest` 且两字段齐发，导致「钉住≠最新」这一 feature 核心场景从未被锻炼。

**强制规则（每条需求一句对抗性测试）：**
1. **每条需求条款必须对应至少一条对抗性测试**，其 fixture 必须让该条款**可证伪**。若「实现错了测试也绿」，说明 fixture 没打到痛处，该条款未覆盖。
2. **版本相关测试强制 `pinned != latest`**：凡涉及 `pipeline_version_id` 的测试，fixture 中钉住的版本必须明确不等于 `GetLatest`；并额外构造「钉住版本被后续新建版本挤成过时」的时序。
3. **耦合字段必须测「只发其一」的 PATCH**：`pipeline_id` / `pipeline_version_id` 是耦合对，必须为「只发 version 省略 pipeline」「只发 pipeline 省略 version」「两者都省略」各写一条，断言省略字段沿用已绑值。
4. **归属/权限边界必须负向测**：跨 canvas 的 `pipeline_version_id`、悬空 `pipeline_version_id`、agent canvas 误绑 dataflow 版本，必须各自返回 Missing/拒绝，而非静默回落。
5. **回落链必须逐元素断言**：§5 的版本回落链与 canvas 上下文回落链，必须分别有测试证明「跳过已钉版本直接取 latest」会被拒、「省略 pipeline_id 让上下文变 nil」会被拒。

§9 各 T 条目（T6/T7/T14/T26 等）须按上述规则补对抗性 fixture；新实现的 RED 测试若仅用 happy-path fixture，视为未达 TDD 门禁。

6. **保存语义必须测 `autosave` 正反两例**：必须分别证明「`autosave:true` 不刷版本、仅更新 live draft」与「手动 Save 刷版本」；仅测手动 Save 而漏掉自动保存不刷版本的用例，视为未覆盖（对应 T32/T33/T38）。
