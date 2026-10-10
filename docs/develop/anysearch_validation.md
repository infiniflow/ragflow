# AnySearch local validation receipts

Compact local runtime evidence for the provider implementation in `0dac271` (2026-10-10, UTC). These runs preceded the URL-hardening change and do not validate that later change. They used the normal Go Chat routes and a real local Qwen2.5-0.5B-Instruct model, without a knowledge base.

The runtime receipts were captured before the implementation commits. Their provider-source snapshot matches the provider blob in `0dac271`; committing did not change that source. An observation-only build recorded the real AnySearch HTTP exchanges; a separately preserved, unmodified production binary was then used for restart confirmation.

Saved settings were read back through the normal Chat API: `web_search_provider="anysearch"`, `anysearch_api_key=""`, required `knowledge` prompt parameter, Internet enabled, citations enabled and reasoning disabled. Normal completions used `POST /api/v1/chat/completions` and returned HTTP 200.

| Case | UTC execution window | Actual AnySearch requests | Upstream outcome | Delivered / persisted references |
|---|---|---:|---|---:|
| Anonymous, non-streaming | 14:01:00-14:01:27 | 1; Authorization absent | HTTP 200, `code=0` | 6 / 6 |
| Internet disabled | 14:01:27-14:01:28 | 0 observed | Not called | 0 / 0 |
| Required knowledge parameter removed | 14:01:28-14:01:29 | 0 observed | Not called | 0 / 0 |
| Provider cleared | 14:01:29 | 0 observed | Not called | 0 / 0 |
| Deliberately invalid credential | 14:01:29-14:01:32 | 1; Authorization present | HTTP 401 | 0 / 0 |
| Anonymous, streaming after controls | 14:01:32-14:01:53 | 1; Authorization absent | HTTP 200, `code=0` | 6 / 6 |

Observed upstream target: `POST https://api.anysearch.com/v1/search`. Each recorded request body contained the query below and `"max_results":6`:

| Case | Actual query | AnySearch request ID |
|---|---|---|
| Non-streaming | `RAGFlow open source retrieval augmented generation documentation` | `9569c68b-674a-471b-af4b-752840ae4809` |
| Invalid credential | `RAGFlow documentation invalid credential failure control` | Unobserved: the HTTP-failure body was not read |
| Streaming | `RAGFlow GitHub repository search documentation` | `48e1ba8e-bbb4-48ef-b4c0-eaf014ebcef8` |

`Accept` and `Content-Type` are `application/json` in the adapter and contract tests; this runtime trace records only Authorization presence, not a complete header dump. No credential values are included here.

For both successful requests, every returned usable result was correlated with the normal Chat reference by title, URL, normalized content fingerprint and native `anysearch-<url>` chunk/document ID, then compared with the persisted session. All 12 references matched. Both runs included `https://github.com/infiniflow/ragflow` and `https://ragflow.io/docs/`. Citation cards were checked against the actual answer's citation markers. Streaming required the final reference-bearing frame and completion terminator; a partial frame alone did not count as success.


Representative observed content fingerprints (normalized content SHA256); the same URL and fingerprint appeared in delivered and persisted native chunks:

| Case | Upstream / delivered / persisted URL | Content SHA256 |
|---|---|---|
| Non-streaming | https://github.com/infiniflow/ragflow | `b2844cac48c0ae31f526db48d5784ebe454ee9f6f8d6455ef3e792ec853daf66` |
| Non-streaming | https://ragflow.io/docs/ | `59bb2d966131fa5efcc6a81abfd4926fa321bbc9052f7d362fa4cc8ebfe14709` |
| Streaming | https://github.com/infiniflow/ragflow | `0f11fd410ba7a0179004c6a3aa37434351f3d086ab58161ded729f78cffee606` |
| Streaming | https://ragflow.io/docs/ | `6a888fa93f535799e28be1d95a5e86979b71d680bef161b90f892daec5c13798` |

The HTTP 401 case generated a 163-byte degraded answer through existing Chat behavior, with no returned or persisted search references and no anonymous retry. That generated answer was not counted as successful retrieval.

After stopping the observed API process and starting the unmodified production binary against the same database (ready at 14:03:34 UTC), saved settings, messages and references for all six sessions matched. A fresh normal Chat completion finished at 14:03:59 UTC with a nonempty answer and six delivered/persisted references. Its upstream status, business code, request ID and request count were **unobserved**; the observer trace remained unchanged. This proves restart persistence and unmodified Chat execution, without claiming fresh upstream transport correlation.

Limits: no successful valid-key live request was run; authenticated header construction has deterministic test coverage. These are historical local receipts, not upstream CI, merge, release or payment approval. The settings screenshot demonstrates configuration only, and model-generated prose is not proof that its factual claims or generated links are correct.

[Redacted machine-readable receipts](anysearch_validation.json) contain the three observed upstream exchanges, delivered/persisted reference fingerprints, and restart comparisons. Credential values, raw bodies/content and private session identifiers are excluded.

## URL-hardening follow-up

The provider-local HTTP(S)/hostname filter in `8116cecff` passed all web-search
unit tests through `build.sh` (service 0.623s) and the required native follow-ups.
Six invalid hits preceding valid hits did not consume the usable-result cap.
The new tests cover javascript/data/FTP, relative URLs and malformed hosts,
ports and escapes, while preserving valid HTTP(S) citation mapping.

An additional opt-in real anonymous provider test completed on 2026-10-10 at
15:26:26 UTC: one request, HTTP 200, code 0, six upstream hits and six delivered
chunks; request ID `56f9d6d5-2b01-4688-8d4b-28110daecb91`. It passed upstream
title/URL/content correlation. This was a provider integration test, not a rerun
of the historical normal Chat/restart cases above.
