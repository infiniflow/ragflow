# Disposable Python RAGFlow regression

Run from the repository's Python environment with Docker Desktop and installed
local Ollama models `qwen2.5:7b-instruct`, `llama3.1:8b-instruct-q4_K_M`, and
`bge-m3:latest`. The runner validates all three through RAGFlow. Ollama must be
reachable at `host.docker.internal:11434` from containers. It neither downloads
models nor selects cloud models.

```powershell
.venv/Scripts/python.exe -B test/integration/live_ragflow/runner.py `
  --source-root S:/candidate/source `
  --dist-root S:/frontend-work/source/web/dist `
  --frontend-archive S:/artifacts/frontend-candidate.tar.gz `
  --evidence-dir S:/ragflow/output/live-proof
```

Use a frontend build made from the same frozen candidate. The runner requires
and verifies its archive/receipt, matches the mounted dist, records complete hashes and verifies
the candidate before and after execution. Without `candidate.json`, evidence is
explicitly marked as a development run. Container dependencies come from the
recorded existing RAGFlow image; this lane does not certify a rebuilt release image.

Each run uses a random Compose project, new PostgreSQL/MinIO/Elasticsearch
volumes, fresh generated administrator credentials, and loopback ports 19382 and
19383. Ports must be free. Never point these tests at shared application data.
Catalog initialization only accepts an empty disposable database; catalog,
mapping and tracked parser-resource bytes are mounted from the selected source
and checked inside the container. Downloaded parser resources remain external
image dependencies. Passwords and tokens remain in memory/environment; logs are
sanitized. A `finally` block removes all resources belonging to this exact run
and checks that no containers, volumes or networks remain.

Default scenarios perform real browser upload, worker parsing, indexed retrieval,
chat, search, two-model UI behavior, and business-document intake. The shared
chat/search fixture uploads a synthetic text document, requires `DONE` with
nonempty chunks, and verifies deletion. The three-PDF upload scenario generates
one-page synthetic text PDFs, retains settings/save/upload/parse/delete checks,
and uses Plain Text parsing. It does not certify complex PDF layout, OCR, tables,
or the benchmark corpus. Business intake requires completed work, nonempty
clarification questions and 2–4 options per question; it is not a document quality benchmark.

The evidence directory contains command logs, JUnit results, browser artifacts,
source/image/assets/dist identities, positive scenario proofs, and cleanup
results. Failed prerequisites or scenarios are failures; an interrupted run is
not passing evidence. Additional positional arguments select native pytest files
for a focused replay, and its narrower scope must be reported.
