## MonkeyOCR Layout & Chunking Plan for RAGFlow

### Scope
- Document current chunking and layout parsing flows in RAGFlow
- Identify gaps for MonkeyOCR (currently end-to-end, one-chunk)
- Propose a phased plan to introduce a dedicated MonkeyOCR layout parsing method and phased chunkers
- Align MonkeyOCR's interfaces and data contracts with DeepDoc so that existing chunkers (e.g., `rag/app/naive.py`) can consume MonkeyOCR output without bespoke conversions

### Current Flow (High-level)
- Task dispatch (rag/svr/task_executor.py)
  - Selects chunker via `parser_id` from FACTORY, EXCEPT when `parser_config.layout_recognize == "MonkeyOCR"`, then forces `monkeyocr` module
- Naive parser (rag/app/naive.py)
  - PDF path is multi-phase: OCR/layout/table detection -> merge sections into chunks by `chunk_token_num` and `delimiter`
  - Non-PDF parsers also follow phase -> chunk separation
- Other chunkers
  - `one.py`: one chunk
  - `qa.py`: QA extraction; chunk per Q/A
  - `table.py`: table rows; chunk per row (with field typing)
  - `picture.py`: OCR/VLM for images; chunk per image/description
- Current MonkeyOCR (rag/app/monkey_ocr_parser.py)
  - End-to-end: model load -> `cedd_parse` -> read enhanced markdown -> tokenize -> single chunk
  - No separate "layout phase" API, no fine-grained chunking

### Gaps
- MonkeyOCR does not expose a layout-only method to feed other chunkers
- Forced special-case in `task_executor` prevents mixing MonkeyOCR layout with other chunkers
- Cannot split by pages, headings, or token limits in a reusable manner

### Goals
- Expose a dedicated MonkeyOCR layout adapter matching `naive`’s semantics (sections, tables, optional figures)
- Enable multiple chunking strategies based on MonkeyOCR layout: naive-like, per-page, per-heading, table-centric
- Honor standard parser_config: `chunk_token_num`, `delimiter`, `split_pages`, etc.
- CRITICAL: Allow cross-combination both ways
  - Use MonkeyOCR for layout parsing + any chunking method (naive/one/qa/table/monkeyocr)
  - Use DeepDoc/PlainText/Vision layout + MonkeyOCR chunking method
  - Ensure tag format and return shapes are byte-for-byte compatible with DeepDoc (`@@pn\tleft\tright\ttop\tbottom##`)

### Option A (Minimal change; faster)
- Keep `task_executor` special-case
- Add layout adapter inside `rag/app/monkey_ocr_parser.py`:
  - API: `parse_layout(filename, binary=None, from_page=0, to_page=..., callback=None, parser_config={}) -> (sections, tables, figures|None)`
  - Internals: call `MonkeyOCRParser.parse_document(...)`, read enhanced markdown, segment into `sections` (and `tables`), add simple page tags when possible
- Add chunking variants in `monkey_ocr_parser.py`:
  - `chunk_monkeyocr_naive`: naive_merge + tokenize
  - `chunk_monkeyocr_by_page`: group by page then split by tokens
  - `chunk_monkeyocr_heading`: split by headings then tokens
  - `chunk_monkeyocr_table`: emit tables as chunks (optional)
- Route `chunk(...)` to selected variant using `parser_id`/`parser_config`

Implementation note (supporting `parse_layout`):
- Refactor `monkeyocr/cedd_parse.py` to expose two-phase APIs and optional normalized layout artifact:
  - `load_model(config_path: str) -> MonkeyOCR`
  - `parse_only_to_md(input_file: str, output_dir: str, model: MonkeyOCR, split_pages: bool=False, pred_abandon: bool=False) -> str`
  - `ocr_only_to_enhanced_md(parsed_folder: str, model: MonkeyOCR) -> str`
  - `full_pipeline(input_pdf: str, output_dir: str, config_path: str, split_pages: bool=False, pred_abandon: bool=False) -> str`
- Emit `{stem}_layout.json` (when feasible) with `sections/tables/figures` in DeepDoc contract (tags: `@@pn\tleft\tright\ttop\tbottom##`), colocated with markdown. `parse_layout(...)` may consume this JSON directly for robustness.
- Add CLI flags `--split-pages`, `--pred-abandon` (default False) to avoid behavior change.

Phased emission of layout.json:
- Option A (now): sections-only
  - Build from `..._content_list.json` text items; set `tables=[]`, `figures=null`.
- Option B (later, flag-gated): tables via MonkeyOCR
  - Use `chat_model.batch_inference(..., task="table")` on candidate images to produce HTML tables.
  - Extend `tables` with `((image_rel_path, html), tag)` entries when HTML detected.

Pros:
- Self-contained in MonkeyOCR module; minimal refactor

Cons:
- Still coupled to special-case dispatch; less flexible long-term

### Option B (Recommended; scalable)
- Remove special-case in `task_executor`; always use `parser_id`
- Make chunkers use MonkeyOCR layout when `parser_config.layout_recognize == "MonkeyOCR"`
  - Add `rag/app/layout/monkeyocr_layout.py` with `parse_pdf(...) -> (sections, tables, figures|None)`
  - In `rag/app/naive.py` PDF path, if `layout_recognizer == "MonkeyOCR"`, call the adapter, then proceed with existing merging/tokenization
- This enables any chunker (`naive`, `one`, `qa`, `table`, …) to leverage MonkeyOCR layout

To ALSO enable using the MonkeyOCR chunker with other layout engines (DeepDoc/PlainText):
- Extract a reusable layout selector utility:
  - `rag/app/layout/engine_registry.py`
    - `get_layout_engine(name: str) -> Engine`
    - Engines: `deepdoc`, `plaintext`, `vision:<model>`, `monkeyocr`
  - `Engine.parse_pdf(...) -> (sections, tables, figures|None)` unified contract
- Refactor chunkers (including the MonkeyOCR chunker) to do:
  - `engine = get_layout_engine(parser_config.get("layout_recognize", "DeepDOC"))`
  - `sections, tables, figures = engine.parse_pdf(...)`
  - Then apply their own chunking logic

Pros:
- Orthogonal layout engine vs. chunker; maximum reuse; consistent UX

Cons:
- Touches `task_executor` and `naive.py` (and possibly other chunkers later)

### Proposed Layout Adapter Contract
- Function: `parse_pdf(filename, binary=None, from_page=0, to_page=..., callback=None, parser_config={}) -> (sections, tables, figures|None)`
- Returns:
  - `sections: List[Tuple[str, str]]` where the second element is a tag (page/bbox like `"@@pn\tleft\tright\ttop\tbottom##"` or empty if unknown)
  - `tables: List[((image_or_none, html_string), tag_string)]`
  - `figures: Optional[...]` (same pattern as naive; can be None)
- Honors: `from_page`, `to_page`
- Text-first sections to keep tokenization robust
- Optional argument parity: `separate_tables_figures: bool` supported and honored (when True, figures returned; when False, omitted)

### MonkeyOCR Chunking As A Method (decoupled from layout)
- Provide a dedicated MonkeyOCR chunker module (e.g., `rag/app/monkeyocr_chunk.py`) implementing `chunk(...)`
  - It should NOT perform layout itself; it should use the selected layout engine via the registry
  - Variants:
    - Default: naive-like (token-size + delimiter)
    - By-page: group sections by page tag, then token split
    - Heading-based: split by headings (from markdown), then token split
    - Optional table-centric: emit tables as chunks
- Wire `parser_id: "monkeyocr"` (and optional sub-ids like `monkeyocr_by_page`) to this chunker
- Result: Any layout engine (DeepDoc/PlainText/Vision/MonkeyOCR) can be used with the MonkeyOCR chunker

### Chunking Variants (based on parser_id or flags)
- Default `monkeyocr` (naive-like): `naive_merge(sections, chunk_token_num, delimiter)` -> `tokenize(...)`
- `monkeyocr_by_page`: group by page tags, split by token limit
- `monkeyocr_heading`: split by markdown headings, then token split
- `monkeyocr_table`: emit tables; similar to `table.py` (Phase 2)

### Configuration
- `parser_config` keys:
  - `layout_recognize`: "MonkeyOCR" to use MonkeyOCR engine
  - `chunk_token_num`: int
  - `delimiter`: string or regex string
  - Optional: `split_pages` (bool), `heading_split` (bool), `table_as_chunks` (bool)
  - Optional (future): `heading_levels` (e.g., [1,2,3])
  - Optional (future): `page_range` to restrict layout parse early

### Testing
- Unit tests (layout adapter):
  - Non-empty sections; respects `from_page/to_page`; page tags when possible
- Integration tests:
  - `parser_id = "monkeyocr"` with `layout_recognize: "MonkeyOCR"` yields multiple chunks with varying `chunk_token_num`/`delimiter`
  - `split_pages: true` produces per-page chunks
- Backward compatibility:
  - Existing `monkey_ocr_parser.chunk` remains usable; during migration, delegate to the new default variant
- DeepDoc parity tests:
  - Naive consumes MonkeyOCR `parse_pdf(...)` output with no additional mapping.
  - Tag strings and table tuple structures match those produced by DeepDoc-based parsing.

### Milestones
- Phase 1 (Option A)
  - Implement `parse_layout` in `monkey_ocr_parser.py`
  - Implement default naive-like chunking + by_page variant
  - Wire `chunk(...)` to honor `chunk_token_num`, `delimiter`, `split_pages`
  - Refactor `cedd_parse.py` per two-phase APIs; optionally emit `{stem}_layout.json`
- Phase 2
  - Add heading-based and table-based variants
  - Logging/perf parity with `naive`
  - Enable table inference via MonkeyOCR flag; expand `{stem}_layout.json` with tables when present
- Phase 3 (Option B)
  - Introduce layout engine registry: `rag/app/layout/engine_registry.py`
  - Implement `monkeyocr_layout.py` engine; add adapters for DeepDoc/PlainText/Vision
  - Update all chunkers (naive/one/monkeyocr at minimum) to use the registry
  - Remove special-case from `task_executor` (always route by `parser_id`)
  - Add new MonkeyOCR chunker module decoupled from layout

### Open Questions
- Do we require page/bbox tags in chunk metadata?
- Should table HTML be preserved like `naive.table`?
- Additional chunkers (e.g., OMR/forms-only) desired?
- Naming for MonkeyOCR chunk variants: keep single `parser_id` with flags, or multiple `parser_id`s?

### Not integrating `one.py` (intentional)
- `rag/app/one.py` uses a YOLOv10-based layout path that is inferior to MonkeyOCR’s pipeline for layout recognition. We will not swap it to MonkeyOCR at this time to avoid behavior changes. Optionally revisit after registry is fully adopted across other chunkers.

### References
- Dispatcher: `rag/svr/task_executor.py`
- Naive: `rag/app/naive.py`
- MonkeyOCR (current): `rag/app/monkey_ocr_parser.py`
- Other chunkers: `rag/app/one.py`, `rag/app/qa.py`, `rag/app/table.py`, `rag/app/picture.py`
