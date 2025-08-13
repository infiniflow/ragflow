## MonkeyOCR Integration Steps (component-focused)

- Context: Implement incremental changes to enable MonkeyOCR layout adapter and flexible chunking. Each step targets a single component to keep PRs small and reviewable.
- Objective: Mirror DeepDoc's two-phase pipeline (layout engine -> chunker) so MonkeyOCR can plug into the same interfaces used by `rag/app/naive.py` without bespoke branches.

### Step 1 — `monkeyocr/chunk_adapter.py`: Honor parser_config flags
- Goal: Ensure `chunk_token_num`, `delimiter`, `split_pages` in `parser_config` are parsed and used.
- Tasks:
  - Validate defaults and guard `None` cases.
  - Thread config through the existing `chunk(...)` path without changing current defaults.
- Acceptance:
  - No behavior change by default; with flags set, chunk sizes/splitting reflect config.
  - Defaults align with the naive parser's defaults for `chunk_token_num` and `delimiter` to prevent divergence.

### Step 2 — `monkeyocr/layout_adapter.py`: Add `parse_layout(...)` adapter
- Goal: Expose layout-only API returning `(sections, tables, figures|None)` with a contract identical to the DeepDoc layout used by `rag/app/naive.py`.
- Tasks:
  - Implement `parse_layout(filename, binary=None, from_page=0, to_page=None, callback=None, parser_config=None, separate_tables_figures: bool=False)`.
  - Extract sections/tables from enhanced markdown; include page tags when possible.
  - Match tag format exactly: `"@@{pn}\t{left}\t{right}\t{top}\t{bottom}##"`.
  - Ensure when `separate_tables_figures=True`, returns `(sections, tables, figures)`; otherwise `(sections, tables)`.
- Acceptance:
  - Non-empty `sections` for sample PDFs; respects page range; tags include page index if available.
  - Output shape and tag format are consumable by `rag/app/naive.py` without transformation (DeepDoc parity).

### Step 2.5 — `monkeyocr/cedd_parse.py`: Two-phase APIs + normalized layout output
- Goal: Expose clean two-phase pipeline and an optional normalized layout artifact to accelerate/solidify `parse_layout(...)`.
- Tasks:
  - Add public functions with docstrings:
    - `load_model(config_path: str) -> MonkeyOCR`
    - `parse_only_to_md(input_file: str, output_dir: str, model: MonkeyOCR, split_pages: bool=False, pred_abandon: bool=False) -> str`
    - `ocr_only_to_enhanced_md(parsed_folder: str, model: MonkeyOCR) -> str`
    - `full_pipeline(input_pdf: str, output_dir: str, config_path: str, split_pages: bool=False, pred_abandon: bool=False) -> str`
  - Emit optional normalized JSON next to markdown (when feasible): `{stem}_layout.json` with keys:
    - `sections: List[Tuple[str, str]]`, `tables: List[((image_rel_or_none, html), str)]`, optional `figures`
    - Tags in `@@pn\tleft\tright\ttop\tbottom##` format (use page-only when bbox unknown)
  - Add CLI flags `--split-pages`, `--pred-abandon`; keep defaults False to avoid behavior change.
  - Keep current `cedd_parse(...)` behavior intact; implement wrappers to call internal functions.
- Acceptance:
  - Running `full` mode with no flags behaves exactly as today.
  - When present, `{stem}_layout.json` matches the DeepDoc layout contract and can be directly consumed by `parse_layout(...)`.
  - New functions have docstrings and are imported without side effects.

### Step 2.6 — `monkeyocr/cedd_parse.py`: Emit `{stem}_layout.json` (Option A: sections-only)
- Goal: Produce a minimal DeepDoc-compatible layout artifact from `..._content_list.json`.
- Tasks:
  - Build `sections` from `type=="text"` items: `(text, "@@{page_idx+1}\t0\t0\t0\t0##")`.
  - Set `tables=[]` and `figures=null` by default.
  - Save JSON next to markdown as `{stem}_layout.json`.
- Acceptance:
  - File exists and validates against contract (shape + tag format).
  - No tables/figures present unless later steps enable them.

### Step 2.7 — `monkeyocr/cedd_parse.py`: Optional table inference via MonkeyOCR (Option B)
- Goal: Populate `tables` using MonkeyOCR `chat_model.batch_inference` with `task="table"` when configured.
- Tasks:
  - Add config flag `parser_config.table_infer: bool` (default false).
  - Collect image candidates from `..._content_list.json` (`type=="image"`).
  - Batch infer with instruction: "Please output the table in html format." Only include responses that parse as HTML tables.
  - Emit entries as `((image_rel_path, html_string), "@@{page}\t0\t0\t0\t0##")`.
- Acceptance:
  - With flag off: identical to Step 2.6.
  - With flag on: tables populated when images contain tables; no effect on sections.

### Step 2.8 — `rag/app/monkey_ocr.py`: Prefer `{stem}_layout.json` via layout adapter
- Goal: Use the thin wrapper to call `monkeyocr/layout_adapter.parse_layout(...)`, which prefers `{stem}_layout.json`.
- Tasks:
  - Ensure wrapper delegates to layout adapter; when layout JSON exists, it loads directly; else falls back to markdown parsing.
- Acceptance:
  - When layout JSON exists, `parse_layout(...)` returns its content unchanged; otherwise heuristic parsing applies.

### Step 3 — `monkeyocr/chunk_adapter.py`: Add in-place chunking variants
- Goal: Provide naive-like and by-page chunking using the adapter output.
- Tasks:
  - Implement `chunk_monkeyocr_naive(...)` (default) and `chunk_monkeyocr_by_page(...)` (gated by `split_pages`).
  - Route `chunk(...)` to the proper variant based on `parser_config`.
- Acceptance:
  - With `split_pages: true` → per-page chunks; otherwise naive merge; token limits respected.
  - Tokenization and delimiter handling produce comparable chunk boundaries to naive for the same `sections` input.

### Design Note — Layout-based mapping and chunking (for later integration)
- Goal: Use MonkeyOCR layout as the single source of truth, then map to chunks.
- Mapping plan:
  - Parse first: `parse_layout(..., separate_tables_figures=True)` → `(sections, tables, figures)`
  - Text blocks: keep as `(text, tag)` in `sections`
  - Tables:
    - If HTML present: keep as `((None, html), tag)` in `tables`
    - If only image: optional later — run MonkeyOCR `batch_inference(..., task="table")` to derive HTML
  - Figures/images (non-table): optional later — run MonkeyOCR `batch_inference(..., task="text")` and append OCR text into `sections` as `(ocr_text, tag)`
- Chunking variants:
  - Default (naive-like): `naive_merge(sections, chunk_token_num, delimiter)` → `tokenize_chunks`
  - By-page: group `sections` by page tag → `naive_merge` per page → `tokenize_chunks`
  - Heading-based (later): split on markdown headings in `sections` → `naive_merge` per block → `tokenize_chunks`
  - Table-centric (later): emit each table HTML as its own chunk; text remains separate
- Heuristics (later):
  - Dedup OCR-vs-text on same page if overlap high; merge tiny fragments; never split inside table HTML
  - Respect `split_pages`; preserve order by `(page, top, left)` when available
- Metadata to retain in chunks:
  - `pos_tag` (original tag string), `page`, `bbox` when available, `source_type` (text | image_ocr | table_html), `image_ref` (if any)
- Config flags:
  - `layout_recognize`, `chunk_token_num`, `delimiter`, `split_pages`
  - Optional later: `ocr_images`, `table_infer`, `table_as_chunks`
- Execution flow:
  1) `(sections, tables, figures) = parse_layout(..., separate_tables_figures=True)`
  2) Optionally OCR figures; optionally infer table HTML from images
  3) Apply chosen variant; `tokenize_chunks` and return

### Step 4 — `test/test_monkeyocr_layout.py`: Unit tests for layout adapter
- Goal: Validate `parse_layout(...)` behavior.
- Tasks:
  - Tests: non-empty sections, page range respected, presence of page tags when available.
- Acceptance:
  - Tests pass locally and in CI.

### Step 5 — `test/test_monkeyocr_chunking.py`: Unit tests for chunking variants
- Goal: Validate naive and by-page chunking.
- Tasks:
  - Tests for token limit splitting and per-page grouping.
- Acceptance:
  - Tests pass; no regressions to legacy behavior.

### DeepDoc Parity Checklist (must-haves before registry integration)
- Sections emitted as `List[Tuple[str, str]]` where index 0 is text, index 1 is the line tag string.
- Tag format exactly `@@pn\tleft\tright\ttop\tbottom##` for lines and tables where available.
- Tables emitted as `List[((image_or_none, html_string), tag_string)]`.
- Optional figures emitted when `separate_tables_figures=True`.
- `from_page`/`to_page` and `separate_tables_figures` honored in the adapter.

### Step 6 — `docs` + `zDocs/Change_logs.md`: Document new flags
- Goal: Communicate configuration keys and examples.
- Tasks:
  - Update relevant docs to include `chunk_token_num`, `delimiter`, `split_pages` usage for MonkeyOCR.
  - Add brief changelog entry.
- Acceptance:
  - Docs render; examples are minimal and correct.

### Step 7 — (Removed) Layout engine registry
- Decision: We centralized MonkeyOCR under `monkeyocr/` with a thin wrapper `rag/app/monkey_ocr.py`. No registry needed now.

### Step 9 — `rag/app/naive.py`: Consume MonkeyOCR layout when configured
- Goal: Allow naive chunker to consume MonkeyOCR layout.
- Tasks:
  - Detect config, call `rag/app/monkey_ocr.parse_layout(...)`, proceed with existing merging/tokenization.
- Acceptance:
  - Behavior unchanged by default; works with MonkeyOCR layout when configured.
  - Naive consumes MonkeyOCR engine output with zero code changes beyond the registry call site (proof of parity).

### Step 10 — `rag/app/monkey_ocr.py`: Thin wrapper
- Goal: Provide a stable app-level entrypoint that delegates to `monkeyocr/chunk_adapter.py` and `monkeyocr/layout_adapter.py`.
- Tasks:
  - Implement `chunk(...)` and `parse_layout(...)` wrappers.
  - Support mock mode switching via env `MONKEYOCR_USE_MOCK=1` or `parser_config.use_mock=true`.
- Acceptance:
  - Wrapper routes mock/real correctly; no extra app-level logic remains.

### Step 11 — `rag/svr/task_executor.py`: Dispatch behavior
- Current: If `parser_config.layout_recognize == "MonkeyOCR"`, route to `rag/app/monkey_ocr.chunk(...)`; otherwise route by `parser_id`.
- Later (optional): Remove this special-case once all chunkers consistently consume layout via adapters.

### Step 12 — `rag/app/monkey_ocr_parser.py`: Backward-compat shim
- Goal: Keep legacy entry point functional during migration.
- Tasks:
  - Delegate legacy `chunk(...)` to new `monkeyocr_chunk` default path; log deprecation.
- Acceptance:
  - Legacy integrations function; deprecation message present.

### Step 13 — `test/`: Integration tests for registry + combinations
- Goal: Ensure any chunker can use any layout engine.
- Tasks:
  - Cases: `parser_id="monkeyocr"` with multiple `layout_recognize` values; `parser_id="naive"` with `layout_recognize="MonkeyOCR"`.
- Acceptance:
  - Green tests locally and in CI.

### Step 14 — `docs` + `zDocs/Change_logs.md`: Migration notes and samples
- Goal: Explain orthogonal layout vs chunker selection and provide sample configs.
- Tasks:
  - Add migration guide; update examples accordingly.
- Acceptance:
  - Docs concise; migration path clear.

### Out of scope (for now)
- `rag/app/one.py`: keep existing PDF path (YOLOv10-based layout). Do not switch to MonkeyOCR layout engine to avoid regression; reassess later if needed.


