# Parser 层 Go ↔ Python 一致性对齐 Handoff 文档

> 用途：多会话并行处理「Go 与 Python 在 Parser 层是否对同一文件产出相同结构」的核对与收敛工作。
> 本文档自包含，新会话只需读本文即可独立开工，无需回看原对话。
> 范围：**排除 PDF 解析器**（PDF 走独立原生后端 `internal/deepdoc/parser/` + `pdf_vision_dispatch.go`，不在此 scope）。

---

## 0. 目标与验收标准

**目标**：确认 DSL `File → Parser → Chunker → Extractor → Tokenizer` 中「Parser」这一独立阶段，在 Go 与 Python 两端对相同输入文件是否产出**相同结构**。

**验收标准（"相同结构"的定义）**：对同一份输入文件，
- **一级闸门（拼接对比）**：把两端 Parser 产出的 items 的 `text` 按序拼接、归一化后比较，**内容等价**即通过核心验收。切块所有权保留现状（§2.3 决策 1），故 token 切块的 item 边界**不要求一致**。
- **二级比对**：内容等价基础上，逐项比 `text` / `doc_type_kwd` / `ck_type` / `image` 取值；**块抽取粒度以 Python 为准**（Go 收敛到 Python 的块边界，§2.3 决策 3），但 token 切块的 item 数差异属保留现状。
- 字段形状：至少 `text` / `doc_type_kwd` 应一致；`ck_type` / `image` 等按 §2.3 决策对齐。

**重要**：这里的「Parser 阶段输出」指
- Go：`ParseResultProducer.ParseWithResult(...)` 返回的 `ParseResult`（字段 `JSON`/`Markdown`/`Text`/`HTML` + `File`）；
- Python（flow）：`rag/flow/parser/parser.py` 里 `Parser._invoke` → `set_output` 产出的 `sections` 列表（每项形如 `{text, doc_type_kwd, image?, ck_type?}`）。

---

## 1. 关键定位纠正（务必先读）

**正确的 Python 对应物是 `rag/flow/parser/parser.py`，不是 `deepdoc/parser/*` + `rag/app/naive.py`。**

- 原因：你给的 DSL 是**分阶段**的，Go 端 `internal/parser/parser/*` 的移植注释明确引用 `rag/flow/parser/parser.py:_invoke` 与 `set_output`（`internal/ingestion/component/parser.go:392` 注释）。Go 的 `ParseResult` 契约移植自 `port-rag-flow-pipeline-to-go.md §4.2/§6.5`。
- `deepdoc/parser/*` + `rag/app/naive.py` 是 **naive 摄取路径**，它把 parser+chunker+tokenizer **融合**在一个 `chunk()` 里，**不是**独立 Parser 阶段的对应物。但注意：**flow 解析器内部又委托了 naive 解析器**（`_markdown`→`naive.Markdown`，`_html`→`HtmlParser`，`_docx`→`naive.Docx`），所以 naive 解析器是底层实现，阶段边界仍以 `rag/flow/parser/parser.py` 为对齐面。

**架构根因（必须在任何收敛方案里承认）**：
- Go 把 Parser 当作**干净的「未切块」阶段**：`ParseResult` → 重塑成 `schema.Page`（`internal/ingestion/component/parser_dispatch.go` 的 `jsonItemsToPages`/`buildParserOutputs`）→ 交给下游 Chunker 才做 token 切块。
- Python flow 解析器复用了 naive 解析器，而 naive 解析器（html/txt/epub 尤其）**在 parser 内部就做了 token 切块**。因此即便输出 key 名一致，parser 阶段的粒度已经不同。

---

## 2. 类型 ↔ 实现映射与一致性总览

| 类型 | Go 实现 | Python(flow) 实现 | 默认输出格式 | 一致性 | 风险等级 |
|---|---|---|---|---|---|
| markdown | `internal/parser/parser/markdown_parser.go` | `rag/flow/parser/parser.py:_markdown` → `naive.Markdown` | json | ✅ 基本一致（ck_type 保留 / delimiter 接受差异） | 中 |
| html | `internal/parser/parser/html_parser.go` | `rag/flow/parser/parser.py:_html` → `HtmlParser` | json | ⚠️ 表格已修复（保留切块差异，会话 B/D2） | 中 |
| text&code | `internal/parser/parser/text_parser.go` | `rag/flow/parser/parser.py:_code` → `TxtParser` | json | ❌ 不一致 | 高 |
| docx | `internal/parser/parser/docx_parser.go`（office_oxide） | `parser.py:_docx` → `naive.Docx`（python-docx） | json | ⚠️ 结构类似，引擎不同 | 中 |
| xlsx/xls/csv | `xlsx_parser.go` / `xls_parser.go` / `csv_parser.go` | `parser.py:_spreadsheet` → `ExcelParser` | html | ⚠️ 默认 html，待细核 | 中 |
| pptx/ppt | `parser/pptx_parser.go`（office_oxide） | `parser.py:_slides` → `RAGFlowPptParser` | json | ⚠️ 引擎不同 | 中 |
| epub | `parser/epub_parser.go` | `parser.py:_epub` → `EpubParser` | json | ❌ 不一致（接受差异，§3.5） | 中 |
| email | `parser/email_parser.go` | `parser.py:_email` | json/text | ✅ 基本一致 | 低 |
| doc | `parser/doc_parser.go`（office_oxide） | `parser.py:_doc`（tika） | json | ❌ 格式不同 | 中 |
| json | `parser/json_parser.go` | **flow 无对应分支** | json | — Go 独有 | — |

---

## 2.1 Go 端 Parser 的 item 模型（对拍基础）

对拍时反复提到的「item」在 Go 端有精确定义，先统一术语，避免与下游 `schema.Page` 混淆。

### item 是什么
- **item = `ParseResult.JSON` 里的一个 `map[string]any` 元素**（`internal/parser/parser/parse_result.go:48`），代表从文件解析出的**一个逻辑单元**（块 / 段落 / 表格 / 图片 / 行 / 幻灯片…）。
- 关键字段：
  - `text`：该单元文本（表格单元里是 HTML 字符串）
  - `doc_type_kwd`：类型，取值 `text` / `image` / `table`
  - 可选 `ck_type`：更细分类（heading/paragraph/list/code/quote/table/image）
  - 可选 `image`：base64 图片；`page_number`：页码
- **仅当 `OutputFormat=="json"` 时才存在 item 列表**。markdown/text/html 输出格式的解析器产出的是**单个字符串**（`ParseResult.Markdown`/`Text`/`HTML`），reshape 后变成 **1 个 `schema.Page`**，**没有 JSON item**。
- 下游 dispatch（`internal/ingestion/component/parser_dispatch.go`）：`OutputFormat=="json"` → 每个 item 一个 page（`jsonItemsToPages`）；`markdown/text/html` → 整段作 1 个 page；未知 → 按 `\f` 切页。所以「item 数」对比**只对 json 输出格式有意义**。

### item 数量（因格式与输入而异，无固定值）
| 格式 | OutputFormat | item 数量 |
|---|---|---|
| markdown | json | = 顶层块数；**含表时不再塌缩**（表内联为 HTML 块后逐块成项，会话 A 修复 `markdown_parser.go`）；空输入补 1 个占位 |
| html | json | = 有文本的块级元素数（`walkHTMLBlocks` 每块 1 项） |
| text&code | json | = 非空段数（Go 按默认 delimiter `"\n!?;。；！？"` 切 + keep_delimiters，与 Python `_code` 一致；超 8192B 就近再切）— **与 Python 默认 delimiter 现已一致**（会话 C / PR #18022），item 数仍因不做事后 OVER_CAP token 合并而与 Python 合并后不同，详见 §3.3 |
| docx | json | = section/element 数（段落/标题/图/表各自成项） |
| xlsx/xls/csv | **默认 html** | **0 个 JSON item**（单个 HTML 字符串）；走 json 路径才按行成项 |
| pptx/ppt | json | = 幻灯片数（按 `\f` 分页） |
| epub | json | = spine 条目数 |
| email | json/text | json 时 **1 个**（单个字典项，含 from/to/subject/body…） |
| json | json | = 逻辑记录数（数组每元素 / 对象 / JSONL 每行） |

### item 类型（按 `doc_type_kwd`，共 3 类；全 Go 解析器 JSON 输出一致）
1. **`text`** —— 绝大多数：段落、标题、列表、代码块、单元格文本、行文本、幻灯片文本、email 正文等。docx 标题项额外带 `ck_type:"heading"`。
2. **`image`** —— 图片，必带 `image`(base64)。来源：markdown 的 `![alt](src)` 解析（`markdown_parser.go:287-292`，含 HTTP 抓取+SSRF 防护）；docx 内嵌图。
3. **`table`** —— 表格，`text` 字段是 HTML 表格字符串。来源：docx 表格（`docx_ir.go`）、markdown 内联表（未整篇塌缩时）。
- 更细一层的 `ck_type` 词汇（仅 markdown 等部分解析器输出）：`heading` / `paragraph` / `list` / `code` / `quote` / `table` / `image`。**Python flow 的 `set_output` 不带 `ck_type`**，是字段契约差异点（见 §3.1）。

### 对照 Python（可比对维度）
- Python flow `set_output` 的 json 路径产出同形状 `[{text, doc_type_kwd, image?, ...}]`，因此「**item 数 + 每 item 的 `text`/`doc_type_kwd`**」是两端可直接对拍的维度（即 §5 逐项/计数比对层）。
- 例外：xlsx/csv 在 Go 默认 html（0 个 json item），Python `_spreadsheet` 默认也是 html（`ExcelParser().html`）→ 两端都是单字符串，item 维度均为 0，比对时按 HTML 字符串走。

---

## 2.2 Schema 模型（契约信封 / 数据结构）

§2.1 描述的是 item（JSON 列表里的元素）。本节描述「装着 item 的信封」与组件间契约——即 Parser 输入输出、配置、以及 Parser→Chunker 的桥接结构。两端刻意镜像（Go `schema/parser.go` 注释多处写明 "mirrors rag/flow/parser/schema.py"）。

### Go 端
- **`ParseResult`**（`internal/parser/parser/parse_result.go:48`）：解析器统一返回契约。
  - 字段：`OutputFormat`(json/markdown/text/html)、`File`(map)、`JSON`(`[]map[string]any`)、`Markdown`/`Text`/`HTML`(string)、`Err`。
  - 仅 `OutputFormat=="json"` 时 `JSON` 填充（即 §2.1 的 item 列表）。
- **`schema.Page`**（`internal/ingestion/component/schema/parser.go:58`）：`Page = map[string]any`，Parser 把 `JSON` 重塑后交给下游 Chunker 的载体（json→每 item 一 page）。注释明确：它故意保持 Python `dict` 形态，字段形如 `{text, doc_type_kwd, ck_type?, image?, page_number?, positions?}`（`parser.go:52-57`）。
- **`ParserFromUpstream`**（`parser.go:29-41`）：Parser 组件输入（name/file/abstract/author/created_time/elapsed_time）。注释写明 mirrors Python `ParserFromUpstream`。
- **`ParserOutputs`**（`parser.go:110-136`）：Parser 组件输出契约。字段 `output_format` + 四选一（`json`/`markdown`/`text`/`html`）+ `file` + `_ERROR`。**直接镜像 Python `set_output` 的写入面**。
- **`ParserSetup` / `ParserParam`**（`parser.go:64-94`）：每文件类型的配置块 + 静态配置（含 `AllowedOutputFormat` 白名单）。`Defaults()` 注释写明**逐字复制自 Python `ParserParam.__init__`**。
- **`ChunkDoc`**（下游，`internal/ingestion/component/schema/chunk_types.go`）：Chunker 产出结构（text/content_with_weight/doc_type_kwd/ck_type/image/page_number/positions…）。属 Parser 之后的阶段，仅作上下文，不在本 scope。

### Python 端（flow）
- **`ParserFromUpstream`**（`rag/flow/parser/schema.py:18-26`，Pydantic）：输入契约。字段与 Go 完全一致（created_time/elapsed_time/name/file/abstract/author，`populate_by_name`、`extra="forbid"`）。
- **Parser 输出面**（`rag/flow/parser/parser.py` 内 `set_output`）：无单一命名 model，但写入键固定为 `output_format` + 四选一（`json`/`text`/`markdown`/`html`）+ `file` + `_ERROR` → **与 Go `ParserOutputs` 一一对应**。json 路径写 `set_output("json", [...])`（即 §2.1 的 item 列表）。
- **`TokenChunkerFromUpstream`**（`rag/flow/chunker/schema.py:20-35`，Pydantic）：**Parser→Chunker 的桥接契约**，Chunker 消费它。
  - 字段：`output_format`(json/markdown/text/html/chunks)、`json_result`(别名 `json`)、`markdown_result`/`text_result`/`html_result`、`chunks`、`name`/`file`/`created_time`/`elapsed_time`。
  - 即 Chunker 读 `from_upstream.output_format` 与 `from_upstream.json_result`（见 `token_chunker.py:313-345`）。这正是 Go `ParseResult.JSON`/`schema.Page` 在 Python 侧的对应物。
- Chunker 输出：`TokenChunker.set_output("chunks", [{"text", "doc_type_kwd", ...}])`（`token_chunker.py:440` `_finalize_json_chunks`）→ 对应 Go 的 `ChunkDoc`。

### 跨端结构映射表
| 概念 | Go | Python(flow) |
|---|---|---|
| 解析器返回 | `ParseResult`（`parse_result.go:48`） | （隐式）`set_output` 写入面 |
| 组件输入 | `ParserFromUpstream`（`schema/parser.go:29`） | `ParserFromUpstream`（`parser/schema.py:18`） |
| 组件输出 | `ParserOutputs`（`schema/parser.go:110`） | `set_output`：`output_format`+`json/text/markdown/html`+`file`+`_ERROR` |
| item 载体 / 页 | `schema.Page` = `map[string]any`（`schema/parser.go:58`） | `TokenChunkerFromUpstream.json_result`（`chunker/schema.py:30`） |
| 配置块 | `ParserSetup` / `ParserParam`（`schema/parser.go:64-94`） | `ParserParam.setups` / `allowed_output_format`（`parser.py:71-...`） |
| Chunker 输入 | `schema.Page` 列表 | `TokenChunkerFromUpstream`（`chunker/schema.py:20`） |
| Chunker 输出 | `ChunkDoc`（`chunk_types.go`） | `chunks: [{"text","doc_type_kwd",...}]`（`token_chunker.py:440`） |

**对齐要点**：结构信封两端已高度镜像（`ParserFromUpstream`/`ParserOutputs`/`ParserParam` 是逐字对应），所以「对拍」的重点不在信封，而在信封内的 **item 内容/粒度/字段**（§2.1 + §3）。唯一需留意的是 `TokenChunkerFromUpstream` 还有 `chunks` 字段——当上游已是 chunks 时 Chunker 直接透传，这是 Python 在 Parser 之后、Chunker 之前可能已被预切块的另一证据（关联 §3.2 的切块归属讨论）。

---

## 2.3 已确认的对齐决策（全局，2026-08-07）

以下为已拍板、跨类型生效的决策，各并行会话直接遵循，无需再议：

1. **切块所有权：保留现状**。不做 pipeline 重构——Go Parser 仍不切块、Python flow Parser 仍内嵌 token 切块（html/epub 等）。不在「把切块挪到 Chunker 统一」上做改动。
2. **对比方法：采用拼接对比（stitch-compare）**。以「拼接后内容等价」作为一级闸门（见 §5），item 数 / 字段比对作为二级。验收重点是「两端 Parser 提取出相同内容」，而非 item 边界一致。
3. **块抽取粒度基准：以 Python 为准**。逐块边界（段落 / 块 / 表 / 列表续行等）的对齐目标是让 **Go 收敛到 Python 的块边界**；Python 现状视为事实基准。
4. **`rag_tokenizer.tokenize` 身份暂未确认**：`deepdoc/parser/html_parser.py:236` 处 `rag_tokenizer.tokenize` 真实分词器实现尚未确认（原假设 `RagTokenizer` 已证伪）。HTML 会话 B 处理块抽取 token 口径前需先确认该身份，再判断 Parser/Chunker 是否同一 tokenizer。当前不作为阻塞项。
5. **doc / docx / pptx / xlsx 引擎差异：保留（接受差异）**。Go 用 `office_oxide`、Python 用 python-docx / pptx / excelize / tika，文本与分段发散属可接受分歧，记录原因即可。
6. **json：Python flow 不补分支**。Go `json_parser.go` 为 Go 独有，Python 侧声明 flow 不支持 `.json`，接受此不对称。

---

## 3. 已核实的具体分歧（带 file:line，供新会话直接验证）

### 3.1 Markdown
- **表格塌缩（已修复，会话 A / 方案 Y）**：原 Go `markdown_parser.go` 的 `renderMarkdownTablesInline` 只要文档出现**任意一张 GFM 表格**，就把**整篇**重写为单个 item `{"text": rendered, "doc_type_kwd":"text"}`。Python `separate_tables=False`（`parser.py:1080`）只把表格内联成 HTML 块，但**仍按块切成多个 section**，并对每张表**额外产出一个独立的 `doc_type_kwd:"table"` item**。→ 同输入，Go 可能 1 个大 item，Python 是 N 个（含表 item）。**修复（方案 Y）**：`ParseWithResult` 不再整篇短路返回，改为先 `renderMarkdownTablesInline` 把表内联为 `<table>` HTML 块（并**在表格前后补空行**，满足 MD058，使 gomarkdown 将其保留为单个 `*ast.HTMLBlock` 而非重新解析成散落单元格文本），再调用 gomarkdown 逐顶层块成项。对表格 HTML 块，`walkMarkdownBlocksWithImages` 同时产出**两项**：① 内联副本 `{"text": "<table>...</table>", "doc_type_kwd":"text"}`（保留在原始段落顺序，与 Python 内联表位置一致）；② 独立的 `{"text": 表格原始文本, "doc_type_kwd":"table", "ck_type":"table"}` item（与 Python `_markdown` 为每张表 append 的真实 `doc_type_kwd:"table"` item 一致，下游 Chunker 据此走表格分支）。非表格 HTML 块（`<div>`/`<style>` 等）**仅作为纯文本**，**不**加 `ck_type`/`doc_type_kwd:"table"`。回归用例 `TestMarkdownParser_TableNotCollapsed`（表不塌缩 + 表 item 含真实 `<table>` 文本 + 非表格 HTML 块不被误标为 table）与 `TestMarkdownParser_NonTableHTMLBlockNotTable` 守护。
- **ck_type 字段（保留差异，已确认）**：Go 输出带 `ck_type`（heading/paragraph/list/code/table，经 `walkMarkdownBlocksWithImages` 写入）；Python `_markdown` 的 `set_output` 输出**不含** `ck_type`（`parser.py:1089-1101`）。→ 字段形状不同。用户决策：**ck_type 保留**，作为 Go 额外字段。**注意 `ck_type:"table"` 仅加在真实表格 HTML 块上**（经 `isTableHTML` 判定 `<table` 前缀），普通 HTML 块（`<div>`/`<style>` 等）不带 `ck_type`——此前曾 blanket 套在所有 HTMLBlock 上，已修正。拼接对比时忽略 `ck_type`（Python 无此字段）。**下游消费真相**：Chunker 实际读的是 `doc_type_kwd`（`chunker/common.go:itemDocType` 读 `DocType`；`chunker/token.go` 中 `ck_type!="text"` 则整 item 不切），`ck_type` 仅由 `cropImageChunks`/`needsCrop`（`pdfcrop_cgo.go`）用于定位图片/表格裁剪（markdown 为 no-op）。因此表格正确处理依赖 `doc_type_kwd:"table"` 而非 `ck_type`。
- **delimiter 切分（接受差异，已确认）**：Go markdown 解析器**不做** delimiter 切分；Python 把 `conf.get("delimiter")` 传入 naive 解析器做二次切分（`parser.py:1081`，底层 `deepdoc/parser/markdown_parser.py:167-174, 305-345`）。但注意：`setups["markdown"]`（`parser.py:163-168`）**无 `delimiter` 键**，故 `conf.get("delimiter")` 永远为 `None`——**Parser 端没有 delimiter 配置入口**（与 text&code 同理，见 §3.3「delimiter 配置归属」）。真正用户可配的 delimiter 在 **Chunker**（`TokenChunker.delimiters`）。所以两端在 markdown 上都不会因"配置 delimiter"而分歧；若有差异，边界分歧来自 Go 完全不做 delimiter 切分、Python 即便 delimiter=None 也有其默认段落处理。**用户决策：delimiter 差异接受**，在拼接对比阶段用 `WithDelimiterStrip(delimiter)` 把分隔符替换为空格（保留 token 间距），使 Go 的内联分隔符与 Python 的 item 切分在拼接后内容等价。
- **图片关联（已修复潜在 bug，会话 A）**：原 `resolveMarkdownImage(txt, rawText)` 扫描**整篇文档**，一旦文档含任意图片，会把**每一个** block 都标成 `doc_type_kwd:"image"`，导致 multi-block 文档几乎零 `text` item → 拼接对比看到空 Go 文本。修复为 `findBlockImage(child)` 仅从**当前 block 的 AST** 解析第一个 `*ast.Image` 的 `Destination`（`markdown_parser.go:307, 324-343`），图片 payload 只挂到真正包含 `![alt](src)` 的那一个 block；`resolveImageURL` 仍支持 `data:image/` base64 与 `http(s)://` 抓取（SSRF 防护）。
- 图片：Go 把 `![alt](src)` 内联成 base64 + `doc_type_kwd:"image"`（`markdown_parser.go:307-314`，含 HTTP 抓取与 SSRF 防护 `:399-479`）；Python 走 `return_section_images=True` + vision 增强（`parser.py:1081-1088` 附近），机制不同但目标类似。

### 3.2 HTML（粒度本质不同）
- **切块时机**：Go `html_parser.go` 的 `walkHTMLBlocks`（`:109-136`）每个块级元素产 1 个 item，**不切块**。Python `_html` 调 `HtmlParser()(name, blob, int(conf.get("chunk_token_num", 512)))`（`parser.py:1154`，阈值来自 `self._param.setups["html"]["chunk_token_num"]`，**配置驱动、默认 512**；html 族默认 setup `parser.py:188-193` 未设该 key，默认部署下即 512）。`HtmlParser.chunk_block` 用 `rag_tokenizer.tokenize` 的真实 token 数累加、超阈值即切（`deepdoc/parser/html_parser.py:232-237, 287-313`）。→ Go 是原始块，Python 已切成 token 块。注意 `text&code` 的同类阈值默认是 128（`parser.py:1135`），两族默认值不同。
- **表格表示（已修复，会话 B / D2 方案 a）**：原 Go 把 `<table>` 折叠成单个文本块（如 `"NameAge\nAlice30"`）并标 `ck_type:"table"`，**但 `text` 不带任何 `<table>` 标记**——这会让下游 embedding/retrieval/LLM 渲染丢失行列结构（"塌方"）。**修复（方案 a，镜像 markdown 的 `markdown_parser.go:305-328` + `:366-367`）**：两处都渲染出完整 `<table>…</table>` 标记串并各产**两项**（内联 `doc_type_kwd:"text"` 副本 + 结构化 `doc_type_kwd:"table"/ck_type:"table"` 项）：① **顶层表**：`walkHTMLBlocks` 的 `case "table":`（`html_parser.go:136-157`）直接渲染；② **嵌套表（`<table>` 被 div/section/article/… 包裹）**：`walkHTMLBlocks` 命中 default 分支走 `htmlLeafText`（`html_parser.go:159`），`walkHTMLLeaf` 现已对 `<table>` 感知（`html_parser.go:323-344`）——渲染标记串写入父块的单条 `text` 项（保住结构又不改变粒度），并收集结构化表项。两路的结构化项都在 `walkHTMLBlocks` 循环结束后统一 `append`（与 markdown 表 item 排在 text 项之后一致），由下游 Chunker 走表格分支消费。`renderTableHTML` 辅助（`html_parser.go:167`）用 `html.Render` 序列化；渲染失败返回 `""`（html.Render 对解析后的 `<table>` 不会失败），调用方跳过该表而非进入渲染死循环。回归用例 `TestHTMLParser_TableProducesStructuredItems`（顶层表）+ `TestHTMLParser_NestedTableProducesStructuredItems`（嵌套表，原塌方案例）守护——两者均校验：内联副本含 `<table>`+单元格文本、结构化项 `doc_type_kwd:"table"/ck_type:"table"`、结构化项排在尾部 `<p>` 之后。
- **表格"塌方"根因 vs 接线神话（已核实并纠正）**：早期曾误判 HTML 解析器需改接线走向 `PayloadFormatHTML → emitOne`（把整段拼成 1 个 text chunk 并覆盖 `ck_type:"text"`）。**该判断错误**——HTML 解析器早已产出 `OutputFormat:"json"`（`html_parser.go:94-101`），`parser_dispatch.go:404-408` 据此映射到 `output_format:"json"` + `json` 键，Chunker 走 `default → tableItems/invokeJSONPayload` 分支**已保留 `ck_type`**。那条 flatten 路径只对 `html` 标量字符串生效，而 HTML 解析器从不产出标量 `html` 串。→ **本次修复纯 parser 内部**，无需任何 Chunker/dispatch 接线改动。
- **列表（嵌套，已验证无塌方）**：`<ul>/<ol>` 及其嵌套子表在 `walkHTMLLeaf` 中按 `<li>` 硬换行逐条输出，所有项文本按文档顺序保留（验证用例 `TestHTMLParser_NestedListItemsPreserved`）。与 Python `read_text_recursively`（`html_parser.py:108-160`）一致——两边都保内容、**都不表示嵌套层级深度**。接受的粒度差异：Python 每个 `<li>` 一条记录，Go 整个 `<ul>` 产 1 个 `text` 项（`ck_type:"list"`，顶层表）；列表嵌在 div/section 等非列表容器时折叠进该容器的单条 `text` 项、丢失 `ck_type:"list"`（变 `"text"`）——属保真度细节非塌方，下游切块读 `doc_type_kwd` 不读 `ck_type`（§3.1）。无需修复。
- 两者都做 CSS 空白折叠与 `<br>` 硬换行（`html_parser.go:222-281` ↔ `html_parser.py:169-178`），这部分是对齐的。

**Python flow 管线后续仍走独立 Chunker（`TokenChunker`，`rag/flow/chunker/token_chunker.py`），但默认不是二次切分**：`_html` 产出的 `sections` 已是 512-token 块（`parser.py:1157-1159` 写入 `json_result`），Chunker 的 json 分支（`token_chunker.py:344-442`）默认 `delimiter_pattern` 为空（只认反引号自定义分隔符，`:70-76`），`_build_json_chunks` 不把 section 切更小，仅 `_merge_text_chunks_by_token_size`（`:432-434`，阈值默认 512）做相邻合并、绝不分裂 → 默认近似透传。两处隐患（**(a) 待核实 / FIXME**）：(a) **Parser 与 Chunker 的 token 计数口径是否一致 —— 待核实**：`TokenChunker` 用 `num_tokens_from_string`（`common/token_utils.py:126-132`），底层是 `tiktoken.get_encoding("cl100k_base")`（`common/token_utils.py:45`，OpenAI GPT BPE）—— 此点已确认。`HtmlParser._token_count` 用 `rag_tokenizer.tokenize`（`html_parser.py:236`），但 `rag_tokenizer.tokenize` 实际解析到哪个分词器**尚未确认**：原先假设为 `infinity.rag_tokenizer.RagTokenizer` 已确认不正确（见 `html_parser.py` 内 FIXME 注释）。因此「两套分词器不一致」这一结论目前**不成立/待证**，需先确认 `rag_tokenizer.tokenize` 的真实实现，再判断 Parser/Chunker 是否同一 tokenizer、以及「512 token」前后含义是否一致。(b) 若 Chunker 配了自定义分隔符，`_build_json_chunks` 会把已切好的 section 内部再切（`:120-121`）= 真正的二次切分。**已确认决策（见 §2.3）**：切块所有权**保留现状**——不移除 Python 在 Parser 阶段的 token 切块，也不让 Go 预切块；验收以**拼接对比**的内容等价为准（一级闸门），item 数 / token 切块差异不要求一致。`rag_tokenizer.tokenize` 身份（FIXME）保留不动。块抽取粒度以 **Python 为准**，Go 的块边界在二级比对中收敛到 Python。

### 3.3 Text / Code
- Go `text_parser.go:146-175` 仅按空行 `\n\n` 分段，超 8192 字节就近按 `\n` 再切，**无 delimiter、无 token 统计**。
- Python `_code` 调 `TxtParser(..., delimiter, keep_delimiters=True)` 再 `merge_paragraphs(OVER_CAP)` token 合并（`deepdoc/parser/txt_parser.py:46-63`）。→ 段落切分完全不同。
- **delimiter 配置归属（已核实，重要）**：`delimiter` 在 **Parser 组件上没有配置入口**。`setups["text&code"]`（`parser.py:169-187`）只有 `suffix` + `output_format`，无 `delimiter` 键；Go 端 `ParserSetup` 同样无此字段。所以 `_code` 的 `conf.get("delimiter", "\n!?;。；！？")`（`parser.py:1136`）**永远命中硬编码默认** `"\n!?;。；！？"`，用户无法在 Parser 端改它。真正用户可配的 delimiter 在 **Chunker**（`TokenChunker.delimiters` 列表 + `delimiter_mode`，`token_chunker.py:35-65`）——这是**另一套 key、另一形状（str vs list）、另一默认值**，且 Parser **不读** Chunker 的 `delimiters`。因此"Parser 用的 delimiter"实为写死常量，与 Chunker 的 `delimiters` 互相独立（用户改 Chunker 不影响 Parser 切分）。
- **对齐做法（据此修正）**：Go 不需要、也不应新增"Parser 端 delimiter 配置键"（Python 也没有）。正确对齐是——Go `TextParser` 把硬编码的 `\n\n` 分段，**改为与 Python 相同的默认 delimiter** `"\n!?;。；！？"`（含 `keep_delimiters=True` 把分隔符并回前段），即为纯内部常量对齐，**无需 `ConfigureFromSetup` 为 delimiter 服务、无需任何 schema 改动**。token 合并（OVER_CAP 128）仍保留现状（决策 1）。
- 注意：Go 文件头注释（`text_parser.go:17-31`，尤其 `:143-145` 称 "split rule mirrors the python TxtParser"）**不准确**——Go 用 `\n\n` 而非 Python 默认 delimiter，应在对齐时一并修正。

### 3.4 docx / pptx / xlsx / doc（引擎差异）
- 这四类 Go 用 `office_oxide` 原生库；Python 用 `python-docx` / `python-pptx` / `excelize`（等价物）/ `tika`。**引擎不同 → section 数量与文本本身就会不同**，即便 JSON 包裹形状类似。按 §2.3 决策 5，此为**接受差异**，记录原因即可，不作为阻塞项。
- docx JSON 包裹形状两侧都是 `{text, image?, doc_type_kwd}` + 表格单独 `{text, doc_type_kwd:"table"}`，且 Go 额外带 `ck_type:"heading"`（`docx_ir.go:buildDOCXJSONSections` 附近）；结构基本对齐，但文本来源不同。
- **doc 特例**：Go `doc_parser.go` 输出 `OutputFormat:"text"`（`office_oxide.PlainText`）单字符串；Python `_doc` 配置 `output_format:"json"`，产出**行列表** `{"text": line, "doc_type_kwd":"text"}`（`parser.py` `_doc` 分支）。→ 输出格式都不同。

### 3.5 epub / email

**epub —— 不一致（中/高风险，与 HTML §3.2 同根因，非"基本一致"）**
- 切块粒度：Go `extractEPUBTextItems`（`internal/parser/parser/epub_parser.go:197-210`）每个 spine 文档产 1 个 item，整篇纯文本，**不切块**。Python `_epub`（`rag/flow/parser/parser.py:1374-1389`）→ `EpubParser`（=`deepdoc/parser/epub_parser.py:RAGFlowEpubParser`，`deepdoc/parser/__init__.py:18` 别名）→ 对每个 spine 文档调用 `RAGFlowHtmlParser(item_path, binary=html_bytes, chunk_token_num=512)`（`epub_parser.py:66`），**按 512 token 切块**。→ 同输入 item 数：Go = spine 数 N；Python = 数十~上百 token chunk。**必然不一致**（小文档每 spine <512 token 时可能巧合一致，但粒度能力已不同）。
- 文本提取机制：Go 用正则 `stripHTMLTags`（`:245-277`）crude strip；Python 用 `RAGFlowHtmlParser` 结构化解析（表格/块级）。→ `text` 内容本身也会不同。
- 降级方案：把 epub 并入会话 B（html）一并处理，根因都是"Python 在 Parser 阶段内嵌 token 切块"。

**email —— 结构接近一致（单 item）；4 项差异中 2 项已对齐（PR 18005）、2 项接受差异**
- 1) `.msg` 支持：Python 走 `extract_msg.Message`（`parser.py:1316-1349`）；Go 显式报错（`email_parser.go:79-83`）。→ 输入 `.msg` 时 Go 直接报错、Python 出内容。**能力缺口**。
- 2) metadata 收集逻辑：**已对齐（PR 18005）**。Python 无条件建 `metadata` 字典（`:1253`），非基本 6 字段 header 全部进 metadata 且不受 `metadata` 是否在 fields 限制；基本 6 字段若不在 target_fields 则丢弃（`:1255-1261`）。Go 现对齐：`content["metadata"]=meta` 无条件（`email_parser.go:178-182`），非基本 header 恒进 meta，基本字段不在 fields 则丢弃（不泄漏进 meta）。测试 `TestEmailParser_MetadataAlwaysPresent`。
- 3) 附件 payload 解码：Python `payload.decode(part.get_content_charset())` 无 fallback 链（`:1306`），charset 缺失/失败即抛异常；Go 有完整 fallback 链 utf-8→gb2312→gbk→gb18030→latin1（`:345-361`）。→ 多编码附件下 Go 更稳健，结果可能不同。
- 4) `text_html` 空值处理：**已对齐（PR 18005）**。Python 总设 `text_html`（可空串，`:1296`）；Go 现无条件设 `content["text"]`/`content["text_html"]`（`email_parser.go:194-195`），纯文本邮件也产出空 `text_html`、HTML-only 产出空 `text`。测试 `TestEmailParser_TextHTMLAlwaysPresent`。

### 3.6 JSON（Go 独有，接受不对称）
- Go 有 `json_parser.go`（数组/对象/JSONL 判别，`:85-200`）；Python flow `function_map` / `setups` **无 json 分支**，flow 不处理 `.json`。
- **已确认决策（见 §2.3 决策 6）**：此不对称**接受**，Python flow **不补** json 分支，声明 flow 不支持 `.json` 即可。无需再议，无需二选一。邮箱会话 E 按计划只补 Go 侧测试。

---

## 4. 建议的并行拆分（每个会话认领一块）

按风险等级与依赖关系，建议如下拆分（各会话独立、互不冲突，最后统一回本文档的"对齐决策表"）：

| 会话 | 认领类型 | 任务 |
|---|---|---|
| A | markdown | 以 Python 为准对齐块边界（表格塌缩 / ck_type / delimiter）；切块保留现状；用拼接对比验证内容等价；写对拍测试 |
| B | html（+epub） | 以 Python 为准对齐块边界与表格表示；切块保留现状（不重构）；拼接对比验证内容等价；写对拍测试。**D2 表格塌方已修复（方案 a，纯 parser 内部，无需接线改动）** |
| C | text&code | 以 Python 为准对齐块边界：把 Go 硬编码的 `\n\n` 分段改为与 Python 相同的默认 delimiter `"\n!?;。；！？"`（含 keep_delimiters）；**不新增 Parser 端 delimiter 配置键**（Python 也无）；token 合并（OVER_CAP 128）保留现状；写对拍测试。**已提交 PR #18022**（基于 #18014 分支复用 `align_test.go` 框架；diff 含 #18014，待其合入后 rebase 到 `main`） |
| D | docx/pptx/xlsx/doc | 引擎差异保留（接受）；记录文本/分段发散原因；补基线测试 |
| E | epub/email/json | epub 并入 B；email 小修（Go 对齐 Python）；json 接受 Go 独有（Python 不补分支）；补基线测试 |

---

## 5. 每个会话的标准开工步骤（复现法）

1. **取样本**：为该类型准备 2-3 份代表性样本（含边界：表格、长段落、嵌套列表、图片引用、多 sheet 等）。
2. **跑 Go**：构造调用 `parser.GetParser(fileType).ParseWithResult(ctx, name, data)`，打印 `ParseResult.JSON`/`Text`/`HTML` 的 item 数与内容。
3. **跑 Python**：调用 `rag/flow/parser/parser.py` 的 `Parser._invoke`（或对应 `_xxx` 方法 + `set_output`），打印 `sections` 的 item 数与内容。
4. **拼接对比（一级闸门，优先做）**：把两端 items 的 `text` 按 item 顺序拼成一段，归一化（统一换行、折叠空白、strip；更稳可再转 token 多集合）后比较。相等即「内容等价」——这是核心验收（决策见 §2.3：切块保留现状，验收重点是内容一致而非 item 边界）。
   - 图片/表格项无 `text`（或表示不同），需**单独**比 item 数与存在性，不计入文本拼接。
5. **逐项 / 计数比对（二级）**：在拼接等价基础上，再比 item 数、`text`、`doc_type_kwd`、`ck_type`、`image` 是否存在/取值，定位剩余边界差异。**粒度基准以 Python 为准**（§2.3）：Go 的块边界应收敛到 Python。
6. **记录决策**：在本文档 §6「对齐决策记录表」对应行写明结论与关键决策（以 Python 为准 / 接受差异+原因）。
7. **补测试**：在对应包加对拍测试（Go 端见 `internal/parser/parser/*_test.go` 现状；Python 端可在 `rag/flow/parser/` 或 test 目录），测试应以拼接对比为断言主体。

注意 Go 构建依赖：`//go:build cgo` 的文件（docx/xls/xlsx/ppt/doc）需要 `office_oxide` 原生库，用 `bash build.sh --test ./internal/parser/parser/...` 跑，勿直接用裸 `go test`（会缺 CGO 标志）。详见 `AGENTS.md`「Go Test Tiers」。

---

## 6. 对齐决策记录表（各会话回填）

| 类型 | 结论（一致/以X为准/接受差异） | 关键决策 | 测试位置 | 负责人/会话 |
|---|---|---|---|---|
| markdown | ✅ 基本一致（表塌缩已修复 + 接受字段差异） | 块边界 Go 收敛到 Python（**方案 Y**：表内联为单个原始 `<table>` HTMLBlock + 独立 `doc_type_kwd:"table"` item，与 Python `_markdown` 一致；非表格 HTML 块不误标 table）；**ck_type 保留**为 Go 额外字段（仅真实表带 `ck_type:"table"`，下游实际读 `doc_type_kwd`）；**delimiter 接受差异**，拼接对比用 `WithDelimiterStrip` 替换为空格保持 token 间距；图片按 block 关联（修复整篇扫描误标 bug）；拼接对比为一级闸门 | `TestMarkdownParser_TableNotCollapsed`（表不塌缩 + 表 item 真实文本 + 非表格 HTML 不误标）+ `TestMarkdownParser_NonTableHTMLBlockNotTable` + `TestMarkdownParser_AlignmentGolden`（拼接归一化 golden，基于 `testdata/markdown.python.golden.json`，由 `testdata/gen_markdown_golden.py` 从真实 Python 引擎生成） | A |
| html | 以 Python 为准（粒度）+ 表格已修复 | 切块保留现状；**表不塌缩（方案 a，顶层 + 嵌套均覆盖）**：`<table>` 标记内联为 `doc_type_kwd:"text"` 副本 + 独立 `doc_type_kwd:"table"/ck_type:"table"` 项（walkHTMLBlocks 循环后 append，顺序同 markdown）；顶层表由 `walkHTMLBlocks case "table"` 处理，`<table>` 被 div/section/article 包裹的嵌套表由 `walkHTMLLeaf` 的 `<table>` 感知分支处理（保结构且**不改粒度**）；纯 parser 内部修复、**无需 Chunker/dispatch 接线改动**（早期"需改接线走 emitOne"判断已证伪）；拼接对比验证内容等价（HTML 标题 `#` 标记 / HTML 标签 / 空白折叠归一化） | `TestHTMLParser_TableProducesStructuredItems`（顶层表结构守护）+ `TestHTMLParser_NestedTableProducesStructuredItems`（嵌套表结构守护，原塌方案例）+ `TestHTMLParser_AlignmentGolden`（拼接归一化 golden，基于 `testdata/html.python.golden.json`，由 `testdata/gen_html_golden.py` 从真实 Python `RAGFlowHtmlParser` 生成；golden 把 `<table>` 内联在文档位置为 `doc_type_kwd:"text"` 项，刻意不调 `chunk_block(512)` 以停在 parser-block 粒度） | B（D2 已落地） |
| text&code | ✅ 已完成（以 Python 为准对齐，PR #18022） | 切块保留现状；Go 由硬编码 `\n\n` 改为与 Python 相同的默认 delimiter `"\n!?;。；！？"`（含 keep_delimiters=True，与 `_code` 实际调用一致）；不新增 Parser 端 delimiter 配置键；token 合并（OVER_CAP 128）保留现状由 Chunker 负责；移除 8192B 单 item 上限（parser 退化为纯分隔符切分器，尺寸管理交给 Chunker / embedding 截断，与 Python `parser_txt` 一致）；CRLF/CR→LF 归一化 | `TestTextParser_AlignmentGolden`（table-driven 对拍 en/zh，基于 `testdata/textcode.python.en.golden.json` / `testdata/textcode.python.zh.golden.json`；按「不提交 generator」原则，baseline 由 golden 的 meta 块可复现）+ `TestTextParser_ParseWithResult_DefaultDelimiter`（钉死新 delimiter 切分 + 保留分隔符） | C（PR #18022） |
| docx | 接受差异（引擎） | office_oxide vs python-docx，文本/分段发散保留 | 待补对拍测试（会话 D） | D |
| xlsx/xls/csv | 接受差异（引擎） | office_oxide/excelize vs ExcelParser，保留 | 待补对拍测试（会话 D） | D |
| pptx/ppt | 接受差异（引擎） | office_oxide vs python-pptx，保留 | 待补对拍测试（会话 D） | D |
| doc | 接受差异（引擎） | office_oxide vs tika，且输出格式不同（Go text / Py json），保留 | 待补对拍测试（会话 D） | D |
| epub | 不一致（接受差异/并入会话B） | Python 在 Parser 内嵌 512-token 切块（同 HTML §3.2）；Go 每 spine 1 item，Python 每 spine 多 chunk；文本提取机制也不同 | 待补对拍测试（与 html 合并，会话 B） | E |
| email | ✅ 已完成（以 Python 为准对齐 + 接受差异） | .msg：接受差异（Go 无 MSG 后端）；metadata 门控逻辑已对齐（PR 18005）；附件解码 Go 更稳健接受差异；text_html 空值已对齐（PR 18005） | email_parser_test.go（8 用例含 MetadataAlwaysPresent/TextHTMLAlwaysPresent，会话 E） | E |
| json | 接受不对称（Go 独有） | Python flow 不补 json 分支；声明 flow 不支持 .json | 无需（仅 Go 侧测试） | E |

---

## 7. 关键文件索引（快速跳转）

**Go 端**
- 分派/调用：`internal/ingestion/component/parser.go`（`ParserComponent.Invoke`）、`internal/ingestion/component/parser_dispatch.go`（`dispatchParse` / `GetParser` 入口 `parser_type.go:31`）
- 解析器：`internal/parser/parser/{markdown,html,text,docx,xlsx,xls,csv,doc,pptx,epub,email,json}_parser.go`
- 输出契约：`internal/parser/parser/parse_result.go`（`ParseResult`）、`internal/ingestion/component/schema/parser.go`（`Page`/`ParserSetup`/`ParserParam`）

**Python 端（flow，正确对应物）**
- `rag/flow/parser/parser.py`（`Parser` 类、`_invoke`、`set_output`、`_markdown/_html/_code/_docx/_spreadsheet/_slides/_epub/_email/_doc`、`function_map`、`setups`）
- 底层实现（被 flow 委托）：`deepdoc/parser/{markdown,html,txt,docx,excel,ppt,epub,json}_parser.py`、`rag/app/naive.py`（`Markdown`/`Docx`/`Html` 包装 + `naive_merge*`）

**切勿混淆**：`internal/deepdoc/parser/docx/` 是 Go 端另一套 DOCX 解析器，**未被 ingestion Parser 组件接入**；ingestion 实际用的是 `internal/parser/parser/docx_parser.go`。
