# Implementation Plan: Go `knowledge_compiler/tree` (原 `raptor`) 概念整合与 gap 修复

## Overview

将 Go 版知识编译中基于 raptor 的树构建(`internal/ingestion/component/knowledge_compiler/raptor`)对齐 Python `rag/flow/compiler/compiler.py` 的 tree 模板语义,并把公开概念从 `raptor` 收敛为 `tree`(`raptor` 降为内部构建算法细节)。同时修复一次调查发现的关键功能 gap:默认摘要 prompt 与 Python 生产 `tree.yaml` 不一致。

## Background / 现状

- Python 入口:`rag/flow/compiler/compiler.py`;`_compile_tree_templates` 使用 `api/db/init_data/compilation_templates/tree.yaml` 组织 prompt,但**整个编译过程不调用 `structure.py` 抽取实体/关系**。
- Go 入口:`internal/ingestion/component/knowledge_compiler/component.go`;variant `VariantRaptor`(值 `"raptor"`),`compile_kwd="raptor"`,内部用 `watershed` 聚类构建树。
- Go **完全不读取任何模板 yaml**(已 grep 确认 `knowledge_compiler/` 下无 `.yaml` 读取)。
- 关键发现:Go 默认 prompt `raptor.go:548 defaultRaptorPrompt` = `"Please write a concise summary of the following texts:\n{cluster_content}"` —— 这其实是 `compiler.py:128` 的 **fallback**,不是生产 prompt。生产用的是 `tree.yaml` 的 `"Please summarize the following paragraphs. Be careful with the numbers, do not make things up. Paragraphs as following:\n{cluster_content}\nThe above is the content you need to summarize."`。Go 仅通过 `extra["prompt"]` 覆盖,组件解析模板后只 stamp 模板 ID、**不读取模板的 `raptor.prompt`** → 默认情况下 Go 摘要与 Python 生产不一致。

## Architecture Decisions

- **不引入 yaml 读取器**:tree 模板对 Go 有价值的只有 `raptor.prompt`,其余(`entity_types`/`global_rules`/`threshold`/`random_seed`/`max_cluster`/`ext`)是 `structure.py` 桩或 AHC/GMM 专用,Go 不需要。prompt 直接 hardcode。
- **概念整合**:公开概念统一为 `tree`(目录、variant 名、`compile_kwd`)。`raptor` 仅作为内部构建算法细节存在;**已决定**将 `raptor/` 包改名为 `tree/`(机械 churn 收益 >= 命名一致)。
- **刻意保留 / 需澄清的设计差异**(在 PR 中说明):
  - 聚类算法:**不是**根本差异。Python 的 `_get_clusters_ahc`(`rag/advanced_rag/knowlege_compile/raptor.py:277-326`)本质就是 1D 相邻对余弦相似度的百分位切分 + 连续贪心分配,与 Go `watershed` 同属一族算法(Python 把它命名为 "AHC" 是误导,它并非 scikit-learn 层次聚类)。**真正差异仅在于参数命名与默认值**:Python `cluster_percentile=30` 直接指定百分位;Go `tree_order=4` 间接得到 `percentile=100/4=25`。PR 文档应表述为"参数命名差异(`cluster_percentile` vs `tree_order`),默认百分位 30 vs 25",而非"两种不同算法"。(R2)
  - 存储/图谱模型:Python 走 ES 图谱(`compile_kwd="tree"`,nodes: title/description/embedding/children/layer) vs Go 把节点存成 chunk 行(`parent_kwd`/`children_kwd`/`raptor_layer_int`/`raptor_kwd`/`vector`/`content`/`title_tks`)。这是真实的结构差异。
  - datasetnav:Go 是独立 variant,不基于 raptor 树。
- **行为对齐优先于结构复刻**:以用户可见的摘要结果和产物结构对齐为准,不追求逐行移植 Python。
- **模板组参数:单 group(刻意范围收窄)**:Python `compiler.py:234` 调用 `resolve_template_ids_from_groups(self._param.compilation_template_group_ids, tenant_id)`——复数,支持**多个** group 同时解析模板。Go 版决定改用 `compilation_template_group_id`(**单数**,只取**单个** group)。**这是 Go 移植中刻意的 scope 收窄,不移植 Python 的多 group fan-out**;模板解析路径只按单个 group 查询,无需实现 `resolve_template_ids_from_groups` 的复数展开。(2026-07-31 决策)

## Task List

### Phase 0: 修复默认 prompt gap(P0 — 必做)

- [ ] Task 1: 将 `raptor.go:548 defaultRaptorPrompt` 从 fallback 改为 `tree.yaml` 生产 prompt。**必须保留 `{cluster_content}` 行前的 6 个前导空格**(来自 YAML `|-` literal block 的 base indent=6;Python `self._prompt.format(...)` 会把这 6 空格原样拼到 cluster 文本前):
  ```
  Please summarize the following paragraphs. Be careful with the numbers, do not make things up. Paragraphs as following:
        {cluster_content}
  The above is the content you need to summarize.
  ```
  (`{cluster_content}` 占位符格式与现有 `buildClusterContent` 注入保持一致;单测常量必须用含 6 空格的精确字符串,见 Task 3。R1)
- [ ] Task 2: 保留 `extra["prompt"]` 覆盖路径(用户自定义模板仍可覆盖默认),确认 `summarizeTexts` 仍读取 `extra["prompt"]` 优先于 `defaultRaptorPrompt`。
- [ ] Task 3: 在 `raptor/raptor_test.go` 加单测,断言 `defaultRaptorPrompt` 内容等于 `tree.yaml` 生产 prompt(防止后续漂移),并验证 `extra["prompt"]` 覆盖仍生效。

### Checkpoint: P0(已执行 2026-07-31)
- [x] Go 默认树摘要 prompt == Python `tree.yaml` 生产 prompt(`defaultRaptorPrompt` 改为含 6 空格的生产 prompt)。
- [x] 用户自定义 `prompt` 仍可覆盖(`resolveRaptorPrompt` 仍优先 `extra["prompt"]`)。
- [x] `build.sh --test ./internal/ingestion/component/knowledge_compiler/...` 全绿(含 `tree` 17s golden + `golden`)。

### Phase 1: 概念整合(P1 — variant / compile_kwd 改名)

- [ ] Task 4: 将 variant 值 `"raptor"` 改为 `"tree"`(`VariantRaptor` → `VariantTree`,或保留常量名仅改值;需确认下游/前端引用)。
  - **列名保留**:`raptor_kwd` / `raptor_layer_int` 是 ES schema 级列名(`infinity_mapping.json:43-44`),variant 改名后**列名不变**(改了需 ES mapping 迁移);仅把 `component.go:401-420` `applyVariantColumns` 的 `case` 标签从 `VariantRaptor` 改为 `VariantTree`。(R5)
  - **`normalizeVariant`**:`types.go:161-162` 的 `case "raptor": return VariantRaptor` 改为 `case "tree": return VariantTree`。这意味着已有的 `variant: "raptor"` DSL/canvas JSON 会直接报 `ErrUnknownVariant`——**破坏性变更**,需确认线上无 `variant: "raptor"` 配置;按 AGENTS.md 不保留 deprecated alias。(R7)
- [ ] Task 5: 将产物 `compile_kwd` 由 `"raptor"` 改为 `"tree"`,对齐 Python tree 模板 `compile_kwd="tree"`。
  - **`variantCompileKWD` map**:`component.go:182` `common.VariantRaptor: "raptor"` → 改为 `common.VariantTree: "tree"`(同步改)。(R9)
  - **`compilation_template_kind_kwd` 列**:`component.go:240` `doc.SetExtraValue("compilation_template_kind_kwd", string(p.Variant))` 会因 variant 值变而从 `"raptor"` 变为 `"tree"`(列定义在 `infinity_mapping.json:52`)。需确认无检索侧/文档结构端点按 `"raptor"` 过滤该列。(R4)
- [x] Task 6: ✅ **已确认**:检索侧 / 前端**不依赖** `compile_kwd="raptor"` 过滤树节点 → 直接改名,无需兼容层。
- [x] Task 7: ✅ **已决定**:将 `raptor/` 包改名为 `tree/`,并把 `buildTree`/`summarizeTexts` 等公开名收敛为 tree 语义;`raptor` 仅留作注释说明构建算法(机械 churn 收益 >= 命名一致,值得做)。

- [ ] 测试同步(R6):上述 variant 改名后,以下测试传入的 `"raptor"` 字符串需改为 `"tree"`:
  - `component_test.go:340,353` — `runVariant(t, "raptor", ...)`
  - `component_test.go:549` — `"variant": "raptor"`
  - `golden_test.go:251,253` — `runVariantChunks(t, "raptor", ...)`
  - `golden_test.go:235` — `loadBaseline(t, "raptor_baseline.json")`(基线文件名含 raptor,需同步重命名或改加载逻辑)

### Checkpoint: P1(已执行 2026-07-31)
- [x] variant 与 `compile_kwd` 统一为 `tree`:`types.go` `VariantRaptor→VariantTree`(值 `"raptor"`→`"tree"`)、`normalizeVariant` case、`variantCompileKWD` map(值 `"raptor"`→`"tree"`)、`applyVariantColumns` case、`component.go` 调度 case 全部改;`compilation_template_kind_kwd` 因 `string(p.Variant)` 自动变 `"tree"`。检索侧无依赖,R7 破坏性变更(无 `variant:"raptor"` 配置)已确认安全。
- [x] `raptor/` → `tree/` 包改名执行:`git mv` 目录 + 4 文件 `package raptor→tree` + `component.go` import 路径改 `tree` + `raptor.Run→tree.Run`(R5 `raptor_kwd`/`raptor_layer_int` 列名保留);测试同步(R6):`component_test.go`(函数名 + 2×`runVariant` + log + `"variant":"tree"`)、`golden_test.go`(函数名 `TestGolden_Tree_Structure` + `runVariantChunks` + `loadBaseline` + log)、`golden/golden_test.go`(6×`compile_kwd:"tree"` 夹具)、基线文件 `raptor_baseline.json→tree_baseline.json`。
- [x] 构建/测试绿(执行阶段验证):`bash build.sh --test ./internal/ingestion/component/knowledge_compiler/...` 全绿。

### Phase 2: 可选小 gap(P2 — 已决定**不补**)

> **决策(2026-07-31)**:P2 两项(`rewrite_duplicate_tree_names` / `max_cluster` Psi rebalance)**不补**。用户判定这两项都是"垃圾"(Python 侧也属非核心/噪声逻辑),对应的 Go 已知小 gap 接受为刻意差异,不在本计划执行。

- [x] Task 8: 放弃补 `rewrite_duplicate_tree_names`(`chunk_post_processor.py:539` 重复 title 重命名)。
- [x] Task 9: 放弃补 `max_cluster=64` fanout 上限 / Psi rebalance。

### Checkpoint: P2
- [x] P2 两项显式决策为"不补",记录完毕。

### Phase 3: 文档化刻意差异(P3)

- [x] Task 10: 代码注释已记录设计差异(2026-07-31 执行):
  - 聚类算法:**参数命名差异**(`cluster_percentile=30` vs `tree_order=4→percentile=25`),`watershed.go:1-21` 已注释与 Python `_get_clusters_ahc` 同族、非层次聚类(R2)。
  - 存储/图谱模型:ES 图谱 vs chunk 行(真实结构差异,代码注释 + PR 描述)。
  - datasetnav 独立 variant。
  - **前端澄清**(R8):前端 `tree_builder:'raptor'|'psi'`/`ProcessingType.raptor`/`parser_config.raptor` 属文档级 RAPTOR 生成路径,与本次 ingestion variant 改名是不同子系统——**PR 描述需显式说明**(代码侧不改这些)。
- [x] Task 11: `defaultRaptorPrompt` 注释引用源已从 `raptor.py` 改为 `tree.yaml`(R10);Go 不读模板 yaml、prompt 直接 hardcode 已在注释说明。

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| `compile_kwd` 改名破坏检索侧/前端过滤 | ~~High~~ → **已消除** | 已 grep 确认检索侧不依赖 `compile_kwd="raptor"`,无需兼容层。 |
| 默认 prompt 改动导致 golden 测试漂移 | Medium | Task 3 先锁 prompt 常量单测,再跑 golden。 |
| `raptor/` 包改名引发大范围 churn 与冲突 | Medium | 已决定执行;改名前 grep 全仓引用,分批机械替换 + 绿测试。 |
| 误将"参数命名差异"写成"算法差异"误导 PR 文档 | Low | R2 修正:聚类本质是同一族算法,PR 文档仅表述为 `cluster_percentile` vs `tree_order` 的参数/默认值差异。 |

## Out of Scope(不在本次范围)

- 不引入 yaml 读取器 / 模板引擎。
- 不对齐 ES 图谱存储模型、datasetnav 独立 variant(仅文档说明)。聚类算法(1D 相邻对百分位扫描)Python/Go 同族,仅参数命名(`cluster_percentile` vs `tree_order`)与默认值不同,不构成"不对齐"项(R2)。
- 不移植 Python 的多 group fan-out:Go 用单个 `compilation_template_group_id`(非复数 `compilation_template_group_ids`),不实现 `resolve_template_ids_from_groups` 的复数展开(刻意范围收窄)。
- 不移植 `structure.py` 实体/关系抽取(tree 路径本就不调用)。
- 不补 `rewrite_duplicate_tree_names`(P2,判定为非核心噪声逻辑)。
- 不补 `max_cluster` fanout 上限 / Psi rebalance(P2,判定为垃圾)。

## Open Questions(已决策)

- ~~检索侧是否依赖 `compile_kwd="raptor"`?~~ → **否**,直接改名,无兼容层。
- ~~`raptor/` 包改名是否值得?~~ → **值得**,执行 `raptor/` → `tree/` 改名。
- ~~P2 两项是否要补?~~ → **不补**,两项均为垃圾,接受为已知小 gap。

---

## Review (2026-07-31)

> 以下为基于全量代码核查的评审意见。每条标注严重级别。

### P0 — 阻塞级(必须修正后才能执行)

#### R1. tree.yaml 生产 prompt 的 `{cluster_content}` 行有 6 个前导空格,Task 1 漏了

`tree.yaml` 原文(YAML `|-` literal block,base indent = 6):

```
      Please summarize the following paragraphs. Be careful with the numbers, do not make things up. Paragraphs as following:
            {cluster_content}
      The above is the content you need to summarize.
```

`{cluster_content}` 行实际缩进 12 列,base 6 列,所以 **prompt 字符串中 `{cluster_content}` 前有 6 个空格**。Python `self._prompt.format(cluster_content=...)` 会把这 6 个空格原样拼到 cluster 文本前。Task 1 展示的 prompt 把这 6 个空格丢了 → 不是"与 Python 生产一致"。

**修正**:Task 1 的 hardcode 必须保留这 6 个空格,或显式决策"丢弃缩进"并记录为刻意差异。单测(Task 3)的断言常量也必须用含空格的精确字符串。

#### R2. Python "AHC" 就是 1D 相邻对百分位扫描 — 与 Go `watershed` 是同一算法族,计划描述错误

计划 Phase 2 / Risks 反复将 "AHC ↔ watershed" 列为**刻意差异**。但核查 `rag/advanced_rag/knowlege_compile/raptor.py:277-326`(`_get_clusters_ahc`):

```python
# L2-normalize → adjacent cosine sims (O(N), NOT O(N²))
adj_sims = np.sum(normalized[:-1] * normalized[1:], axis=1)
# percentile threshold
threshold = float(np.percentile(adj_sims, self._cluster_percentile))
# greedy contiguous assignment
for i in range(1, n):
    if adj_sims[i - 1] >= threshold:
        labels[i] = cluster_id
    else:
        cluster_id += 1; labels[i] = cluster_id
```

这就是 Go `watershed` 做的事(1D 相邻对相似度百分位切分 + 连续贪心分配)。Python 把它**命名为** "AHC" 是误导(它不是 scikit-learn 的 `AgglomerativeClustering`,不是层次聚类)。两者唯一真正的差异是**参数映射**:

| | Python | Go |
|---|---|---|
| 参数 | `cluster_percentile=30` | `tree_order=4` → `percentile=100/4=25` |
| 含义 | 直接指定百分位 | 间接:百分位 = 100/tree_order |
| 默认百分位 | 30 | 25 |

**修正**:将 "AHC ↔ watershed 聚类算法差异" 从刻意差异列表中**移除或改写**为"参数命名差异(`cluster_percentile` vs `tree_order`),默认百分位 30 vs 25"。当前描述会误导执行者以为两者是根本不同的算法,可能引发不必要的"对齐移植"诱惑(正是计划自己想避免的)。

### P1 — 重要(执行前应补充)

#### R3. Phase 1 Checkpoint 标记 `[x]` 但代码未执行

Checkpoint P1 标了 `[x] raptor/ → tree/ 包改名执行`,但实际代码:
- `raptor.go:14` 仍是 `package raptor`
- `types.go:24` 仍是 `VariantRaptor Variant = "raptor"`
- `component.go:98` 仍 dispatch `common.VariantRaptor`

**修正**:Checkpoint 改为 `[ ]`,或注明"决策已定,执行待 Phase 1 落地"。

#### R4. 改 variant 值会影响 `compilation_template_kind_kwd` 列,计划未提及

`component.go:240`:
```go
if err := doc.SetExtraValue("compilation_template_kind_kwd", string(p.Variant)); err != nil {
```

variant 从 `"raptor"` 改为 `"tree"` 后,`compilation_template_kind_kwd` 列值也从 `"raptor"` 变为 `"tree"`。该列定义在 `infinity_mapping.json:52`。计划 Task 5 只提到改 `compile_kwd`,漏了这个。需确认是否有检索侧/文档结构端点按 `compilation_template_kind_kwd="raptor"` 过滤。

#### R5. `raptor_kwd` / `raptor_layer_int` ES 列名应保留,计划未声明

`component.go:401-420` 的 `case common.VariantRaptor:` 写入 `raptor_kwd` 和 `raptor_layer_int`(定义在 `infinity_mapping.json:43-44`)。variant 改名为 `"tree"` 后:
- 列名**不应改**(它们是 ES schema 级列名,改了需要 ES mapping 迁移)。
- 但 `applyVariantColumns` 的 `case` 标签要从 `VariantRaptor` 改为 `VariantTree`。

**建议**:Task 4/7 补一句"`raptor_kwd`/`raptor_layer_int` 列名保留不变(ES schema 约束),仅 Go 常量名/值变"。

#### R6. 测试文件需同步更新,计划未列出

以下测试传入 `"raptor"` 作为 variant 字符串:
- `component_test.go:340` — `runVariant(t, "raptor", nil)`
- `component_test.go:353` — `runVariant(t, "raptor", ...)`
- `component_test.go:549` — `"variant": "raptor"`
- `golden_test.go:251,253` — `runVariantChunks(t, "raptor", ...)`
- `golden_test.go:235` — `loadBaseline(t, "raptor_baseline.json")`

variant 改为 `"tree"` 后这些全部要改。计划 Task 列表没有这一项。

#### R7. `normalizeVariant` 中 `"raptor"` case 的处理

`types.go:161-162`:
```go
case "raptor":
    return VariantRaptor
```

如果 variant 值改为 `"tree"`,这里要改为 `case "tree": return VariantTree`。计划说"无兼容层"(Task 6),意味着已有的 `variant: "raptor"` DSL 配置会直接报 `ErrUnknownVariant`。**需确认**:是否有线上 DSL/canvas JSON 用了 `variant: "raptor"`?如果有,要么加 alias(违背 AGENTS.md "no deprecated APIs"),要么接受破坏性变更。

### P2 — 建议(非阻塞)

#### R8. 前端 "raptor" 引用是另一层,计划应显式澄清避免混淆

前端有大量 "raptor" 引用(`tree_builder: 'raptor' | 'psi'`、`ProcessingType.raptor = 'RAPTOR'`、`TraceType.Raptor = 'raptor'`、API URL 段)。但这些属于**文档级 RAPTOR 生成路径**(`parser_config.raptor`),与 Go `knowledge_compiler` ingestion pipeline variant 是**不同子系统**。计划 Task 6 说"前端不依赖 `compile_kwd='raptor'`"是正确的(grep 确认前端无 `compile_kwd` 引用),但应在 PR 描述中显式说明"本次改名不影响前端 `tree_builder`/`ProcessingType.raptor`,两者是不同层",避免 reviewer 混淆。

#### R9. `variantCompileKWD` map 需同步改

`component.go:182`: `common.VariantRaptor: "raptor"` → 需改为 `common.VariantTree: "tree"`。Task 5 隐含了这一点,但没有显式列出这个 map。

#### R10. `defaultRaptorPrompt` 的注释引用了 `raptor.py` 而非 `tree.yaml`

`raptor.go:543-547` 的注释说 "mirrors Python `raptor.py`",但 P0 修正后应该改为 "mirrors `tree.yaml` production prompt"(因为对齐目标从 Python 代码 fallback 变成了 YAML 生产 prompt)。

### 总结

| 级别 | 条目 | 要点 | 状态(已并入正文) |
|------|------|------|------|
| **P0 阻塞** | R1 | tree.yaml prompt 的 `{cluster_content}` 前有 6 空格,Task 1 漏了 | ✅ Task 1 已保留 6 空格 |
| **P0 阻塞** | R2 | Python "AHC" = watershed 同族算法,不是刻意差异,描述错误 | ✅ Architecture Decisions / Task 10 / Risks / Out of Scope 已改写 |
| P1 重要 | R3 | Checkpoint `[x]` 但代码未执行 | ✅ Checkpoint P1 已改回 `[ ]` 并注明 |
| P1 重要 | R4 | `compilation_template_kind_kwd` 列值会变,未提及 | ✅ Task 5 已补 |
| P1 重要 | R5 | `raptor_kwd`/`raptor_layer_int` 列名应保留,需声明 | ✅ Task 4 已补 |
| P1 重要 | R6 | 测试文件需同步更新,未列出 | ✅ Phase 1 测试同步项已补 |
| P1 重要 | R7 | `normalizeVariant` 的 `"raptor"` case 处理 + 破坏性变更确认 | ✅ Task 4 已补 |
| P2 建议 | R8 | 前端 raptor 引用是另一层,应显式澄清 | ✅ Task 10 已补 |
| P2 建议 | R9 | `variantCompileKWD` map 需同步改 | ✅ Task 5 已补 |
| P2 建议 | R10 | 注释引用源应从 `raptor.py` 改为 `tree.yaml` | ✅ Task 11 已补 |

**结论**:P0 两项(R1 prompt 空格、R2 算法描述)已修正并并入正文;P1 五项(R3–R7)已补进 Task 列表 / Checkpoint。计划的整体方向(prompt 对齐 + 概念收敛)正确,上述评审意见全部落地,计划可直接进入执行(P0 → P1 → P3)。

---

## Code Review (2026-07-31 执行后)

> 计划执行后的全量代码核查。所有 `bash build.sh --test ./internal/ingestion/component/knowledge_compiler/...` 通过。

### 执行正确的部分

| 项 | 状态 | 验证 |
|----|------|------|
| P0 R1: prompt 6 空格 | ✅ | `defaultRaptorPrompt` 含 `\n      {cluster_content}`,`TestDefaultRaptorPromptMatchesTreeYAML` 锁定 |
| P0 R2: 算法描述修正 | ✅ | `watershed.go:1-8` 注释说明 "not agglomerative hierarchical clustering",`DefaultTreeOrder` 注释对比 `cluster_percentile=30` |
| P1 variant 改名 | ✅ | `VariantRaptor→VariantTree`(值 `"tree"`),`normalizeVariant` case 更新,`variantCompileKWD` map 更新 |
| P1 compile_kwd 改名 | ✅ | `variantCompileKWD[VariantTree]="tree"`,`compilation_template_kind_kwd` 随 `string(p.Variant)` 自动变 `"tree"` |
| P1 R5: ES 列名保留 | ✅ | `raptor_kwd`/`raptor_layer_int` 列名不变,仅 `case` 标签改 `VariantTree` |
| P1 R6: 测试同步 | ✅ | `component_test.go`/`golden_test.go` 的 `variant:"raptor"`→`"tree"`,基线文件 `raptor_baseline.json→tree_baseline.json` |
| P1 包改名 | ✅ | `raptor/→tree/`,4 文件 `package raptor→tree`,`component.go` import 路径改 |
| P1 R10: 注释引用源 | ✅ | `defaultRaptorPrompt` 注释引用 `tree.yaml` 而非 `raptor.py` |

### 执行中发现并修复的问题(reviewer 修复)

| 项 | 位置 | 问题 | 修复 |
|----|------|------|------|
| C1 | `tree/raptor.go:28` | 注释 "Run executes the raptor variant" | → "tree variant" |
| C2 | `tree/raptor.go:31,34` | 错误前缀 `"raptor:"` | → `"tree:"` |
| C3 | `tree/raptor.go:204,206,292,302,307,311` | 日志前缀 `"raptor:"` (6 处) | → `"tree:"` |
| C4 | `tree/watershed.go:49` | 错误前缀 `"raptor:"` | → `"tree:"` |
| C5 | `component_test.go:112` | 注释 "wiki/raptor/mindmap" | → "wiki/tree/mindmap" |
| C6 | `component_test.go:364` | `t.Fatalf("raptor(tree_order=2)...")` 遗漏 | → `"tree(tree_order=2)..."` |
| C7 | `component_test.go:537-538,568,583` | 注释/错误消息 "RAPTOR"/"raptor" | → "tree"/"tree" |
| C8 | `golden_test.go:233` | **函数名 `TestGolden_Raptor_Structure`** 与 Checkpoint 声称的 `TestGolden_Tree_Structure` 不符 | → `TestGolden_Tree_Structure` |
| C9 | `golden_test.go:228` | 注释 "raptor.go::watershed" | → "tree/raptor.go::watershed" |
| C10 | `golden/metrics.go` | **函数名 `AnalyzeRaptorProducts`** + 注释 "raptor product tree"/"raptor output" | → `AnalyzeTreeProducts` + "tree product tree"/"tree output" |
| C11 | `golden/golden_test.go:47,85` | 测试名 `TestAnalyzeRaptorProducts_*` + 调用 `AnalyzeRaptorProducts` | → `TestAnalyzeTreeProducts_*` + `AnalyzeTreeProducts` |
| C12 | `golden/corpus.go:20` | 注释 "raptor clustering" | → "tree clustering" |

### 保留未改的 "raptor" 引用(合理)

| 位置 | 引用 | 保留原因 |
|------|------|----------|
| `component.go:402-409` | `raptor_kwd` / `raptor_layer_int` | ES schema 列名(R5 决定保留) |
| `tree/raptor.go:6,9` 等 | 注释中 "raptor.go::watershed" / "RAPTOR tree" | 文件名仍为 `raptor.go`,算法名 RAPTOR 是文献术语 |
| `tree/raptor.go:95-102` 等 | `raptorMaxRetries`/`raptorDefaultMaxToken`/`raptorTruncationMarkerRE` 等 | unexported 常量,非公开 API;改名纯 churn 无功能收益 |
| `tree/raptor.go:552,556,560,564` | `raptorSystemHelper`/`raptorTitleInstruction`/`resolveRaptorPrompt` | 同上,unexported |
| `common/deps.go:107` | 注释 "RAPTOR uses it" | 描述算法用途 |

### 结论

执行整体正确,P0(prompt gap)和 P1(概念收敛)核心变更全部到位,所有测试通过。reviewer 额外修复了 12 项遗留 "raptor" 引用(C1–C12),主要是错误/日志消息前缀和测试函数名——这些是执行时机械遗漏,不影响功能但影响命名一致性。保留的 "raptor" 引用均为合理(ES 列名、unexported 常量、算法文献名)。
