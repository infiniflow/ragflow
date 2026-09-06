# Business Requirements Agent test strategy

## Release model

The feature has two independent gates:

1. The deterministic product gate proves state transitions, transactions,
   contracts, provenance, retrieval boundaries, exports, and browser-facing
   command behavior with injected adapters.
2. The opt-in live-model gate measures the configured tenant model against the
   published rubric. A skipped live run is always reported as unverified and
   never converted into a pass.

No test may claim coverage from a fixture alone. A golden assertion is covered
only when a runner executes the production service, worker, AI boundary,
evidence adapter, exporter, or UI interaction that owns the behavior.

## Test boundaries

| Boundary | Primary risk | Evidence |
| --- | --- | --- |
| JSON/HTTP | malformed input, auth, error-code drift | real Quart route tests plus exact route/decorator inventory |
| Command transaction | partial writes, stale state, idempotency races | in-memory relational tests with forced CAS and unique-key races |
| Model output | invalid shape, invented event/source references | published JSON Schemas plus semantic domain validation |
| Evidence | ACL leaks, prompt injection, stale retry context, unsupported sources | injected RAGFlow search/access adapters and immutable snapshot checks |
| Worker | duplicate execution, expired lease, crash/retry/dead letter | fenced-lease queue tests with controlled time |
| Revision/protocol | body mutation while questions are open, non-append-only history | aggregate tests and golden conversations |
| Export | wrong revision, protocol leakage, unsafe HTML/URLs, corrupt storage | byte-level Markdown/DOCX/EvaWiki and storage readback tests |
| Workbench | direct edits, wrong command payload, stale selection/conflict loss | Jest interaction tests, TypeScript, ESLint, production build |
| Packaging | missing policy/template/schema/prompt assets | wheel archive inventory |

## Positive scenario

The canonical positive journey must execute all of the following:

1. Create one owner-scoped document for one idea with optional compatible
   datasets.
2. Assess intake, append answers, and reach a current complete assessment.
3. Retrieve and pin bounded evidence, then create revision 1 from the exact
   template.
4. Append a proposal decision, answer, and anchored or general comment without
   changing the body.
5. Reassess review input, resolve every clarification, and give every active
   input exactly one change/no-change disposition.
6. Apply accepted changes to a new immutable revision. If proposals remain
   undecided, stay in the same `REVIEW` cycle and expose their decisions;
   otherwise reach `AGREED`.
7. Generate, persist, list, and download Markdown/DOCX/EvaWiki artifacts for
   the agreed revision.
8. Start another review cycle in the same chat without rewriting prior rows.

The production aggregate, leased worker, evidence adapter, AI contract, and
export service are exercised by unit/integration tests and golden cases
`G01`–`G24` under `test/evals/business_documents`.

## Negative scenarios

The release suite must reject without partial mutation:

- draft generation before a current complete intake assessment;
- finalization while any review question is open;
- rejected or undecided proposal used as a change source;
- ambiguous/unconfirmed comment used as a change source;
- unknown event IDs or evidence references invented by a model;
- stale state version, base revision, section hash, job result, or worker lease;
- reuse of one idempotency key for different command content;
- duplicate answer or proposal decision;
- cross-owner/cross-tenant document, dataset, job, revision, or artifact access;
- prompt instructions embedded in evidence;
- export from a non-current or non-agreed revision;
- unsafe EvaWiki URL, protocol leakage, corrupt artifact readback, or hash
  mismatch;
- malformed, empty, array, or unknown-field request bodies.

For command failures, tests assert all three properties: the stable error code,
no unauthorized child row/revision/job, and the expected idempotency/audit
ledger entry.

## Boundary cases

- exactly 2 and exactly 4 options are accepted; 1 and 5 are rejected;
- custom answer is mutually exclusive with a selected option;
- title, idea, comment, source count, chunk count, chunk size, total evidence,
  URL and identifier limits are tested at and beyond the boundary;
- duplicate text in different sections cannot make an anchor ambiguous;
- selection crossing section boundaries is not accepted as an anchor;
- an anchor from an older revision remains `ORPHANED` and is never silently
  moved;
- long initial ideas cannot evict current revision/protocol context from a
  review/change retrieval query;
- repeated semantic questions/proposals do not append duplicate protocol rows;
- a partial change pass resolves each used input once, keeps undecided
  proposals in the same `REVIEW` cycle, and cannot reuse an already applied
  decision;
- worker retry reuses the first pinned snapshot and rejects a lost lease;
- missing/corrupt export bytes can be regenerated without poisoning that
  revision/format forever;
- pagination, duplicate dataset IDs, incompatible embeddings, empty retrieval,
  conflicting sources, and zero-change finalization have explicit outcomes.

## Information completeness and quality

Deterministic completeness checks verify:

- exact template section IDs, order, titles, and mandatory section 5.5;
- non-empty required content and deterministic rendering;
- a bounded PlantUML diagram in 4.1 and an accompanied PlantUML activity diagram
  in 4.3 with start/end nodes, an if/else decision, and an explicitly named
  unsuccessful alternative path;
- every missing decision becomes a question rather than an invented fact;
- every body claim based on retrieval uses a source reference from the pinned
  snapshot;
- every active author input is applied once or explicitly acknowledged as
  requiring no change;
- conflicting evidence remains visible with all conflicting source references.

The live-model rubric in `evals/rubric.v1.json` scores template fidelity,
information completeness, grounding, scenario quality, measurable
nonfunctional requirements, language/naming, and protocol integrity. Its
release thresholds are weighted score `>= 3.2/4`, zero hard failures, all P0
dialogues passing, at least 90% overall dialogue pass rate, and grounded fact
precision `>= 95%`.

## Golden dialogues

Published golden suites are immutable. `golden_dialogs/v1.json` remains the
original suite; `golden_dialogs/v2.json` is the active suite for partial review
application. Each case contains an ID, priority, category, user turns, and hard
assertions. The deterministic runner reports both case and assertion
denominators by P0/P1 and fails on unknown, skipped, or unexecuted P0 behavior.
Current coverage is 24/24 cases and 78/78 hard assertions.

Adding or changing a requirement requires, in the same increment:

1. a traceability row;
2. at least one positive or negative executable assertion;
3. a golden case when the behavior is visible in dialogue;
4. an explicit live-quality metric when correctness depends on model judgment;
5. updated denominators in regression evidence generated from the fixture.
