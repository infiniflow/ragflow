## MonkeyOCR DB & Chunk Equivalence Test Plan

Purpose: Ensure that MonkeyOCR chunking returns the exact same format and, where applicable, the same chunk content/ordering as existing chunking methods via SDK and HTTP APIs. DB-visible results (via list/retrieval APIs) must match baseline behaviors.

Scope
- SDK API: `ragflow_sdk` dataset/document/chunk APIs must return the same schema and chunk list semantics when `parser_engine="monkeyocr"` as with baseline engines.
- HTTP API: `list_chunks`, `retrieval_chunks` responses must be schema-identical and behaviorally consistent for MonkeyOCR vs baseline.
- DB perspective: Observed data via APIs (chunk list, retrieval results) must be equivalent for shared scenarios.

Baseline and Variants
- Baseline A: DeepDoc + `chunk_method="naive"` (default config) — treated as reference.
- Variant 1: MonkeyOCR + `chunk_method="monkeyocr"`, configured to mimic baseline behavior (`chunk_token_num`, `delimiter`, and `split_pages=False`).
- Variant 2 (page-split): MonkeyOCR + `split_pages=True` vs a page-based baseline if available (optional; format-only checks apply).

Documents Under Test (DUT)
- Plain text: `sdk/python/test/test_sdk_api/test_data/ragflow_test.txt` (content equivalence expected).
- Simple PDF: `sdk/python/test/test_sdk_api/test_data/test.pdf` (format equivalence required; content equivalence targeted if layout is consistent).
- Simple DOCX: `sdk/python/test/test_sdk_api/test_data/test.docx` (content equivalence targeted).

Common Normalization (for strict comparisons)
- Trim trailing whitespace per chunk; normalize `\r\n` → `\n`.
- Ignore non-deterministic fields (IDs, timestamps).
- When comparing retrieval results, compare ordered `content` arrays and key fields, not IDs.

Test Matrix (high level)
- SDK: create two datasets (baseline vs monkeyocr), upload same file, parse, list chunks, compare.
- HTTP: create two datasets via HTTP, upload, parse, `list_chunks` compare; run `retrieval_chunks` compare.

Planned Test Files
- SDK tests: `sdk/python/test/test_sdk_api/t_monkeyocr_equivalence.py`
  - Will use existing utilities in `sdk/python/test/test_sdk_api/common.py` and `ragflow_sdk`.
- HTTP tests: `sdk/python/test/test_http_api/test_monkeyocr_equivalence.py`
  - Will reuse fixtures and helpers from `sdk/python/test/test_http_api/common.py` and related conftests.

SDK-Level Tests
1) Schema format parity (all types)
   - Create dataset A: baseline (DeepDoc + naive).
   - Create dataset B: MonkeyOCR + monkeyocr.
   - Upload same DUT file to both datasets, parse, then for each `Document.list_chunks()`:
     - Assert returned object list has the same keys as baseline chunks: `id, dataset_id, document_id, content, available, important_keywords, questions, token_count` (plus any existing optional fields that baseline returns).
     - Assert ordering is deterministic and consistent (by create time or index), matching baseline.

2) Content equivalence — Plain text
   - DUT: `ragflow_test.txt`.
   - Same `parser_config` on both datasets: `chunk_token_num`, `delimiter`, `split_pages=False`.
   - After parse, compare chunk counts and per-index `content` strings (after normalization) — expect exact match.

3) Content equivalence — DOCX
   - DUT: `test.docx`.
   - Same config as above; compare counts and contents (normalized). Expect match.

4) Content parity target — PDF
   - DUT: `test.pdf`.
   - Compare counts and contents (normalized). If layout introduces minor diffs, at minimum assert:
     - Same schema keys.
     - Content similarity above threshold (optional, if needed). Primary goal remains exact match; document any exceptions.

5) Retrieval equivalence
   - On both datasets (text DOC), call `rag.retrieve(dataset_ids=[...], document_ids=[...], question="chunk")`.
   - Compare the returned chunk list size and per-index `content` (normalized) and presence/absence of `highlight` per baseline behavior.

HTTP-Level Tests
6) list_chunks parity (reuses existing patterns)
   - Create baseline and MonkeyOCR datasets via HTTP.
   - Upload same file, parse, then call `list_chunks`.
   - Compare `data.chunks` length, and for each index compare `content` (normalized) and fields presence.

7) retrieval_chunks parity
   - Query with `question="chunk"`, compare chunk list length, and ordered `content` equality (normalized). Validate `highlight` field parity if baseline includes it.

Edge and Negative Cases
8) Empty delimiter and custom delimiter
   - Set `delimiter="\n"` and then a custom delimiter (e.g., "\n---\n"). Ensure both engines split identically on TXT and DOCX.

9) Token size boundaries
   - Use `chunk_token_num` small and large values to confirm split behavior parity on TXT.

10) Page split variant (format-focused)
   - With `split_pages=True`, ensure schema and ordering parity; content equality may vary; record expected deviations.

Acceptance Criteria
- Format: MonkeyOCR responses include the same fields as baseline across SDK and HTTP APIs.
- TXT/DOCX: exact equality in chunk counts and per-index content after normalization.
- PDF: best-effort content equality; any deviations documented and gated behind explicit test markers if needed.
- Retrieval: same number of returned chunks and identical ordered contents (normalized) for TXT; consistent `highlight` behavior.

Implementation Notes
- Reuse fixtures: `get_http_api_auth`, `add_dataset_func`, `add_document`, `add_chunks` where applicable.
- Introduce local helpers to normalize chunk lists before comparing.
- Use pytest markers to skip PDF strict-content checks on engines/environments where layout variability is known (mirror existing skip patterns for Infinity/Elasticsearch when necessary).

Naming & Structure (proposed)
- SDK: `t_monkeyocr_equivalence.py`
  - `test_schema_format_parity()`
  - `test_txt_content_equivalence()`
  - `test_docx_content_equivalence()`
  - `test_pdf_content_parity_target()`
  - `test_retrieval_equivalence_txt()`

- HTTP: `test_monkeyocr_equivalence.py`
  - `test_list_chunks_parity_txt()`
  - `test_list_chunks_parity_docx()`
  - `test_list_chunks_parity_pdf()`
  - `test_retrieval_parity_txt()`

Data Contract Checklist (both SDK/HTTP)
- Chunk list ordering consistent with baseline.
- Fields present: `id`, `dataset_id`, `document_id`, `content`, `available`, `important_keywords`, `questions`, `token_count` (or existing baseline names).
- No extra MonkeyOCR-only fields leaking unless baseline has analogues.

Exit Criteria
- All planned tests pass locally and in CI.
- No regressions in existing chunk tests.
- Document any intentional differences (PDF layout) and ensure they’re either resolved or isolated behind explicit markers.


