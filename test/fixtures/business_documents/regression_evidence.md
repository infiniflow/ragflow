# Business requirements agent: regression evidence

Base run date: 2026-08-26; deterministic golden gate refreshed 2026-09-06.
Environment: local Windows checkout `S:\ragflow`,
in-memory SQLite, injected AI/RAGFlow dataset-search/object-storage adapters,
mocked frontend HTTP. No production credentials or customer data were used.

## Verdict

`pass` for the deterministic scripted state-machine gate. All 24 current golden cases
and all 78 hard assertions execute through `BusinessDocumentService`, the
leased worker, injected `BusinessDocumentAI`, pinned Evidence, or the concrete
export service. This verdict is not a live-model quality pass.

## Executed lanes

| Lane | Command/evidence | Result |
| --- | --- | --- |
| Deterministic scripted state-machine gate | `uv run pytest -q test/evals/business_documents/test_golden_dialogue_harness.py` | P0 19/19 and 64/64 assertions; P1 5/5 and 14/14 assertions; all 24/24 and 78/78 |
| Live-quality scorer unit/config | `uv run pytest -q test/evals/business_documents/test_live_quality_scorer.py` | 3 passed; weighted rubric, controlled-reference precision, canonical PlantUML/BPMN fixture shape, hard-failure subset, honest config gating |
| Real-model intake-to-draft quality | `uv run pytest -q test/evals/business_documents/test_live_model_quality.py` | not executed; default behavior verified as skipped, and flag `1` without tenant verified as an explicit failure |
| Domain/worker/evidence/export/API/assets | Focused pytest over `test/unit_test/api/apps/business_documents`, REST contract, assets and all business-document evals | 72 passed, 2 skipped; both skips are opt-in live-model lanes and are not counted as passes |
| Frontend Workbench and client | Focused Jest for Workbench/client | 24 passed (22 Workbench UI + 2 service client) |
| Frontend production build | `npm run build` | passed; 12,095 modules transformed |
| Static checks | Focused Ruff, ESLint and TypeScript gates | passed |
| Wheel package data | `uv build --wheel` plus archive inventory | passed; representative policy/template/schema/prompt/golden assets present without package-data ambiguity |

## Coverage state

- API: `covered` for nine authenticated routes: create/list/get, commands,
  revisions list/get, jobs, export list and export download.
- Worker: `covered` for lease fencing, stale recovery, retry/backoff,
  dead-letter, restart after failure, singleton start and wake.
- Evidence: `covered` deterministically for dataset ACL, bounded retrieval,
  immutable snapshot/hash/source refs, prompt-injection-as-data and conflicting
  sources. G20/G21 use the real evidence component and captured AI input.
- Export: `covered` for agreed revision gating, durable write/readback,
  idempotency, list/download ownership, content hash, Markdown, DOCX ZIP and
  exact EvaWiki rendering with safe URLs and protocol exclusion.
- UI: `covered_by_lower_level` for create/resume, read-only body,
  allowed-command gating, loading/empty/error/conflict/busy states and artifact
  links. A running-stack Playwright journey is outside this deterministic gate.
- Golden requirements: P0 `19/19` and `64/64` assertions; P1 `5/5` and
  `14/14` assertions; all cases `24/24` and `78/78` assertions.
- Live quality: a separate opt-in test now runs the real tenant chat model
  through at least intake and draft, validates the exact published template,
  protocol separation, mandatory monitoring and question bounds, then computes
  the weighted `rubric.v1` score and controlled-fact citation precision from a
  pinned Evidence snapshot. It was not run in this snapshot.

## Stop points and residual risk

- Live LLM quality requires both `BUSINESS_DOCUMENT_LIVE_LLM=1` and
  `BUSINESS_DOCUMENT_LIVE_TENANT_ID=<tenant>`. It skips by default, fails
  explicitly when enabled without a tenant, and is never counted as passed
  unless the real-model test actually completes.
- The live intake-to-draft scorer covers only draft-local hard failures
  (`UNSUPPORTED_SECTION_INVENTED`, `REQUIRED_MONITORING_MISSING`,
  `EVIDENCE_INSTRUCTION_EXECUTED`). Lifecycle hard failures continue to be
  covered by deterministic state-machine tests; this one representative run
  does not claim the rubric's P0/all-case live-suite rates.
- No authenticated live HTTP/browser stack was provisioned in this lane.
- The golden fixture currently contains 19 P0 cases, not 17; counts are derived
  from the fixture at runtime to prevent stale release denominators.
