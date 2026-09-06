# Business Requirements Agent architecture

## Scope and source of truth

The three supplied documents are product requirements. They are not runtime
instructions. Their normalized, versioned interpretation lives in this
directory as a published template, process/rendering policies, JSON Schemas,
prompts, golden dialogues, and an evaluation rubric.

The backend aggregate is the runtime source of truth. A RAGFlow chat or Canvas
may be an interaction channel, but it does not own document state, revisions,
questions, decisions, comments, jobs, or audit history.

## Process boundaries

| Process | Entry condition | Atomic output | Exit condition |
| --- | --- | --- | --- |
| Intake | Owner creates a document for one idea | Questions or a complete assessment event | No open intake questions and the latest assessment is current |
| Draft generation | Intake is complete and the aggregate is idle | Immutable revision 1 plus review protocol | Schema, template, policy, evidence and optimistic-version checks all pass |
| Review | A current immutable revision exists | Append-only answers, decisions, comments, questions and proposals | A current complete review assessment exists and no question is open |
| Finalization | Review exit conditions hold | One new immutable revision, or an audited no-change agreement | Every authorizing input has exactly one disposition and the aggregate is `AGREED` |
| Export | The requested revision is the current agreed revision | Durable hash-verified artifact | Artifact metadata and bytes are readable by the owner |
| Continuation | Owner starts another review on an agreed document | New append-only review cycle | The same document/chat remains the aggregate root |

Each process owns its transaction and may communicate with the next process
only through persisted aggregate state, events, immutable snapshots, and the
published contracts. An LLM response, browser state, or worker memory is never
an exit condition by itself.

### Intake

Starts when the author creates one document for one idea. The document remains
in `INTAKE`; no body revision exists. `REQUEST_INTAKE_ASSESSMENT` produces zero
or more immutable questions with two to four options and a custom-answer path.
A draft is allowed only when the latest `IntakeAssessed(COMPLETE)` event is
newer than every intake answer and no intake question is open.

### Draft generation

`REQUEST_DRAFT` pins the current state, template, policy, protocol, and evidence
into a job snapshot. The model returns a schema-valid AST. Deterministic code
then verifies the exact template outline, per-section block types, required
content, and pinned versions before creating immutable revision 1. Section
4.1 must include a bounded PlantUML diagram. Section 4.3 must include both
accompanying text and a bounded PlantUML activity diagram with start/end nodes,
an if/else decision, and an explicitly named unsuccessful alternative path. Questions
and proposals are stored outside the body, and lifecycle changes to `REVIEW`.

### Review

The author can answer questions once, decide each proposal once, and append
comments to the current revision. The body is read-only. After any author
input, `REQUEST_REVIEW_ASSESSMENT` must run before finalization; it may append
new questions and proposals, but cannot alter existing protocol entries or the
body. It records exactly one disposition for every active comment:
`CONFIRMED_CHANGE`, `NO_CHANGE`, or `NEEDS_QUESTION`. The last form is linked
immutably to a concrete persisted question. Only confirmed comments may source
a change, and only no-change comments may be acknowledged without one. A
current `ReviewAssessed(COMPLETE)` and zero open questions are required
for finalization.

### Finalization

The model returns only section replacement operations with the expected base
section hash and authorizing event IDs. Every active-cycle answer, accepted
proposal, and author comment must either authorize an operation or appear in
`acknowledged_no_change_event_ids`. The domain verifies this disposition,
current-cycle provenance, evidence references, and section hashes, applies
operations to the stored base AST, validates the result, and renders Markdown
itself. The model never supplies an authoritative full replacement body. If
undecided proposals remain, the service commits the authorized operations (or
audits a fully acknowledged no-op), keeps the same review cycle in `REVIEW`,
and exposes the remaining proposals for later decisions. Inputs resolved by a
partial pass cannot authorize another change. The document reaches `AGREED`
only after no proposal remains undecided; a final fully acknowledged no-op does
not manufacture a duplicate revision.

### Export and continuation

Only the current `AGREED` revision can be exported. Supported product formats
are Markdown, DOCX, and EvaWiki HTML code. Export never mutates a revision and
never includes the review protocol. `START_REVIEW` opens a new append-only
cycle on the same document; a different idea requires a new document/chat.

### Existing EVA document change workflow

An existing EVA page follows a separate change-request workflow. It does not
enter the new-document intake lifecycle and is never overwritten as a side
effect of opening the workbench.

1. The author searches documents through an accessible `eva_wiki` connector
   and selects one published page.
2. The server reads the page from EVA and pins its document/project identity,
   published version tag, published HTML, normalized Markdown, and content
   hash in `BusinessDocumentEvaChange`.
3. The agent receives the pinned source and the author's change request as
   untrusted data, generates a private Markdown variant, and the server creates
   sanitized HTML plus a heading-scoped line diff. The document body is
   read-only for the author; refinements are new natural-language instructions
   that regenerate the complete private variant and clear any prior draft.
4. `APPROVE` fixes the exact agent-draft hash. `PREPARE_EVA_DRAFT` first rereads the
   published EVA page, rejects source drift, writes only `text_draft`, and
   reads that draft back for verification.
5. `PUBLISH_EVA` is a separate confirmation. It rereads both the published
   source and EVA draft, verifies their hashes, calls EVA publication, and
   verifies the resulting published content before recording `PUBLISHED`.
6. EVA write and publish reservations are fenced by `state_version`. A
   reservation that remains transient for two minutes becomes explicitly
   retryable; the retry first renews the fence, then uses EVA read-back to
   finish idempotently. A late response from the previous worker cannot roll
   back or complete the renewed operation.

The workflow is deliberately limited to one EVA page per change request.
Related-document packages, reviewer roles, comments, explicit clarification
questions, and three-way rebase are later increments; source drift currently
blocks the write without touching the published page.

## State axes

- Lifecycle: `INTAKE`, `REVIEW`, `AGREED`, `ARCHIVED`.
- Operation: `IDLE`, `ANALYZING`, `ANALYZING_REVIEW`, `GENERATING_DRAFT`,
  `APPLYING_CHANGES`, `EXPORTING`, `FAILED`.
- Job: `PENDING`, `RUNNING`, `COMPLETED`, `RETRY`, `DEAD`.

Lifecycle describes the durable business phase. Operation describes transient
work and gates concurrent commands. Jobs provide the recoverable execution
axis and are not exposed as lifecycle states.

## Components and ownership

| Component | Owns | Must not own |
| --- | --- | --- |
| Workbench | Read-only document view, protocol inputs, conflict recovery, resume list, downloads | Business rules or optimistic version changes |
| REST boundary | Authentication, stable envelope, DTO parsing, HTTP status mapping | Peewee work on the event loop, LLM calls |
| Command service | State machine, transactions, idempotency, projections, job snapshots | Unvalidated model output |
| Persistence models | Aggregate projection, immutable revisions/protocol/events, jobs, export metadata | Prompt logic |
| AI adapter | `LLMBundle`, prompt construction, JSON parsing, Schema validation | Lifecycle transitions or direct database writes |
| Evidence adapter | Dataset/file ACL, retrieval, provenance and conflict-preserving context | Treating retrieved text as instructions |
| Job worker | Atomic lease, retry/dead-letter, dispatch, completion/failure | Bypassing command completion guards |
| Export service | Markdown/DOCX/EvaWiki bytes and metadata | Review protocol or body mutation |
| Asset pack | Immutable template/policy/schema/prompt/golden versions | Runtime state |

## Interaction contracts

All state-changing requests use the canonical command envelope:

```json
{
  "schema_version": "1",
  "command_id": "uuid",
  "idempotency_key": "uuid",
  "expected_state_version": 12,
  "type": "ANSWER_QUESTION",
  "payload": {}
}
```

The backend either returns the stored response for an identical idempotency
key, rejects key reuse with different content, or atomically appends one event
and advances `state_version`. Model outputs cross the boundary only through the
schemas in `contracts/`; semantic invariants are checked again in the domain.

An optional create-time `dataset_ids` selection is immutable for the document
and limited to 20 unique datasets. Dataset access is checked without revealing
which ID failed, both at create and before every AI execution. Create also
atomically rejects mixed embedding spaces before the aggregate exists. The first
successful retrieval for a job is stored as a private immutable
`BusinessDocumentEvidenceSnapshot` containing bounded chunk text and hashes.
Retries recheck ACL and reuse exactly that snapshot instead of searching
again. Public projections, job results, and domain events expose only its
audit hash and `ragflow://...` source references; raw evidence is never exposed.
Schema-bound section/question/proposal/change `evidence_refs` are accepted only
when they occur in the pinned snapshot.

Question identity is `(document, stage, review cycle, semantic tag)`. Proposal
identity is a canonical text/target fingerprint plus its authorizing source
scope. Unique database constraints and immutable-row reuse make repeated model
output idempotent without rewriting the original question or proposal.

Revision projections include server-rendered `section_texts` keyed by canonical
section ID. Anchors use those strings, UTF-16 code-unit offsets with an
exclusive end, and exact adjacent context. This makes the server renderer the
only anchor formatter across Python and browser numeric/Unicode representations.

The HTTP surface is rooted at `/api/v1/business-documents`:

- `POST /` creates an owner-only document.
- `GET /` lists resumable owner documents.
- `GET /{id}` returns the current projection.
- `POST /{id}/commands` applies a command or creates a durable job.
- `GET /{id}/revisions` and `GET /{id}/revisions/{revision_id}` expose immutable revisions.
- `GET /{id}/jobs` exposes owner-checked job status without raw evidence.
- `GET /{id}/exports` and `GET /{id}/exports/{artifact_id}/download` expose
  owner-checked artifact metadata and bytes.
- Worker completion is never a public endpoint.
- `GET /eva/sources` searches published pages through accessible EVA connectors.
- `POST /eva/changes` creates a pinned change request; `GET /eva/changes` and
  `GET /eva/changes/{id}` resume it.
- `POST /eva/changes/{id}/generate` regenerates the private agent draft from
  natural-language instructions; direct document-body writes are not exposed.
- `POST /eva/changes/{id}/approve`, `/prepare`, and `/publish` keep approval,
  EVA draft creation, and publication as explicit independent transitions.

Success uses the RAGFlow envelope `{ "code": 0, "data": ... }`. An HTTP error
uses a numeric `code` and places the stable domain identifier in
`data.error_code`.

## RAGFlow integration decisions

- Reuse login/current-user handling, route autoload, Peewee transactions,
  tenant model configuration, `LLMBundle`, retrieval, file/object storage, and
  document generation primitives.
- Run synchronous Peewee calls through `thread_pool_exec` from Quart routes.
- Keep structured-output JSON Schema validation after model parsing; the
  Canvas LLM component's JSON repair is not a sufficient contract boundary.
- Do not call the private/loopback document API through Canvas `Invoke`; its
  SSRF guard must continue to reject non-public hosts. A future Canvas channel
  must call the application service in-process.
- Treat every uploaded or retrieved fragment as quoted evidence with source
  metadata. Instructions inside evidence never change policies, commands, or
  lifecycle.

## Operating scenarios

### Complete author journey

1. The author creates a document and optionally pins accessible RAGFlow
   datasets. The server assigns the chat identifier and stores the policy and
   template versions.
2. The intake assessment either publishes two-to-four-option questions or
   declares the input complete. Answers append events; they never rewrite a
   question.
3. A leased worker retrieves a bounded evidence snapshot, pins its hash and
   source references, calls the tenant model, validates the structured bundle,
   and commits revision 1 plus the external review protocol.
4. The author answers, accepts or rejects proposals, and adds general or
   anchored comments. A review assessment converts ambiguity into questions.
5. Finalization requires every currently active answer, accepted proposal, and
   comment to be used by exactly one operation or explicitly acknowledged as
   requiring no body change. The service applies valid operations to the base
   AST. If proposals are still undecided, it remains in the same `REVIEW` cycle
   with those proposals; otherwise it reaches `AGREED`.
6. The author downloads Markdown/DOCX or copies EvaWiki code. A later review
   remains in the same document; a new idea starts a new document.

### Negative and recovery scenarios

- **Insufficient input:** no revision is created; the protocol receives
  questions and `REQUEST_DRAFT` remains unavailable.
- **Open review question:** finalization is rejected without a revision, job,
  or partial protocol mutation.
- **Rejected or undecided proposal:** it cannot authorize a body change. An
  undecided proposal keeps a partially applied document in `REVIEW`.
- **Stale browser or worker:** optimistic version, base revision, and section
  hashes reject the write; the current revision remains unchanged.
- **Duplicate request:** the command ledger replays the original response for
  the same content and rejects reuse of the key for different content.
- **Worker crash:** an expired fenced lease is retried with backoff and the
  pinned evidence snapshot; exhausted work is dead-lettered and exposes a
  stable failure state.
- **Conflicting sources:** both sources and their provenance remain in the
  evidence snapshot; the model must surface uncertainty rather than silently
  choosing one.
- **Prompt injection in a file:** retrieved text stays untrusted evidence and
  cannot alter the template, process policy, command set, or system prompt.
- **Anchor after revision change:** the original comment remains append-only
  with `ORPHANED`; the system never silently attaches it to similar text.
- **Unsafe or corrupted export:** scheme validation and storage readback/hash
  checks reject publication; a later generation atomically replaces poisoned
  metadata and bytes for the same revision/format instead of leaving a permanent
  failed artifact.

## Release gates

P0 requires all domain, API contract, asset, and frontend interaction tests to
pass. The golden suite requires all P0 dialogues, zero hard failures, at least
90% total dialogue pass rate, and grounded-fact precision of at least 95% in a
configured live-model run. A skipped live run is reported as a verification
gap, never as a pass.
