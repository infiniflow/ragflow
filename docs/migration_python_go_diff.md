# Ingestion Pipeline Python -> Go Migration Diff

> Scope: `File -> Parser -> Chunker -> Extractor -> Tokenizer` migration from Python (`rag/app/`, `rag/flow/`, `rag/svr/task_executor.py`, `rag/nlp/`) to Go (`internal/ingestion/component/`). The `File` component is explicitly excluded — no validation needed.
>
> Generated: 2026-07-23. Last updated: 2026-07-29 (batch-5 from PR #17419 synced into the local branch via ff-merge of 78872c18 + a8f94721 on 2026-07-27; Chunker 2.1 `sentenceDelimiter` re-confirmed: the English-period `|\.\s` fallback was intentionally dropped (does not match Python); Parser 2.7 re-confirmed as an intentional unsupported stub pending the underlying video-analysis capability, to be wired by a colleague (GitHub issue #17433); token.go / tokenizer.go / media_dispatch.go line numbers re-verified against the merged code; Parser diffs 2.2/2.4/2.5/2.10/2.11 fixed; 2.12 verified aligned, not a gap; Tokenizer diffs 1/3 fixed, 2 downgraded to Keep Go; Extractor diff 19 fixed — string-form `prompts` normalized; Parser 1.6 spreadsheet TCADP + 1.17 PDF TCADP params moved to Already fixed after code verification; Parser 2.11 residual noted — default `"text"` vs allowed `{"json"}` contradiction still open; Wave 2 cleaned of fixed items). Working-tree fixes re-verified against current code on 2026-07-24: line numbers corrected for 1.15 (`media_dispatch.go:176`), 2.5 (`markdown_vision_dispatch.go:84`), Tokenizer diff 3 (`tokenizer.go:441`); Parser diff 2.1 reclassified to P1 and Fixed 2026-07-24 (Go `maybeDispatchImage` mirrors `rag/app/picture.py`, not `rag/flow/parser/parser.py:_image`; the gap was output shape, not the mirrored Python path — see header note below). Doc-quality pass on 2026-07-24: all "Fixed" claims re-confirmed against source (`SplitKeywords` at `internal/utility/split.go:53`; title-trim at `tokenizer.go:428,441`); Effort Summary recomputed to open items only; Verified-facts line corrected (Cat C params now read); `eng=false` TOC residual noted under 1.9/1.10; Wave 1/2/4 cleaned of fixed/non-gap items. Line numbers reflect the code at that time; re-confirm before acting. Parser 2.1 reclassified (2026-07-24): the original P0 claim of "mirrors wrong Python path" was incorrect — the production image path is `rag/app/picture.py` (via `task_executor` FACTORY), which Go already mirrors correctly; the real gap was incomplete output shape (dropped `image`/`doc_type_kwd`, nil JSON on the default path), resolved by the `maybeDispatchImage` rewrite — downgraded from P0 to P1 and now Fixed. Chunker 2.6 ported 2026-07-27: `normalizeOverlappedPercent` (`chunker/common.go`) now mirrors `common/float_utils.py:50-58` — accepts `[0,1)` fraction or `[0,90]` percent, `×100` + `int()` truncate + clamp `[0,90]`; 2.7 and 2.10 were duplicate IDs and have been merged into 2.7.

> **Path & line convention (2026-07-29):** file:line references use bare filenames; canonical locations are: Chunker logic (`token.go`, `title.go`, `group.go`, `hierarchy.go`, `qa.go`, `one.go`, `tag.go`, `pdfcrop_cgo.go`, `register.go`, `image_upload.go`, `common.go`) → `internal/ingestion/component/chunker/`; Parser parse-stage implementations (`pdf_parser_common.go`, `pdf_parser_cgo.go`, `docx_parser.go`, `docx_postprocess.go`, `html_parser.go`, `html_postprocess.go`, `text_toc.go`, `markdown_parser.go`, `xls_tcadp.go`, `xlsx_parser.go`, `xls_parser.go`, `pdf_parser_tcadp.go`, `pptx_tcadp.go`, `pptx_parser.go`, `email_parser.go`, `pdf_postprocess.go`, `json_parser.go`) → `internal/parser/parser/`; Parser orchestration/dispatch (`parser.go`, `media_dispatch.go`, `parser_dispatch.go`, `pdf_vision_dispatch.go`) and schema → `internal/ingestion/component/` (schema under `schema/`); Extractor logic (`extractor.go`, `extractor_tag.go`) → `internal/ingestion/component/`; `SplitKeywords` → `internal/utility/split.go`. **Line numbers are indicative only** — the function/symbol name is the authoritative anchor; re-verify against current code before acting.

## Overview

Items are classified into 4 categories:

| Cat | Meaning | Action |
|-----|---------|--------|
| **A** | DSL orchestration already replaces the Python-specific component (e.g. book/paper/laws/manual/resume/email chunkers are TitleChunker/TokenChunker + Parser params in Go DSL templates) | Not a gap — removed from work list |
| **B** | Parameter relocated (Python Parser setup → Go Chunker DSL), not missing | Not a gap — note only |
| **C** | DSL field exists in `defaultSetups()` and templates but Go Parser/Chunker code never reads it | Real gap — feature unimplemented |
| **D** | Core algorithm/branch missing, cannot be replaced by DSL config | Real gap — needs implementation |

### Verified facts

- DSL templates exist for all Python chunker types: `internal/ingestion/pipeline/template/ingestion_pipeline_{book,paper,laws,manual,resume,email,picture,audio,qa,table,presentation,one,general}.json`.
- book/paper/laws/manual/resume use `TitleChunker`; email uses `TokenChunker`; picture/audio use `OneChunker` (Parser handles OCR/VLM/ASR).
- `remove_toc`/`flatten_media_to_text`/`remove_header_footer` are in `defaultSetups()` (`parser.go:242-283`) and DSL templates, and are now consumed by the backends (DOCX/HTML via `docx_postprocess.go`/`html_parser.go`/`text_toc.go`, markdown via `markdown_parser.go`, PDF via `pdf_parser_common.go`) — these are no longer unread. The earlier `eng` residual (hardcoded `false` at the DOCX/HTML call sites) is **closed** (2026-07-30): `removeTOCWord`/`removeContentsTable` now receive `isEnglishItems(items)` (`text_toc.go`, mirroring Python `_is_english` content heuristic), so English TOC removal uses the 2-word English prefix instead of the CJK 3-char prefix (see Cat C 1.9/1.10).
- audio/video use tenant default models (`resolveTenantModelByType`, `media_dispatch.go:162,256`), not `setup["vlm"]["llm_id"]` — the Python flow `check()` for vlm.llm_id does not apply.

### Already fixed

- Parser 2.8 (FileType→family mapping: `visual`→`image`, `aural`→`audio`) — `parser_dispatch.go:312-318` confirmed correct.
- Parser 1.19 (`check()` business validation) — `Check()` method added to `parser.go`, called by `NewParserComponent`.
- Parser 1.18 (`image2id`) — not a gap; image upload is intentionally relocated to Chunker stage via `imageUploadDecorator` + `DefaultImageUploader` (`chunker/image_upload.go`).
- Parser 1.2 (PDF `remove_header_footer`) — not a gap; PDF backend reads it via `ConfigureFromSetup` (`pdf_parser_common.go:144`) and wires it into the CGO struct (`pdf_parser_cgo.go:46`).
- Parser 1.3 (PDF `flatten_media_to_text`) — not a gap; `pdf_parser_common.go:139` + `pdf_parser_cgo.go:44`.
- Parser 1.4 (PDF `remove_toc`) — not a gap; `pdf_parser_common.go:142` + `pdf_parser_cgo.go:45`, with post-processing in `pdf_postprocess.go` (`applyRemoveTOC`).
- Parser 1.5 (PDF `enable_multi_column`) — not a gap; `pdf_parser_common.go:147` + `pdf_parser_cgo.go:43`.
- Parser 1.15 (image `system_prompt`) — **Fixed**: `media_dispatch.go:176` read `setup["prompt"]` (the video family's key) instead of `setup["system_prompt"]`, so the user-configured image VLM prompt was silently dropped. Now reads `setup["system_prompt"]`, matching Python `parser.py:1119` and Go `defaultSetups()` (`parser.go:295`). The `video` family is currently an explicit "not yet supported" stub (see diff 2.7), so no `setup["prompt"]` read remains.
- Parser 2.2 (email default `output_format`) — **Fixed** (2026-07-24): `defaultSetups()` email family changed from `"text"` to `"json"` (`parser.go:305`), matching Python `parser.py:212`. Email backend emits structured JSON items (from/to/cc/date/subject/body/attachments/metadata).
- Parser 2.4 (DOCX vision trigger inverted) — **Fixed** (2026-07-24): `maybeDispatchDOCXVision` now triggers only on `OutputFormat == "json"` and enriches JSON items (`doc_type_kwd` image/table with non-empty `image`) by appending the VLM description to `text` — mirroring Python `enhance_media_sections_with_vision` (`utils.py:162`). The markdown path is no longer enhanced. Removed the dead markdown-string insertion path and `extractDOCXFiguresFromDispatch`.
- Parser 2.5 (vision skips table) — **Fixed** (2026-07-24): `markdown_vision_dispatch.go:84` now also handles `table` (`kd != "image" && kd != "table"`); the existing `image != ""` guard already matches Python's `item.get("image") is not None`.
- Parser 2.10 (PDF `parse_method` whitelist) — **Fixed** (2026-07-24): `isNamedPDFParseMethod` in `pdf_vision_dispatch.go` now matches the `Check()` 8-value whitelist exactly — removed `"plain text"`/`"plaintext"`/`"tcadp"` (kept `"tcadp parser"`); added the `@mineru` layout suffix to match `@paddleocr`/`@somark`/`@opendataloader`.
- Parser 2.11 (audio output_format data loss) — **Fixed**: `maybeDispatchAudio` now wraps the transcription as JSON items (`text` + `doc_type_kwd:"audio"`) when `output_format == "json"`, fixing the data-loss bug; and the audio default was changed from `"text"` to `"json"` in `defaultSetups()` (`parser.go:314`), aligning with `AllowedOutputFormat["audio"]={"json"}` (`schema/parser.go:88`). The `"text"`-default residual noted in the 2026-07-24 working-tree pass is now closed (see Semantic Differences 2.11).
- Parser 2.12 (PDF markdown figure rendering) — **Not a gap** (2026-07-24): Go already inlines figure base64 in `sectionsToMarkdown`/`inlinePNGDataURL` (`pdf_parser_common.go:443-473`), matching `parser.py:724-725`; covered by `pdf_parser_common_test.go:305`. The genuine missing capability is PDF `enhance_media_sections_with_vision` (Parser omission 1.1), tracked separately.
- Parser 1.6 (spreadsheet TCADP parser) — **Fixed** (2026-07-24): `parseSpreadsheetWithTCADP` implemented in `xls_tcadp.go:16-83`; wired into `xlsx_parser.go:97-103` and `xls_parser.go:80-86`. Sends `TableResultType`/`MarkdownImageResponseType` config.
- Parser 1.17 (PDF TCADP `table_result_type` / `markdown_image_response_type`) — **Fixed** (2026-07-24): `pdf_parser_tcadp.go:38-41` sends `TableResultType` + `MarkdownImageResponseType` in the TCADP request body; `pdf_parser_common.go:252-257` reads them from setup.

### Cross-cutting systemic issues

1. **PDF position/coordinate chain** — mostly closed on the TokenChunker path: `restore_pdf_text_previews` (1.3, text-chunk preview via `cropImageChunks` + `imageUploadDecorator`), `merge_pdf_positions` (1.6/2.8, `removeTag` + `mergePositionMatrix`), and `positions`→`position_int`/`page_num_int`/`top_int` (1.4, owned by `internal/ingestion/task.processChunkPositions`). **Fixed** (2026-07-27): `invokeGroup` / `invokeHierarchy` (and the `TitleChunker` dispatcher) now acquire the PDF engine via `newPDFEngineFromUpstream` and call `cropTitleChunks` (mirroring `token.go:260`), so PDF previews are generated on the Title/Group/Hierarchy paths too — the previous residual is closed. `cropTitleChunks` derives `ck_type` from `doc_type_kwd` (the Group/Hierarchy chunks don't emit `ck_type`) so `needsCrop` (`pdfcrop_cgo.go:151`) fires, then strips the derived `ck_type` from the output so the downstream chunk shape is unchanged.
2. **`remove_tag` cleanup** — largely resolved: Group/Hierarchy (1.6/2.8, `buildChunksFromRecordGroups` calls `removeTag` + `mergePositionMatrix`); TokenChunker overlap (2.2, both text-path `mergeOrNew` and JSON-path `mergeByTokenSizeFromJSON` strip parser tags before computing overlap). **Fixed** (2026-07-27): both TokenChunker output paths now strip parser tags from the final chunk text — text path at `token.go:403` and JSON path at `token.go:809` (via `removeTag`), so `@@...##` markers no longer leak into indexed/embedded chunk text on the non-overlap merge path either.
3. **LLM call robustness gap** — Extractor lacks retry/`message_fit_in`/temperature; Tokenizer timeout default wrong (600s vs 60s); concurrency limits diverge.

---

## P0 (correctness-impacting, fix first)

| ID | Issue | Cat | Python | Go |
|----|-------|-----|--------|-----|
| Parser-2.4 | DOCX vision enhancement trigger inverted — **Fixed** (2026-07-24) | D | `rag/flow/parser/parser.py:911` | `internal/ingestion/component/docx_vision_dispatch.go` |
| Extractor-1 | tag matching algorithm fundamentally different (ES vs Jaccard) | D | `rag/nlp/search.py:813-823` | `internal/ingestion/component/extractor_tag.go:528-608` |

---

## Parser Component

### Go Omissions

#### Cat A — DSL orchestration already replaces (NOT a gap)

| # | Item | Note |
|---|------|------|
| 1.16 | audio/video `vlm.llm_id` | Go uses tenant default models (`resolveTenantModelByType`); DSL does not need vlm config. See also diff 2.6. |
| 1.19 | `check()` business validation | **Fixed** — `Check()` method added. |
| 1.18 | `image2id` post-processing | **Not a gap** — relocated to Chunker via `imageUploadDecorator`. |

#### Cat B — Parameter relocated (NOT a gap)

| # | Item | Python | Go |
|---|------|--------|-----|
| 1.12 | text&code `chunk_token_num` / `delimiter` | Parser setup | TokenChunker DSL (`token_size`/`delimiters`) |
| 1.13 | HTML `chunk_token_num` | Parser setup | TokenChunker DSL |
| 1.14 | markdown `delimiter` | Parser setup | TokenChunker DSL |

#### Cat C — DSL field exists but was unread (all resolved)

These switches exist in `defaultSetups()` (`parser.go:242-283`) and DSL templates. 1.2–1.5 were never real gaps — the PDF backend already reads them (see Already fixed above). 1.8–1.11 and 1.15 were genuine gaps and are now fixed; their fix notes are below.

| # | Item | DSL field | Impact | Sev | Diff | Effort | Status |
|---|------|-----------|--------|-----|------|--------|--------|
| 1.8 | DOCX `remove_header_footer` | default false | header/footer mixed into text | P2 | Low | S | **Fixed** — `docx_parser.go` reads `remove_header_footer`; filtering in `docx_postprocess.go`. |
| 1.9 | DOCX `remove_toc` | default false | TOC indexed | P2 | Mid | M | **Fixed** — `docx_postprocess.go` (`removeTOCWord` + `extractDOCXOutlines`). **Resolved (2026-07-30):** `eng` is no longer hardcoded `false` — `removeTOCWord` receives `isEnglishItems(sections)` (`docx_parser.go:111,153`), mirroring Python `_is_english` content heuristic, so English docs use the 2-word prefix correctly. Affects **only** the `remove_toc` heuristic path; `remove_header_footer` (precise match via `removeDOCXHeaderFooterSections`) is unaffected. |
| 1.10 | HTML `remove_toc` (+ `remove_header_footer`) | default false | nav/TOC mixed into text | P2 | Low | S | **Fixed** — `html_parser.go` + `html_postprocess.go` (`stripHTMLHeaderFooter`) + `text_toc.go` (`removeContentsTable`). **Resolved (2026-07-30):** `removeContentsTable` receives `isEnglishItems(items)` (`html_parser.go:89`), same content-based `eng` as 1.9. Affects **only** `remove_toc`; `remove_header_footer` via `stripHTMLHeaderFooter` is precise-match and unaffected. |
| 1.11 | markdown `flatten_media_to_text` | default false | cannot force media→text | P3 | Low | S | **Fixed** — `markdown_parser.go` `FlattenMediaToText` + `walkMarkdownBlocksWithImages`. |
| 1.15 | image `system_prompt` | default "" | VLM cannot receive custom prompt | P1 | Low | S | **Fixed** — `media_dispatch.go:176` now reads `setup["system_prompt"]` (was `setup["prompt"]`). |

#### Cat D — Core algorithm/branch missing (real gap)

| # | Item | Impact | Sev | Diff | Effort |
|---|------|--------|-----|------|--------|
| 1.1 | PDF `enhance_media_sections_with_vision` post-processing | image/table entries lack LLM description, losing media semantics | P1 | Mid | M | **Fixed** (2026-07-27) — `maybeDispatchPDFVisionEnhancement` added to `pdf_vision_dispatch.go` (103 lines, mirrors Python `utils.py:162-209`): after PDF parser produces JSON items, image/table items (`doc_type_kwd` in {"image","table"} with non-empty `image` field) are enriched via IMAGE2TEXT VLM call; description appended to `text` field. Wired after `dispatchParse` in `parser.go` alongside existing DOCX/Markdown vision enhancement |
| 1.7 | slides TCADP parser | missing TCADP slides parsing branch | P2 | Mid | M | **Fixed** (2026-07-27) — `parsePresentationWithTCADP` implemented in `pptx_tcadp.go` (83 lines, mirrors `xls_tcadp.go`); wired into `PPTXParser.ParseWithResult` when `parse_method == "tcadp"`; `PPTParser` delegates through a shared `PPTXParser` instance with `ConfigureFromSetup` forwarding |

### Semantic Differences

| # | Difference | Cat | Python | Go | Fix | Sev |
|---|------------|-----|--------|-----|-----|-----|
| 2.1 | image branch output shape incomplete (drops `image`/`doc_type_kwd`; default json path returned nil JSON so downstream Chunkers rejected with `errRequiredField{"json"}`) | D | `rag/app/picture.py:67-103` | `media_dispatch.go:90-210` | **Fixed** (2026-07-24, line re-verified 2026-07-27) — `maybeDispatchImage` (`media_dispatch.go:90`) now returns `OutputFormat:"json"` + `JSON:[{text, image:dataURI, doc_type_kwd:"image"}]` via `imageDispatchResult` (`media_dispatch.go:217`); OCR+VLM-combine path preserved. Note: Go mirrors `rag/app/picture.py` (production image path via `task_executor` FACTORY), NOT `rag/flow/parser/parser.py:_image` (flow-canvas component) — the original P0 claim of "mirrors wrong Python path" was incorrect | P1 |
| 2.2 | email default output_format | D | `parser.py:212` `"json"` | `parser.go` `"text"` | **Fixed** (2026-07-24) — email default changed to `"json"` (`parser.go:305`); backend emits structured JSON items | P1 |
| 2.3 | Go added `json` family not in Python | — | none | `parser.go:259-262` + `json_parser.go` (229 lines) | **Keep Go** (2026-07-27) — complete `.json` parser (handles JSON arrays, objects, JSONL) that Python doesn't provide natively; intentional feature enhancement, not a parity gap | P3 |
| 2.4 | DOCX vision trigger inverted | D | `parser.py:_doc` JSON branch (enhance_media_sections_with_vision) | `docx_vision_dispatch.go` (was markdown) | **Fixed** (2026-07-24) — triggers on JSON path, enriches JSON items whose `doc_type_kwd` is image/table with non-empty `image`; markdown path no longer enhanced | P0 |
| 2.5 | vision enhancement skips table | D | `parser/utils.py:181` (image+table) | `markdown_vision_dispatch.go:84` (image only) | **Fixed** (2026-07-24) — `kd != "image" && kd != "table"`; preserves `image != ""` guard | P1 |
| 2.6 | audio/video model resolution | A | `parser.py:1149,1162` (require llm_id) | `media_dispatch.go:162,256` (tenant default) | **Keep Go** — Go behavior is more sensible | P3 |
| 2.7 | video parsing (provider-specific: raw bytes / inline_data / file://) | D | `_video` → `cv_mdl.async_chat(video_bytes=<raw bytes>)`; OpenAI系抽 3 帧→`image_url` data URI（`cv_model.py:213` `_describe_video_frame`），Gemini 原始字节 `inline_data` Blob 或 Files API `uri`（`cv_model.py:1067`），Qwen 临时文件 `file://`（`cv_model.py:438`） | `media_dispatch.go:54` `maybeDispatchVideo` | **Intentional stub (2026-07-27)** — Go now returns explicit `video parsing is not yet supported` error; the prior `video_url` data-URI path was removed because no model driver honors it (OpenAI ignores `video_url`; Gemini `googleMessageParts` only accepts `text`/`image_url` and silently drops `video_url`), so it produced no real video understanding. Real parsing deferred until the underlying video-analysis capability is built (provider-specific: OpenAI 系抽帧→`image_url` / Gemini `inline_data` / Qwen `file://`). Tracked as a known stub, **not a closed gap and not Keep Go**; the underlying video-analysis capability is to be wired in by a colleague (see GitHub issue #17433). | P1 |
| 2.9 | image output_format | D | `parser.py:1101` (hardcoded json) | `media_dispatch.go:217` (`imageDispatchResult`) | **Fixed** (2026-07-24, line re-verified 2026-07-27) — `imageDispatchResult` (`media_dispatch.go:217`) hardcodes `OutputFormat:"json"`; setup override removed; same change as 2.1 | P1 |
| 2.10 | PDF parse_method whitelist | D | `parser.py:257` | `pdf_vision_dispatch.go` (`isNamedPDFParseMethod`) | **Fixed** (2026-07-24) — aligned with `Check()` 8-value whitelist (`deepdoc`/`plain_text`/`mineru`/`docling`/`opendataloader`/`tcadp parser`/`paddleocr`/`somark`); removed `plain text`/`plaintext`/`tcadp`, kept `tcadp parser`. **Note:** `@mineru` is intentionally NOT a `parse_method` value here — it is a `layout_recognizer` selector resolved separately (see `pdf_vision_dispatch.go:62-68,378-382`), so it must not appear in this whitelist. | P2 |
| 2.11 | audio output_format default contradicts allowed list | D | `parser.py:232` | `parser.go` + `schema/parser.go` | **Fixed** (2026-07-27) — audio default changed from `"text"` to `"json"` in `defaultSetups()` (`parser.go:313`), matching Python and aligning with `AllowedOutputFormat["audio"]={"json"}` | P1 |
| 2.12 | PDF markdown missing figure image rendering | D | `parser.py:719-728` | `pdf_parser_common.go` | **Not a gap** (2026-07-24) — Go already inlines figure base64 via `sectionsToMarkdown`/`inlinePNGDataURL` (`pdf_parser_common.go:443-473`); matches `parser.py:724-725`; covered by `pdf_parser_common_test.go:305` | P1 |
| 2.13 | presentation (TCADP) items drop page/slide location | D | `presentation.py:148-149` (sets `position_int=[(pn+1,0,0,0,0)]` carrying the real 1-indexed page number; `top_int=[0]`) | `pptx_tcadp.go` → `pdfItemsToResult` (`pdf_parser_remote_common.go:19`) passes items through; `tcadpAnyToItems` (`pdf_parser_tcadp.go:135`) builds each item, now attaching the per-element page number as `positions:[page-1,0,0,0,0]` via the new `extractTCADPPage` helper (reuses the same key set as `collectPDFPageNumbers`, `pdf_parser_tcadp.go:178`); previously it discarded the page number that the raw TCADP response contains. The ingestion pipeline (`processChunkPositions`→`AddPositions`, `internal/ingestion/task/position.go:29`) converts this to `page_num_int=[page]`, `top_int=[0]`, `position_int=[[page,0,0,0,0]]`, matching Python `presentation.py:148-149`. Contrast: Table chunker path is parity-correct — Python `table.py` also sets no `top_int`/`position_int`/`page_number` (the `dummy position` tuple at `table.py:148` is an internal image-table struct, not a chunk-output field), and neither Go spreadsheet parser emits page info; elements without a page (spreadsheet TCADP) emit no `positions`, staying aligned. | **Fixed** (2026-07-27) — `tcadpAnyToItems` attaches per-slide `positions` so the pipeline derives `top_int`/`position_int`/`page_num_int`; covered by `TestTCADPAnyToItems_PropagatesPageNumber`. No downstream crash (consumers use `omitempty`/fallback). | P2 |

---

## Chunker Component

### Go Omissions

#### Cat A — DSL orchestration already replaces (NOT a gap)

Verified via `internal/ingestion/pipeline/template/ingestion_pipeline_*.json`:

| # | Type | DSL template chunker | Note |
|---|------|---------------------|------|
| 1.1a | book | TitleChunker | Parser `remove_toc:true` + TitleChunker |
| 1.1b | paper | TitleChunker | same |
| 1.1c | laws | TitleChunker | uses TitleChunker + `isColonTitle` (diff 2.9) instead of `tree_merge` |
| 1.1d | manual | TitleChunker | same |
| 1.1e | resume | TitleChunker | Parser `flatten_media_to_text:true` + TitleChunker |
| 1.1f | email | TokenChunker | `delimiter_mode:token_size` |
| 1.1g | picture/audio | OneChunker | Parser stage handles OCR/VLM/ASR; OneChunker passes through |

#### Cat B — Parameter relocated (NOT a gap)

| # | Item | Note |
|---|------|------|
| 1.2 | TokenChunker rejects `delimiter_mode=="one"` | Intentional split: "one" is a dedicated `OneChunker` component (picture/audio/one templates); no production path feeds "one" to TokenChunker, so rejecting it in `Validate` is by design, not a gap (**Keep Go**). |

#### Cat C/D — Real gaps (feature unimplemented)

| # | Item | Cat | Impact | Sev | Diff | Effort |
|---|------|-----|--------|-----|------|--------|
| 1.3 | Missing `restore_pdf_text_previews` | D | PDF text previews lost | P1 | Mid | M | **Fixed** (2026-07-24) — `needsCrop` (`pdfcrop_cgo.go`) now also returns true for `text` chunks carrying `PDFPositions`/`Positions`; `cropImageChunks` crops the text region and writes `ChunkDoc.Image` (base64 data URL), and `imageUploadDecorator` (`register.go`+`image_upload.go`) uploads it to `img_id`, mirroring Python `restore_pdf_text_previews` + `image2id`. Wired in the TokenChunker JSON path (`token.go:286`, `cropImageChunks` call). Residual: Title/Hierarchy chunkers do not yet call `cropImageChunks` (they reuse the same `needsCrop` gate but the crop invocation is only in `token.go`) — tracked as a follow-up, not blocking the token path. |
| 1.4 | Missing `_finalize_json_chunks` | D | context_above/below not concatenated, PDF coords not converted | P1 | Mid | M | **Fixed** (2026-07-24) — (a) `context_above/below` already concatenated in `buildChunkDoc` (`token.go:613`); (b) the PDF coord → `position_int/page_num_int/top_int` conversion is centralized in `internal/ingestion/task` (`processChunkPositions`→`AddPositions`), **not** in the chunker — the chunker's only real gap was dropping `positions`/`_pdf_positions` on merge, fixed in `mergeByTokenSizeFromJSON` (`extendRawJSONArray`) + `cloneChunkDoc` deep-copy; `ChunkDoc.ToMap` decodes `positions` to `[][]float64` consumed by `processChunkPositions` (verified). Diffs 2.3 / 2.5 are the same root cause. |
| 1.5 | Title series missing PDF outline detection | D | only regex, low accuracy | P1 | Mid | L | **Fixed** (2026-07-24) — `newLevelContext` (title.go) now tries `resolveOutlineLevels` first: the Parser-supplied `file.outline` (written by `outlinesToFileMeta` in `pdf_parser_common.go`, shaped `[]map{title,level,page_number}`) is read via `outlineFromInputs` and matched against each text line by character-bigram Jaccard similarity >0.8 (`outlineSimilarity`, ports `common.py:_outline_similarity`), assigning `level+1`; sparse outlines (ratio ≤0.03) and the no-outline case fall back to the existing regex/frequency branch. Wired into Group + Hierarchy title chunkers (group.go/hierarchy.go). |
| 1.6 | Group/Hierarchy missing `remove_tag` + `merge_pdf_positions` | D | parser tags leak, PDF positions lost | P1 | Mid | M | **Fixed** (2026-07-24) — `buildChunksFromRecordGroups` now calls `removeTag` (port of `pdf_parser.py:1934` `re.sub(r"@@[\t0-9.-]+?##","",txt)`) on the joined text and merges each record's `positions`/`_pdf_positions` via `mergePositionMatrix` (dedup + sort by page,top,left), mirroring `common.py:255` + `pdf_chunk_metadata.py:127`. Shared by Group and Hierarchy, so diff 2.8 is also closed. |
| 1.7 | Missing `BULLET_PATTERN` fallback | D | no auto bullet detection for laws etc. | P1 | Mid | M | **Fixed** (2026-07-24) — 4th-level fallback added to `resolveTitleLevels` (`title.go`): when outline + regex levels + layout all produce bodyLevel, `bulletsCategory` selects the best-matching bullet-pattern group (Chinese legal, numbering, Chinese numbering, English legal — mirroring Python `rag/nlp/__init__.py:258-320`) and assigns structural levels. Guarded by `allBodyLevel` so it never overrides an existing level from the outline or regex path. |
| 1.8 | QA/Tag/Table/Presentation/One chunkers missing fields (image/positions/top_int/page_num_int) | D | metadata lost | P1 | Mid | L | **Fixed** (2026-07-27) — all subitems closed: QA chunker (top_int row index, image id + doc_type_kwd, _pdf_positions/positions carried from source item — done 2026-07-24); Tag chunker (`TopInt` from source row index, `tag.go:128`); One chunker (`Positions`/`PDFPositions` preserved from source item, `one.go:147-148`); Table/Presentation confirmed pass-through (no construction, upstream fields preserved) |

### Semantic Differences

| # | Difference | Cat | Python | Go | Fix | Sev |
|---|------------|-----|--------|-----|-----|-----|
| 2.1 | TokenChunker sentence delimiter | D | `naive.py:901` + `nlp/__init__.py:1217` (`"\n!?。；！？"`) | `token.go:338` (`sentenceDelimiter` var) | **Fixed** (2026-07-24, lines re-verified 2026-07-29) — extracted to package-level `sentenceDelimiter` variable (`token.go:338`) = `(\n|[!?。；！？])`. Note: the earlier mirror kept a trailing `|\.\s` English-period fallback (`(\n|[!?。；！？]|\.\s)`); that fallback was **intentionally dropped** — it was a mistake and does not match Python `naive.py:901`/`nlp/__init__.py:1217` (`"\n!?。；！？"`). The batch-5 merge conflict on this regex was resolved by keeping the upstream version without `\.\\s`. | P1 |
| 2.2 | overlap missing `remove_tag` | D | `nlp/__init__.py:1181` | `token.go:403` | **Fixed** (2026-07-24, line re-verified 2026-07-27) — text path (`token.go:403`) and JSON path (`token.go:809`) both call `removeTag()` on the previous chunk text before computing the overlap suffix, matching Python | P1 |
| 2.3 | missing pos handling | D | `nlp/__init__.py:1174-1192` | `token.go:771` | add position append | P1 | **Fixed** (2026-07-24, line re-verified 2026-07-27) — `mergeByTokenSizeFromJSON` (`token.go:771`) now extends `Positions` (and `_pdf_positions`) across merges via `extendRawJSONArray`, mirroring `token_chunker.py:240`. Same root cause as 2.5; coord→position_int conversion is owned by `internal/ingestion/task` `processChunkPositions`. |
| 2.4 | media context uses 4 bytes/token approximation | D | `token_chunker.py:153-166` (sentence boundary + exact count) | `token.go:739-768` | **Fixed** (2026-07-24, line re-verified 2026-07-27) — `takeFromEnd`/`takeFromStart` (`token.go:739-768`) now use `tokenizeStr` for exact BPE token counting instead of the `tokens * 4` byte heuristic | P1 |
| 2.5 | merge drops PDF positions | D | `token_chunker.py:240` | `token.go:836` | merge `PDFPositions` | P1 | **Fixed** (2026-07-24, line re-verified 2026-07-27) — `mergeByTokenSizeFromJSON` merge branch extends `PDFPositions`/`Positions` via `extendRawJSONArray` (`token.go:880`, used at `token.go:836`); `cloneChunkDoc` (`token.go:844`) deep-copies the coordinate byte slices so clones don't alias the source. Same root cause as 2.3. |
| 2.6 | overlapped_percent missing Python normalization | D | `common/float_utils.py:50-58` (canonical `[0,90]` int: `0<x<1 → ×100`, `int()` truncate, clamp) | `schema/chunker.go:225` (validate `> 90`) + `token.go:707` (takeFromEnd/takeFromStart overlap math) | **Fixed** (2026-07-27, line re-verified 2026-07-27) — ported `schema.NormalizeOverlappedPercent` (`schema/chunker.go:264`) mirroring `common/float_utils.py:50-58`: accepts a `[0,1)` fraction or `[0,90]` percent, `0<x<1 → ×100`, `int()` truncation, clamps `[0,90]` (also mirrors Python's `float()` by accepting numeric strings and returning `0` on NaN/Inf/bad input). Wired into `tokenChunkerParam.Update` (`token.go:80`); merge math at `token.go:707` (takeFromEnd/takeFromStart) already matched. `Validate` (`schema/chunker.go:225`) now only guards direct struct construction. | P2 |
| 2.7 | paragraph / oversize splitting | D | flow `token_chunker.py` `naive_merge` vs app `naive.py` paragraph split | `token.go:348` (`mergeByTokenSize`) + `token.go:455` (`sentenceDelim.Split`) | **Fixed (2026-07-27, line re-verified 2026-07-27) — strict mirror of `naive_merge` (`rag/nlp/__init__.py`), superseding the earlier CRLF-only fix:** (a) removed `splitIntoSections` blank-line pre-split; `mergeByTokenSize` now treats the whole payload as a single section (`sections := []string{text}`, mirroring `rag/nlp/__init__.py:1162-1163`), so blank-line runs are preserved verbatim and a paragraph is never a hard chunk boundary; (b) oversize sections split on sentence delimiters with the delimiter DROPPED to match Python — `naive_merge` (`rag/nlp/__init__.py:1222-1225`) uses a capturing-group `re.split` but skips any pure-delimiter segment via `re.fullmatch(dels, sub_sec)`, discarding the delimiter char, then feeds each kept segment to `add_chunk("\n"+sub_sec)`; Go mirrors this exactly with `sentenceDelim.Split` (`regexp.Split`, discards the delimiter, `token.go:453`) and prepends `"\n"` to each segment (`token.go:464-465`). The `splitKeepingDelimiter` helper exists but is used elsewhere, not for oversize splitting; (c) CRLF normalization at entry (landed earlier). Alignment target confirmed as `naive_merge` in `rag/nlp/__init__.py` (not `rag/app/naive.py`, the parser stage; `rag/flow/parser/token_chunker.py` does not exist in this tree). Guarded by `TestMergeByTokenSize_PreservesBlankLines`, `TestMergeByTokenSize_OversizeDropsDelimiters`, and `TestMergeByTokenSize_OversizeDropsBlankLines` (CRLF still covered by `TestMergeByTokenSize_CRLFNormalization`). | P2 |
| 2.8 | Group/Hierarchy missing `remove_tag` | D | `common.py:255` | `group.go:224-231` | add call | P1 | **Fixed** (2026-07-24) — `buildChunksFromRecordGroups` (shared by Group+Hierarchy) now calls `removeTag` on the joined text; the `merge_pdf_positions` part is diff 1.6. |
| 2.9 | TitleChunker has `isColonTitle` | — | flow `common.py:206-239` (none) vs app `laws.py:257` (has) | `title.go:332-336` (has) | **Keep Go** — intentional replacement for laws `make_colon_as_title` | P3 |
| 2.11 | empty-text merge handling | D | `token_chunker.py:236-239` | `token.go:826` | **Fixed** (2026-07-24, line re-verified 2026-07-27) — JSON merge path (`token.go:826`) now assigns incoming text directly when `prev.Text == ""` instead of silently dropping it, matching Python | P2 |
| 2.12 | QAChunker `rmQAPrefix` regex | D | `qa.py:241` (`[\t:： ]+`) | `qa.go:183` (single sep) | **Fixed** — `qa.go:183` (`rmQAPrefixRe`) now uses `[\t:： ]+`, matching `qa.py:241` (2026-07-24) | P2 |
| 2.13 | QAChunker default language | D | `qa.py:299` (default Chinese) | `qa.go:101` (empty→English) | **Fixed** — `qa.go:101` defaults to Chinese when Lang is empty, matching `qa.py:299` (2026-07-24) | P2 |
| 2.15 | children_delimiters no backtick regex | D | app `naive.py:903-909` (supports) vs flow `token_chunker.py:309` (no) | `token.go:1007` (compileChildrenPattern, no backtick regex) | **Keep Go** — aligns with flow Python | P3 |

---

## Extractor Component

### Go Omissions (all Cat D — real gaps)

| # | Item | Impact | Sev | Diff | Effort |
|---|------|--------|-----|------|--------|
| 1 | keyword/question LLM result caching | ~~repeated LLM calls, high cost~~ | ~~P1~~ | Mid | **Keep Go** — checkpoint (pipeline-level fingerprint reuse) already covers the main use-case. Within a single run, chunk text is almost never identical (each page/paragraph is different content); header/footer repetition is negligible (2-3 chunks per 100). The LRU-cache overhead and Redis round-trip cost outweigh the near-zero hit rate. |
| 2 | metadata extraction (`gen_metadata`) | cannot extract structured metadata | P1 | Mid | **Deferred** (2026-07-28) — TOC and metadata extraction will be ported to the compilation layer in a future phase, not within the current migration scope |
| 3 | TOC extraction | Go explicitly returns error | P1 | Mid | **Deferred** (2026-07-28) — TOC and metadata extraction will be ported to the compilation layer in a future phase, not within the current migration scope |
| 4 | LLM gen params (temperature etc.) | cannot configure gen params | P1 | Low | **Fixed** (2026-07-28) — `extractorTemperature=0.2` set in `call()` (`extractor.go`); conveys via `extractorChatRequest.Temperature` |
| 5 | `message_fit_in` truncation | ~~over-long messages may exceed max_length~~ | ~~P1~~ | Mid | **Keep Go** — if a chunk exceeds the model's max_length, the correct fix is at the Chunker layer (enforce per-chunk token limits), not truncating mid-extraction which destroys semantic boundaries. Python's `message_fit_in` was belt-and-suspenders; Go intentionally leaves it out. |
| 6 | LLM retry | single failure gives up | P1 | Mid | **Fixed** (2026-07-28) — Extracted `RetryWithBackoff` into `internal/common/retry.go` (3 retries, 2s exponential backoff capped 1min, ctx-aware sleep). Extractor `call()` wraps `inv.Chat()` with `common.RetryWithBackoff`. Retry exhaustion → fail-stop (persistent errors don't log+skip). Tests use `extractorRetryDelay=time.Millisecond` override for fast execution. |
| 7 | tag file "." → "_" replacement | tag with dots indexes inconsistently | P2 | Low | S |
| 8 | tag content leading newline | first-line format diff | P3 | Low | S |

### Semantic Differences

| # | Difference | Cat | Python | Go | Fix | Sev |
|---|------------|-----|--------|-----|-----|-----|
| 1 | tag matching algorithm (ES vs Jaccard) | — | `search.py:813-823` | `extractor_tag.go:528-608` | **Deferred** (2026-07-28) — assigned to colleague for dedicated ES-integration work; not in current scope | P0 |
| 2 | tag data source (tag_kb_ids vs tag_file_id) | — | `task_executor.py:559-572` | `extractor_tag.go:219-265` | add `TagKbIDs` + ES path | P1 |
| 3 | keyword/question temperature | D | `generator.py:230,245` (0.2) | `extractor.go:689-720` (unset) | **Fixed** (2026-07-28) — `extractorTemperature=0.2` set in `call()`; Temperature conveyed via `extractorChatRequest` | P1 |
| 4 | question_prompt Task missing topn | D | `question_prompt.md:5` | `extractor.go:131` | **Fixed** (2026-07-27, lines re-verified 2026-07-29) — `autoQuestionPrompt` already contains `top %d` (`extractor.go:131`), substituted via `fmt.Sprintf` at `extractor.go:688`; matches `question_prompt.md:5`. No change needed — was already aligned | P2 |
| 5 | tag prompt duplicate "Output:" | D | `content_tagging_prompt.md:32` (none) | `extractor_tag.go:40-65,687` | remove template trailing Output: + add trailing space to user msg | P2 |
| 6 | tag examples JSON format | D | `generator.py:319` (indent=2) | `extractor_tag.go:722` (compact) | `MarshalIndent` | P2 |
| 7 | tag TAG SET ordering | — | Python ES-agg order | Go alphabetic | **Keep Go** — more stable | P3 |
| 8 | tag JSON parsing (json_repair vs strict) | D | `generator.py:338,343` | `extractor_tag.go:738,778-790` | introduce JSON repair | P1 |
| 9 | `**ERROR**` handling | — | Python raises, Go skips chunk | Go best-effort | **Keep Go** — add error counting | P3 |
| 10 | tag LLM call failure swallows error | D | `task_executor.py:597-603` | `extractor_tag.go:701-704` | count failures | P2 |
| 11 | question_kwd empty-line filtering | — | Python keeps, Go filters | Go filters | **Keep Go** — more sensible | P3 |
| 12 | `` tag handling (greedy vs non-greedy) | D | `generator.py:233,248,333` (`re.sub` greedy) | `extractor.go:667-676`, `extractor_tag.go:729-737` (`Index` non-greedy) | **Fixed** (2026-07-28) — `strings.Index` → `strings.LastIndex` for `</think>` stripping in `cleanExtractionResult` and `parseTaggerResponse` | P1 |
| 13 | prompt placeholder `{key}` vs append | D | `flow/extractor/extractor.py:102-103` | `extractor.go` | **Fixed** (2026-07-28) — `substituteChunkPlaceholders` replaces `{field_name}` in prompt/system_prompt with current chunk field values (including `{chunks}` → chunkText alias) before `call()`, mirroring Python's `string_format`. Unmatched placeholders left as-is. | P1 |
| 14 | tag concurrency (10 vs 8) | D | `graphrag/utils.py:42` | `extractor_tag.go:36` | read `MAX_CONCURRENT_CHATS` | P2 |
| 15 | keyword/question concurrent vs serial | D | `task_executor.py:444-448` | `extractor.go:550-574` | **Fixed** (2026-07-28) — when both `AutoKeywords` and `AutoQuestions` are enabled, LLM calls run concurrently per chunk (goroutines + WaitGroup), with mutex-guarded map writes to avoid race | P1 |
| 16 | tag topN truncation | — | Python no truncation | Go truncates | **Keep Go** — more sensible | P3 |
| 17 | tag score string→int conversion | D | `generator.py:349-351` (supports) | `extractor_tag.go:748-755` (no) | add `case string` in type switch | P2 |
| 18 | empty llm_id skips tagging | D | `task_executor.py:573-574` (never skips) | `extractor_tag.go:168,173` | **Fixed** (2026-07-28) — `runAutoTags` removes `in.llmID != ""` guards; `resolveExtractorChatTarget` falls back to tenant default chat model via `resolveExtractorChatDefaultConfig` → `resolveTenantModelByType(ctx, db, tenantID, ModelTypeChat)` when llmID is empty | P1 |
| 19 | `prompts` string form not normalized | D | `agent/component/llm.py:119-120` (string → `[{"role":"user","content":prompts}]`) | `extractor.go:172` (only `[]any`; string silently dropped) | **Fixed** (2026-07-24) — `NewExtractorComponent` now accepts a bare-string `prompts` and normalizes it into `Param.Prompt`, mirroring Python. `prompt` (string) still wins on precedence; the `[{role,content}]` list path is unchanged. Latent only — the pipeline decode path feeds the list form via `components[].obj.params`. | P3 |

---

## Tokenizer Component

### Go Omissions

| # | Item | Cat | Impact | Sev | Diff | Effort |
|---|------|-----|--------|-----|------|--------|
| 1 | `finalize_pdf_chunk` post-processing | D | PDF position fields missing | P1 | Mid | M | **Fixed** (2026-07-24) — the coordinate→`position_int`/`page_num_int`/`top_int` conversion is owned by `internal/ingestion/task` `processChunkPositions`→`AddPositions`, which runs in the executor stage *after* the Tokenizer (verified in `chunk_process.go`: it consumes the tokenizer-owned `question_tks`/`content_ltks` fields, then converts positions). The Tokenizer only needs to preserve the raw `positions`/`_pdf_positions` through its output, which `ChunkDoc.ToMap`/`ChunkDocsToMaps` already do; pinned by the pool-free unit test `TestChunkDocsToMaps_PreservesPDFPositions` (`tokenizer_unit_test.go`). No duplicate conversion in the tokenizer (avoids double-handling with 1.4/2.3/2.5). |
| 2 | None chunk filtering | D | null→zero-value ChunkDoc, phantom chunks downstream | P2 | Low | S | **Fixed** (2026-07-27, lines re-verified 2026-07-29) — `isPhantomChunk` (`tokenizer.go:565`, condition at `:566` also requires `Summary == ""`) + filter in `chunksFromTokenizerUpstream` (`tokenizer.go:533`, call at `:553`) discards a chunk only when **all** of Text/Image/ContentWithWeight/**Summary** are empty; a Summary-only chunk is kept and tokenized, matching Python's none-chunk filtering |
| 3 | embedding batch size not configurable | D | low deployment flexibility | P3 | Low | S | **Fixed** (2026-07-27) — `embeddingBatchSize()` reads `TOKENIZER_EMBEDDING_BATCH_SIZE` env var, defaults to 16, matching Python's configurable `settings.EMBEDDING_BATCH_SIZE` |
| 4 | `_ERROR` output mode | — | Python fail-soft vs Go returns error | **Keep Go** — Go returns error (fail-loud); Python's `_ERROR` output channel has no equivalent in the Go pipeline. By design, not a gap | P3 | Mid | M |

### Semantic Differences

| # | Difference | Cat | Python | Go | Fix | Sev |
|---|------------|-----|--------|-----|-----|-----|
| 1 | keywords split rule | D | `task_executor.py:899` (multi-separator, `if k.strip()`) | `split.go` `SplitKeywords` (multi-separator) | **Fixed** — `nonEmpty` filter aligned to `if k.strip()` (drops whitespace-only parts) | — |
| 2 | empty-title embedding behavior | D | `tokenizer.py:95` (always encode) | `tokenizer.go:434-447` (skips) | **Keep Go** — name 恒非空（File 组件保证 doc_id 回退 UUID / file-list 空名报错），skip 更合理；生产不触发 | P3 |
| 3 | title trim | D | `tokenizer.py:95` (no trim) | `tokenizer.go:424` (`TrimSpace`) | **Fixed** — embedding 用原 name (`tokenizer.go:435`)，空判保留 TrimSpace | — |
| 4 | tokenize empty fallback | D | `tokenizer.py:156-160` (empty) | `tokenizer.go:699-714` (falls back to raw) | **Keep Go** (2026-07-27) — both Go and Python fall back to raw text when Tokenize returns empty; functionally equivalent | P3 |
| 5 | summary empty check | D | `tokenizer.py:155` (truthy) | `tokenizer.go:719` (`TrimSpace` check) | **Keep Go** (2026-07-27) — Go uses `strings.TrimSpace(ck.Summary) != ""`: a whitespace-only summary is skipped so the real `Text` is tokenized. Python's truthy check (`if ck.get("summary"":` at `tokenizer.py:155`) treats `"   "` as present and blanks `content_ltks`, shadowing the text — a defect Go intentionally does not inherit | P2 |
| 6 | `str()` conversion | — | Python `str()`, Go strong typing | Go strong typing | **Keep Go** — safer | P3 |
| 7 | `content_with_weight` fallback | D | none | `tokenizer.go:621-630` | **Fixed** (2026-07-27) — `normalizeChunkTextFallback` already exists, populates `Text` from `ContentWithWeight` when empty | P2 |
| 8 | `chunk_order_int` all paths | D | `tokenizer.py:145-193` (chunks path only) | `tokenizer.go:361-362` (all paths) | **Fixed** (2026-07-27, lines re-verified 2026-07-29) — `ChunkOrderInt` assigned via a **nil-guard** (`if chunks[i].ChunkOrderInt == nil`) for all chunks before full_text/embedding branching (`tokenizer.go:345-346`); chunks that already carry an explicit order are left untouched, matching Python's `chunks`-path behavior across all paths | P2 |
| 9 | JSON path doesn't process questions/keywords | — | `tokenizer.py:185-193` (no) | `tokenizer.go:692` (yes) | **Keep Go** — more consistent | P3 |
| 10 | embedding timeout default | D | `tokenizer.py:99` (60s) | `tokenizer.go:118` (600s, comment wrong) | **Intentional removal (2026-07-29)** — per design change the Tokenizer no longer applies an embedding timeout: `defaultTokenizerTimeout` was removed (it had briefly been set to 60s, then deleted); `embedChunks` / `embedder.Encode` (`internal/ingestion/task/embedder.go:41-55`) only forward `ctx`, with no `context.WithTimeout`. If a deadline is required it belongs to the embedding model-driver layer, not the Tokenizer. Not a parity gap under current design. The 2026-07-27 "re-verified Fixed" note for this item was incorrect — the batch-5 merge (`9b0719fa9`) had already deleted the constant. | P1 |
| 11 | language passing | — | Python global singleton | Go per-request instance | **Keep Go** — safer | P3 |
| 12 | Tokenizer-scoped embedding model override | — | `tokenizer.py:62-73` (unsupported) | `tokenizer.go:262,275-284,336` (supported) | **Keep Go** — Go added per-tokenizer embedding model override, not a gap | P3 |
| 13 | stale comment (NumTokens fallback) | — | — | `tokenizer.go:38-48` | **Fixed** (2026-07-27) — stale CJK CAVEAT comment deleted; `internal/tokenizer/tokenizer.go:574-583` `NumTokensFromString` returns `0` on encoder error, matching Python. No behavioral change | P3 |
| 14 | small maxTokens truncation | D | `common/token_utils.py:183-185` (returns first max_len tokens; empty only when max_len<=0) | `tokenizer.go` (`truncateForEmbedding`) truncates to first maxTokens tokens; returns original text when maxTokens<=0 | **Fixed** (2026-07-27, re-verified 2026-07-29) — `truncateForEmbedding` mirrors Python `truncate(string, max_len)`: positive maxTokens keeps the first maxTokens tokens (always non-empty); maxTokens<=0 returns the original text (never "") so an empty embedding entry never reaches the API batch | P1 |
| 15 | underlying tokenizer impl (pure Python vs C++ rewrite) | — | `rag_tokenizer.py` | `binding/cpp/rag_analyzer.cpp` | **Keep Go** (2026-07-27) — both Go (CGo binding) and Python (CPython binding) use the same underlying C++ RAGAnalyzer engine; not a rewrite, functionally equivalent | P3 |

---

## Suggested Fix Order

1. **Wave 3 (LLM robustness)** — Extractor omission 6 (retry) + diff 8 (json_repair).
2. **Wave 4 (data paths)** — diff 2 (tag_kb_ids) + diff 13 ({key} substitution).
3. **Wave 5 (P2/P3 batch)** — omission 7 + diffs 5/6/10/14/17 + omission 8.

## Effort Summary (real gaps only, Cat C/D)

| Component | P0 | P1 | P2 | P3 | Effort |
|-----------|----|----|----|----|--------|
| Parser | 0 | 0 | 0 | 0 | — |
| Chunker | 0 | 0 | 0 | 0 | — |
| Extractor | 0 | 4 | 6 | 1 | ~3-4 days |
| Tokenizer | 0 | 0 | 0 | 0 | — |
| **Total** | **0** | **4** | **6** | **1** | **~3-4 days** |

Counts reflect open items only ("Fixed" / "Not a gap" / "Keep Go" / "Deferred" excluded). Extractor open items: omission 6 + diffs 2/8/13 = P1×4; omission 7 + diffs 5/6/10/14/17 = P2×6; omission 8 = P3×1. Deferred to colleague: diff 1 (P0 tag matching, ES integration). Deferred to compilation layer: omission 2 (metadata), omission 3 (TOC). Keep Go: omission 1 (cache — checkpoint covers this), omission 5 (message_fit_in — truncation belongs at Chunker layer).
