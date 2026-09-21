# Embedding input token limits

How the ingest path decides how much text to hand an embedding model, and what it
does when the model disagrees. This is an implementation document: everything
described here is in the tree and verified, and every number quoted was measured.

## Summary

- **Invariant.** Text handed to an embedding API must be counted with the model's own
  tokenizer, or with a calibrated upper bound of it; cl100k alone is never enough. And:
  **one over-limit rejection must never fail a whole document.**
- **Layering.** L1 exact counters (registry + four family implementations) → L2 calibrated
  fallback (learn the real/cl100k ratio from the provider's `usage`) → L3 shrink, then
  isolate and retry, when a request is rejected.
- **Why it cannot be avoided.** Embedding providers *refuse* over-length input instead of
  truncating it (bge-m3 rejects an input 0.55% over spec with `400`/`20015`, and one bad item
  rejects the whole batch), the only token count they report comes back with a *successful*
  response, and the tokenizer architectures disagree so much that cl100k under-counts bge-m3 by
  up to 33% on our samples and the spread between estimators reaches 11x on ordinary text.
  See [§Why this module has to exist](#why-this-module-has-to-exist).
- **How it is verified.** `scripts/gen_tokenizer_oracle.py` generates fixtures and
  `TestCountersMatchOracle` compares every sample with the model's own tokenizer
  (**4 families, the same 33 base samples plus `--fuzz`, all equal**); a second tier holds *every* counter to the same
  contract (prefix, UTF-8 integrity, `Count(Trim) <= limit`, idempotence, monotonicity).
- **Normative basis.** Each counter is anchored to the model's own tokenizer file (SHA-1
  pinned, verified at load) and to the algorithm that file declares; **a family with no
  oracle fixture is never tagged** — it falls back to L2 instead. See
  [§How the implementation is kept honest](#how-the-implementation-is-kept-honest).
- **Result.** `78785.md`, which used to fail with `400`/`20015`, now ingests normally
  (36 chunks, MySQL == ES) and **without touching L2/L3** — it takes the exact-count path.
- **Two authorities.** The `tokenizer` field in the model catalog `conf/all_models.json`
  decides *which* counter is used; the model's own tokenizer file decides *how it counts*
  (not the declarations inside that file's config).

## Why this module has to exist

Three measured properties of embedding services make "just send it and let the model deal with
it" impossible. All the numbers below are measured against the provider this deployment uses
(SILICONFLOW, `BAAI/bge-m3`, declared `max_tokens = 8192`). Together they are the reason for an
explicit pre-flight trim, per-model counters, and a calibrated fallback rather than one helper
function.

### 1. Over-length input is refused, never truncated, so the caller must trim

| Request | Result |
|---|---|
| 23,000 chars = **8,176** tokens (inside spec) | `200 OK`, `usage.prompt_tokens = 8176` |
| the incident span, 25,000 chars = **8,237** tokens (**0.55% over spec**) | `400`, `{"code":20015,"message":"The parameter is invalid. Please check again."}` |
| batch `["hello", <the incident span>]` | `400` — the **whole batch**, not just the bad item |
| batch `["hello","world","ok"]` | `200 OK`, `usage.prompt_tokens = 10` |

- **There is no silent-truncation option.** Nothing in the request says "cut it to my window",
  and the service does not do it on its own initiative. Any caller that wants text embedded has
  to cut it explicitly, before sending, itself.
- **The rejection does not diagnose itself.** An input 0.55% over the limit comes back as
  `400 "The parameter is invalid. Please check again."` — indistinguishable, without measuring,
  from a malformed request, a wrong model name or a missing field. That is what makes the incident
  in §Background so hard to read: the same request shape succeeds for 8,397 documents and fails
  for one, and the error text points at "parameters", not at length.
- **The count arrives too late to be used.** `usage.prompt_tokens` is returned *with a successful
  response*; the request whose count you needed is precisely the one that fails and reports
  nothing. A caller therefore cannot probe its way to an answer — it must estimate the count
  *before* sending. And since one over-length item fails the entire batch, the estimate has to be
  conservative; there is no "let this one through and skip it" mode. (Which is why L3 exists, and
  why it isolates the failing input instead of retrying the batch as-is.)

### 2. There is no single way to count tokens across models

The catalog (`conf/all_models.json`) lists 215 embedding entries (152 distinct base models - the
rest are task and quantisation variants); the ones this deployment can count by name fall into
four tokenizer architectures, and the remainder are either hosted services that publish nothing at
all or architectures that are not implemented here (the split, regenerated from the catalog, is in
§Test strategy across all embedding models):

| family | examples | scheme | why an estimate from another family does not transfer |
|---|---|---|---|
| BERT WordPiece | bge-*-en, e5-*, gte-base, jina-v2 | 30k word pieces, greedy longest-match, case folding; words longer than 100 chars collapse to a single `[UNK]` | a 671-char single word is **2** tokens here and **146** under bge-m3 (73x) |
| XLM-R SentencePiece | bge-m3, multilingual-e5, m3e, jina-v3 | 250k Unigram, per-language subwords, byte fallback | CJK and punctuation split far more finely than in a byte-level BPE |
| byte-level BPE | Qwen3-Embedding, gte-Qwen, e5-mistral | merges over a 256-byte alphabet, never `[UNK]`, `▁` for spaces | no unknown-token escape hatch: every byte is billable, including invisible ones |
| tiktoken BPE | text-embedding-3-*, ada-002 | ~100k byte-level BPE tuned on English and code | compresses digits and repeated markup far more than the multilingual ones |

Two consequences: the provider cannot be asked for the answer (no embedding API in this
deployment exposes a pre-flight "how many tokens is this?" call, and the hosted models —
Gemini, Cohere, voyage, Titan, … — publish no tokenizer at all), so the **tokenizer file is the
only authority**; and it has to be *loaded and followed*, not guessed, because the file's own
declarations can be wrong (bge-m3's `.model` says `remove_extra_whitespaces = true` and the
tokenizer actually served does not honour it).

### 3. The estimates differ substantially, in both directions and per content class

Same text, each model's own tokenizer (`TOKENIZER_ORACLE_DIR` fixtures; cl100k measured locally):

| sample | chars | cl100k | bge-m3 | Qwen3-Embedding | e5-mistral | spread |
|---|---|---|---|---|---|---|
| `prose` | 1,800 | 401 | 521 | 401 | 481 | 1.3x |
| `markdown_table` | 1,770 | 1,110 | 1,171 | 1,440 | 1,500 | 1.6x |
| `digits` | 1,100 | 500 | 401 | 1,100 | 1,101 | 2.7x |
| `punctuation` | 1,600 | 481 | 641 | 481 | 681 | 2.5x |
| `fullwidth_forms` | 360 | 521 | 141 | 360 | 881 | **11x** |
| `emoji_zwj` | 360 | 702 | 361 | 480 | 841 | **10.5x** |
| incident span (t[42000:67000]) | 25,000 | 8,143 | 8,237 | 9,782 | 10,259 | 1.4x |
| incident document (`78785.md`) | 78,616 | 28,286 | 27,416 | 34,320 | 35,678 | 1.4x |

- The disagreement is not a rounding error, and **its sign changes with content**: cl100k is
  1.1% *below* bge-m3 on the incident span (the direction that silently defeats a safety margin)
  and **52% above** it on `numbers_and_units`; over the 34 non-trivial samples it is below on
  **14**, by up to **33%** (`punctuation`), and the spread between the cheapest and the most
  expensive counter is a **median of 2.3x**, up to **11x**.
- It is **content-dependent**, so a global fudge factor cannot fix it either: cl100k and bge-m3
  differ by 1.1% on the incident span but by 3.7x on `fullwidth_forms`. A constant margin wide
  enough for the worst class would discard most of the window for every other class. This is why
  the margin is proportional and why L2 *calibrates* the ratio per model from measured `usage`
  instead of assuming one.

> Fidelity to a tokenizer is not the same as being a usable estimator for a different
> architecture, and `long_word` shows how far apart they can be: a 671-character single word is
> 2 WordPiece tokens (the >100-char rule collapses it to one `[UNK]`) and 146 bge-m3 tokens. Sizing
> a byte-level model with a WordPiece count would be wrong by two orders of magnitude, in the
> unsafe direction.

In short: every send needs an explicit trim before it goes out (1); that trim can only be
computed accurately by the model's own tokenizer, which in practice means per-family
implementations (2); and for models whose tokenizer cannot be obtained or trusted, the only
honest fallback is a calibrated upper bound plus a shrink-and-retry path (3).

## The invariant

1. **Counting**: text handed to an embedding API is counted either with the
   embedding model's own tokenizer or with a **calibrated upper bound** of it.
   `cl100k_base` alone is never sufficient: it and a model's tokenizer disagree by
   roughly ±2%, and the sign depends on the content.
2. **Failure**: an over-limit rejection shrinks the input and retries. It never
   fails a document on its own, because the chunks of that document have already
   been deleted by the time the embedding runs.

Everything below exists to hold those two lines.

## Pipeline

```
kb.embd_id ──► ModelProviderService.GetEmbeddingModel
                 └─ service/model_service.go    model + APIConfig + declared max tokens
                     │
                     ├─ EmbeddingModel.ResolveTokenizerID()   catalog "tokenizer" id
                     └─ EmbeddingModel.ResolveMaxTokens()      extra.max_tokens → context_length → 2048
                        │
                        ▼
   TokenizerComponent.Invoke            internal/ingestion/component/tokenizer.go
     · per chunk: trimForEmbedding(embedder, text)
         └─ embedder implements EmbedderTrimmer ⇒ embedder.Trim(text)
     · embeddings: embedder.Encode(ctx, batch)      (batch = BatchSize(), default 16)
                        │
                        ▼
   embedder (internal/ingestion/task/embedder.go)
     · limiter()  = Counter(registry) + ratio(L2 calibration) + margin
     · Trim(text) = limiter.Trim(text, ResolveMaxTokens())
     · Encode: call → usage → TokenCount distribution → calibration
                └─ on over-limit: overLimitLadder → encodeIsolating → floor truncation
```

## The counters (L1)

`internal/tokenizer` owns a small registry. A `Counter` is:

```go
type Counter interface {
	ID() string                                  // catalog id
	Count(text string) int                       // tokens for this tokenizer
	TrimToLimit(text string, limit int) string   // longest prefix with Count <= limit
	Available() bool                             // the asset loaded
}
```

`RegisterCounterLoader(id, load)` installs a lazy loader (run at most once per
process; a failed load is remembered). `ResolveCounter(id)` falls back to cl100k,
and `CounterExact(id)` reports whether the id resolved to a real counter — that is
what decides whether a calibrated ratio has to be applied. Callers that must not
degrade check `CounterExact` first and refuse: the ingest embedder does, so a model
that declares an unavailable counter fails instead of being calibrated silently.

| id | family / models | asset (SHA-1 pinned) | implementation |
|---|---|---|---|
| `cl100k_base` | OpenAI `text-embedding-3-*`, `ada-002` | `ragflow_deps/cl100k_base.tiktoken` (pre-existing) | tiktoken-go + offline loader (`bpe_loader.go`) |
| `xlmr-spm` | `bge-m3`, `multilingual-e5-*`, `m3e`, `gte-multilingual`, `jina-embeddings-v3` | `…/BAAI/bge-m3/sentencepiece.bpe.model` `7e88c49f…` | `spm.go`: Unigram Viterbi, byte fallback, NFKC + whitespace + charmap-lite |
| `bert-wordpiece` | `bge-{large,base,small}-en-v1.5`, `bge-en-icl`, `e5-{base,large}-v2`, `gte-{base,large}`, `jina-embeddings-v2-*` | `…/BAAI/bge-large-en-v1.5/vocab.txt` `c3b41053…` | `wordpiece.go`: BertNormalizer + BertPreTokenizer + `##` longest-match |
| `qwen-bpe` | `Qwen3-Embedding-*` | `…/Qwen/Qwen3-Embedding-0.6B/tokenizer.json` `e6592f4d…` | `bpe.go`: hand-written Split scanner, GPT-2 byte alphabet, BPE |
| `llama-bpe` | `e5-mistral` (Mistral/Llama SentencePiece-BPE) | `…/intfloat/e5-mistral-7b-instruct/tokenizer.json` `92cb22e1…` | `bpe.go`: prepend/replace `▁`, whole-unit merge, byte fallback |

The XLM-R family shares one vocabulary file, so one 5 MB asset covers five model
families; the BERT family shares `bert-base-uncased`'s vocab.

## Choosing a limit (margin + L2)

`EmbeddingTokenLimit(maxTokens)` = `maxTokens − max(32, 2%)`. The margin is a
ratio on purpose: the previous constant was 10 tokens, 0.12% of an 8192 window,
an order of magnitude smaller than the disagreement it was meant to absorb.

`ResolveEmbeddingMaxTokens(declared, contextLength)` prefers the model's own
`max_tokens`, then the catalog's `context_length`, then `EmbeddingTokenLimitDefault`
(2048 — deliberately the small end, because overshooting a window is a rejected
request while undershooting only truncates). The catalog has 512-token embedding
models; a hard-coded 8192 would have overshot those by 16x.

`Limiter` binds a counter, a calibration and a margin:

```go
Limiter.Limit(maxTokens) = EmbeddingTokenLimit(floor(ResolveEmbeddingMaxTokens(...) / Ratio()))
Limiter.Trim(text, maxTokens) (trimmed string, tokens int)
```

- `NewExactLimiter(counter)` — the counter *is* the model's tokenizer, ratio 1.
- `NewCalibratedLimiter(counter, key, cal)` — ratio read from the calibration **on
  every call**, so an over-limit rejection recorded by a retry tightens the budget
  of the very next attempt, not of the next document.

### Calibration

`Calibration` learns `real / own` per `(provider instance, model)` — the key is
`embedder.quotaKey()` (endpoint + region + model + hashed API key), the same key the
rate-limit cooldown uses. It only ratchets **up**:

| Observation | Inference |
|---|---|
| `ObserveUsage(key, own, real)` on a successful call | `ratio = max(ratio, real/own)` |
| `ObserveOverLimit(key, own, maxTokens)` on a rejection | `ratio = max(ratio, maxTokens/own × 1.01)` |

`ObserveOverLimit` must be given **the largest single input's count, not the batch
total**: the window bounds each input, and a batch total is normally above the
window, which would imply a ratio below 1 and teach nothing.

For a model with no exact counter the starting bound is
`DefaultUncountedRatioUpper = 1.05`, which costs ~5% of truncation headroom and is
what makes unknown and future models safe without any asset.

## Recovering from an over-limit rejection (L3)

`isOverLimitErr` classifies a 4xx whose message matches `20015` (SiliconFlow's code
for exactly this — it answers a generic "The parameter is invalid") or the usual
"too long / maximum context / context length / token limit / …" phrasings. A 5xx is
never treated as a size problem.

`overLimitLadder(budget)` = `budget, 0.75b, 0.5b, 0.25b, 0.125b`, floored at
`overLimitFloorTokens = 64`. The first steps are gentle because an over-limit
rejection usually means the input is a little past the window; cutting a quarter of
the content for that would lose text for nothing. Trimming is a **prefix
operation**, so re-trimming an already trimmed text simply shortens it — the ladder
needs no access to the untrimmed input.

If the whole batch still fails at the smallest budget, `encodeIsolating` embeds the
inputs one at a time (a batch is allowed to contain exactly one pathological input)
and returns an error only if an input fails at the floor, i.e. for reasons unrelated
to size.

## Wiring, and the seams that keep it back-compatible

- `component/tokenizer.go` prefers two **optional** interfaces, so every existing
  test stub still satisfies `Embedder`:
  `EmbedderTrimmer{ Trim(text) (string, int) }` and
  `EmbedderMaxResolver{ ResolveMaxTokens() int }`. Without them the component falls
  back to `truncateForEmbedding`, which now also uses the resolved window and the
  proportional margin.
- `embedder.Encode` passes a `*common.ModelUsage` into the driver and reads
  `InputTokens` back. `distributeTokenCount` spreads that request-level total over
  the inputs by our own counts (remainder to the largest, so the sum equals the
  provider's number); with no usage it falls back to our own count rather than to 0,
  which is what it used to report.
- `EmbeddingData.TokenCount` documents that per-input semantics; the stub FIXME is
  gone. `openai.go` now records usage like `siliconflow.go` already did.

## Data: the model catalog

`conf/all_models.json` gained a `tokenizer` field per model (next to the existing
`max_batch_size` / `context_length`). **36** entries are tagged (7 `xlmr-spm`, 14
`bert-wordpiece`, 9 `qwen-bpe`, 6 `cl100k_base`).

Only families whose counter was verified against the model's own tokenizer are
tagged. Untagged models are not a gap: they resolve to the calibrated cl100k
fallback, which errs toward more headroom. A wrong tag is the dangerous direction,
so guessing is deliberately avoided - §Test strategy across all embedding models
states the invariant this rests on and shows how much of the catalog each tag covers.

Two escape hatches for operators (both win over the catalog):
`TOKENIZER_EMBEDDING_TOKENIZER` (pin an id) and `TOKENIZER_EMBEDDING_MAX_TOKENS`
(pin a window). A third one is about where the assets live rather than how they are used:
`MODEL_ASSETS_DIR` points at a directory holding the downloaded model assets in the
HuggingFace repo layout (see §Assets and the oracle).

## Test strategy across all embedding models

Verification here is per **architecture**, not per catalog entry - and the catalog is
mostly entries, not architectures. `scripts/tokenizer_coverage.py` reads
`conf/all_models.json` and prints the current picture (no network, no per-model
guessing):

```
embedding entries: 215 | distinct base models: 152 | tagged entries: 36 | calibrated entries: 179
```

| tagged counter | entries | distinct base models | scheme |
|---|---|---|---|
| `bert-wordpiece` | 14 | 14 | BERT WordPiece |
| `qwen-bpe` | 9 | 6 | byte-level BPE (Qwen pre-tokenizer regex) |
| `xlmr-spm` | 7 | 7 | XLM-R Unigram SentencePiece |
| `cl100k_base` | 6 | 6 | tiktoken BPE (OpenAI table) |

| untagged group (name heuristic) | entries | base models | what it would take |
|---|---|---|---|
| hosted services (cohere, voyage, gemini, titan, nvidia NIM, …) | 77 | 77 | nothing to verify against: no published tokenizer. L2/L3 only |
| other architectures (`jinaai/jina-embeddings-v5*`, `google/embeddinggemma-*`, …) | 95 | 35 | a new implementation + its fixture + an oracle |
| `bert-wordpiece` | 4 | 4 | byte-identity evidence against the family fixture; a different vocabulary (`BAAI/bge-large-zh-v1.5`, `netease-youdao/bce-embedding-base_v1`, …) needs its own artifact + fixture + oracle - see The tag invariant |
| `llama-bpe` | 3 | 3 | byte-identity evidence against the family fixture; a different vocabulary (`mistral/mistral-embed`, two `nvidia/llama-nemoretriever-*`) needs its own artifact + fixture + oracle - see The tag invariant |

The groups are a *prioritisation* heuristic, not a claim: a model is tagged only after
its own artifact has been through the oracle (recipe below).

### What one fixture per architecture does and does not prove

| architecture | fixture artifact | what varies between models of this family | read from the artifact? | exercised by the corpus? |
|---|---|---|---|---|
| `bert-wordpiece` | `BAAI/bge-large-en-v1.5/vocab.txt` | `do_lower_case`, `strip_accents`, `handle_chinese_chars`, `max_input_chars_per_word`, the unk token | **no** - `wordpiece.go` hard-codes the family's shape (`lowercase=true`, `strip_accents` follows it, `max_input_chars_per_word=100`); a cased or differently-configured BERT needs its own family | **partly** - the fixture is `lowercase=true`; no cased model is in the corpus |
| `xlmr-spm` | `BAAI/bge-m3/sentencepiece.bpe.model` | the precompiled charmap, `add_dummy_prefix`, `remove_extra_whitespaces`, `escape_whitespaces`, byte fallback, whether a `Metaspace` pre-tokenizer splits at all | yes | whitespace and charmap classes yes; **byte fallback no** (bge-m3 has none) |
| `qwen-bpe` | `Qwen/Qwen3-Embedding-0.6B/tokenizer.json` | the pre-tokenizer regex, `byte_fallback`, unk presence, `NFC` | yes - the regex is copied verbatim from the artifact | one artifact only |
| `llama-bpe` | `intfloat/e5-mistral-7b-instruct/tokenizer.json` | `byte_fallback`, the `▁` prepend/replace normalizer | yes | one artifact only |
| `cl100k_base` | the shipped `cl100k_base.tiktoken` | nothing - one table | n/a | complete |

So the honest statement of coverage is: **the algorithm is verified per family, the
vocabulary per fixture artifact.** A model that differs from the fixture only in
vocabulary size is *not* covered by the fixture, and that is the axis the table is for.

### The tag invariant

> A tag means "this model's tokenizer **is** the artifact the fixture verified", not
> "this model looks like that family".

The counters load **one fixed artifact per family** (`bert-wordpiece` always reads
`bge-large-en-v1.5/vocab.txt`, `xlmr-spm` always reads `bge-m3/sentencepiece.bpe.model`).
Tagging a model whose vocabulary differs - a Chinese `bge-large-zh`, a Jina-v2 BERT with
its own vocab, a cased model - would count it with the wrong vocabulary and *look exact
while being wrong*, which is worse than the calibrated fallback. Two consequences worth
knowing: the loaders also search a bare `vocab.txt` in the working directory, so dropping
an unrelated `vocab.txt` there silently replaces the verified artifact; and auditing the
36 existing tags against their own artifacts is an open item (the runtime calibration in
§Calibration is what would surface a mistake, as a drift in `real/cl100k`).

### Adding a model (recipe)

1. **Identify the architecture** from the model's own artifact (`tokenizer.json`, or
   `vocab.txt` / `sentencepiece.bpe.model`). If the vocabulary is byte-identical to a
   family fixture, only step 4 is needed; if it differs, the model needs its own fixture.
2. **Add the artifact** to `ragflow_deps/download_go_deps.py`'s `TOKENIZER_ASSETS` list -
   individual files, not `snapshot_download` of a multi-GB repo - pin its SHA-1 in the
   loader map so a mismatch fails at load instead of counting with the wrong table, and add
   it to the copy loop in `Dockerfile` / `Dockerfile_base` / `Dockerfile_go` so every runtime image ships it
   (the `ragflow_deps` image is built from this script's output, so it needs a refresh too -
   see §Assets and the oracle for the whole chain).
3. **Add the fixture line** to `ORACLES` in `scripts/gen_tokenizer_oracle.py`, generate it,
   and run `TestCountersMatchOracle`. Passing means every sample agrees, or sits inside the
   flagged bounded class (the sample count follows `--fuzz`, see §Assets and the oracle);
   anything else is a bug in the counter, not in the model.
4. **Then tag it** in `conf/all_models.json`.
5. **Record it** - regenerate the coverage table with
   `scripts/tokenizer_coverage.py --markdown`. If the new model exercises an axis that was
   not exercised before, add the sample class to the corpus in the same change.

Acceptance criteria for steps 3-5 are the ones in §Verification: oracle at token level,
property tier green, `gofmt` clean.

### What this strategy deliberately does not attempt

- **A fixture per catalog entry** (152 base models). The code encodes the *algorithm* and
  the artifact encodes the *vocabulary*; per-entry fixtures would mean ~150 downloads and
  no extra code coverage - the axes table above is what finds bugs, not the model count.
- **Claiming exactness for untagged models.** They are *calibrated* at runtime and
  observed through the provider's `usage`; the doc says "calibrated", never "verified".
- **Following the catalog's `context_length` blindly** - it is honoured as declared, and
  §Not covered keeps the caveat that nothing here validates what a provider accepts.

### Next moves, cheapest first

1. Tag the **7 base models** whose architecture is already implemented - it needs their
   artifacts, one fixture each, no new code.
2. Close the **`lowercase=false` WordPiece axis** (one cased model fixture) - today that
   flag is read but never exercised.
3. Exercise **`byte_fallback=true` on the SPM family** (a `.model` that ships the byte
   pieces), which the current fixture cannot reach.
4. Only then a **new architecture**, chosen by entry count: the `jina-embeddings-v5*` /
   `embeddinggemma` group is ~35 base models.

## Assets and the oracle

Assets are fetched, never committed: `ragflow_deps/download_go_deps.py` (the Go-side
downloader - `download_deps.py` is upstream and stays untouched) has a `TOKENIZER_ASSETS`
list that downloads **individual files** (not `snapshot_download` of a repo that also
carries multi-GB weights) into `ragflow_deps/huggingface.co/<repo>/<file>`, which
`.gitignore` already excludes (`huggingface.co/`). Loaders read them from disk with a
SHA-1 pin, walk up from the working and executable directories like `bpe_loader.go`, and
perform no network I/O. A missing asset fails loudly at every layer that can report one: the
runtime image does not build without it (the copy loop below exits non-zero), an embedder whose
model declares a tokenizer it cannot load refuses to count rather than substituting the
calibrated estimate, and the cl100k table - the one asset that still comes from
`download_deps.py`, which fetches it from the OpenAI blob - has always been a startup panic.

**The whole chain, because a gap in it is invisible**: `download_go_deps.py` writes
`ragflow_deps/huggingface.co/…`; the `ragflow_deps` image is built from that directory
(`ragflow_deps/Dockerfile` copies `huggingface.co` to `/huggingface.co`); the root
`Dockerfile`, `Dockerfile_base` and `Dockerfile_go` bind-mount that image and copy the four *runtime*
tokenizer assets into `/ragflow/ragflow_deps/huggingface.co/…`, which is where the
counters look (the working directory is `/ragflow`). The `tokenizer.json` files that
exist only as cross-check oracles are deliberately **not** shipped. Two consequences
worth knowing: adding an asset means touching all three places (script, deps image,
runtime image) - and the copy loop **fails the build** when the deps image does not carry
it, so no image can be produced with a counter that would silently fall back - and an
embedder whose model declares a tokenizer it cannot load **fails the ingest** with
`embedding tokenizer "<id>" is declared for <model> but its asset is unavailable …:
refusing to count with the calibrated estimate`, instead of counting differently.

The three places are held together by `ragflow_deps/test_tokenizer_assets.py`, which
fails when a `"runtime"` asset is missing from either Dockerfile, when an `"oracle"`
asset is shipped, or when a runtime asset has no 40-hex SHA-1 pin in
`internal/tokenizer/*.go`. It is there because the first version of this chain silently
skipped step three: the counters loaded fine in a developer checkout and quietly
degraded to the calibrated path in the image.

### Where the assets are looked up, and how to move them

By default the loaders walk the working directory and the executable's directory (and
their ancestors) for the shipped relative names. `MODEL_ASSETS_DIR` overrides that: it
names a directory playing the role of `ragflow_deps/` - downloaded model assets in the
HuggingFace repo layout - and it is **consulted first**. Three spellings are accepted, so
an operator can point at whichever tree they have:

| `MODEL_ASSETS_DIR` points at | candidate used |
|---|---|
| the `ragflow_deps`-like root | `<dir>/huggingface.co/<repo>/<file>` |
| a `huggingface.co`-like root | `<dir>/<repo>/<file>` |
| a flat directory | `<dir>/<file>` |

The variable is deliberately **not** embedding-specific: DeepDoc's weights use the same
layout (`huggingface.co/InfiniFlow/deepdoc`) and `resolveDeepDocModelDir` honours it too,
so one mount can serve every downloaded model asset. `ragflow_deps/download_go_deps.py` and
the three runtime Dockerfiles keep writing the same tree, so nothing changes for a default
deployment. The layout and the precedence are pinned by tests:
`internal/common/model_assets_test.go` for the candidate list, and
`internal/tokenizer/asset_dir_test.go` for the end-to-end case (a child process with an
unrelated working directory loads the SPM counter from the configured tree).

Two things make the state of the assets observable:

- each counter reports the file it loaded (`SourcePath`), so logs and failures can name it;
- the ingestor logs an **availability report once at startup** -
  `embedding tokenizer counters {available: [...]}` plus, when something is missing, a
  warning naming the unavailable ones and pointing at `ragflow_deps/download_go_deps.py` /
  `MODEL_ASSETS_DIR`. The report is diagnostics rather than the guard: startup is not fatal
  (untagged models count with the calibrated estimate by design, and they keep working), but
  ingesting a model that declares a missing asset fails outright - so this report is what
  tells an operator which asset to restore.

The oracle is the point of the whole test story: a hand-written tokenizer that is
"close" is not good enough, because an **under-count** lets an oversized input
through and the provider answers 400.

```
mkdir -p /tmp/tokenizer_oracle
for m in bge-m3 bge-large-en qwen3-embedding e5-mistral cl100k; do
  uv run scripts/gen_tokenizer_oracle.py --model $m --corpus /path/to/78785.md \
    --out /tmp/tokenizer_oracle/$m.json --fuzz 300
done
TOKENIZER_ORACLE_DIR=/tmp/tokenizer_oracle ./build.sh --test -run TestCountersMatchOracle ./internal/tokenizer/
```

`--corpus` is required: the document's **whole** content becomes one sample (`incident_full`),
so the corpus is whatever the caller wants checked - the incident document `78785.md` (see
§Background) is the one that exposed the bug.

Current state: **339 samples per counter (`--fuzz 300`: 32 hand-written samples + the whole
`--corpus` document + 300 fuzzed + 6 fixed extremes), all five counters passing** - four of them
compared token by token, `cl100k_base` by token id. The fixture records the reference's
**token ids and token texts**; comparing texts is what makes the comparison meaningful,
because two different segmentations can share a token count (the incident document had
exactly one such spot, 18,182 tokens in).

### What each counter is checked against

| counter | reference implementation | what is compared |
|---|---|---|
| `cl100k_base` | OpenAI's own `tiktoken` (Python 0.12.0) over the table shipped in `ragflow_deps` | token **ids** (the numbering is shared - both read the same table) |
| `xlmr-spm` | the HuggingFace conversion of `BAAI/bge-m3` (the files SILICONFLOW serves), via `tokenizers` 0.22.2 reading the model's `tokenizer.json` | token **texts** - this family's ids are renumbered by the conversion (below) |
| `bert-wordpiece` | the same library reading `BAAI/bge-large-en-v1.5/tokenizer.json`; the algorithm itself is Google's `WordPiece` / `BasicTokenizer` | token texts (ids match as well) |
| `qwen-bpe`, `llama-bpe` | the same library reading each model's `tokenizer.json` | token texts (ids match as well) |

Two things this table deliberately does **not** claim:

- **Google's `sentencepiece` is a second opinion, not the authority.** It is installed
  (`.venv`, 0.2.2); run over the same corpus it leaves 253/341 samples identical and comes
  out *lower* than the served conversion on 82 of them, every one of them whitespace-shaped
  (the `.model`'s `remove_extra_whitespaces`, which the canonical implementation honours and
  the served conversion does not). Following it there would **under-count**, i.e. fail in the
  dangerous direction - which is the measured reason the served conversion is the reference.
  Method, the remaining classes and the limits of the check are in §The three-way cross-check
  below.
- **Token ids are not portable across artifacts.** The bge-m3 conversion reorders the
  SentencePiece specials and inserts two tokens, so the same piece is `id` in the
  `.model` and `id+1` in the served vocabulary (`<unk>,<s>,</s>,…` becomes
  `<s>,<pad>,</s>,<unk>,…`). Comparing ids across that boundary would report a fake
  failure on every sample; comparing the *texts* isolates real segmentation
  differences. (HuggingFace also renders an unknown token as the source text it could
  not encode, so the fixture normalizes those to `<unk>`.)

### The three-way cross-check (what it is, and how to read it)

For `xlmr-spm` three parties are compared: **two readers of the same `.model` file**, plus
the conversion that is actually served.

| party | what it reads | how its ids are numbered |
|---|---|---|
| **ours** (`spm.go`) | `sentencepiece.bpe.model` | `.model` piece index |
| **the served conversion** (HF `tokenizers`) | `tokenizer.json` built from that `.model` | *renumbered*: same pieces, specials reordered, `<pad>`/`<mask>` inserted ⇒ regular pieces are `+1` |
| **Google `sentencepiece`** (canonical C++) | the same `sentencepiece.bpe.model` | `.model` piece index - i.e. **the same numbering we use** (verified: `▁3`=137, `43`=11547, `<unk>`=0, 250,000 pieces) |

How it runs, and how to read it:

```bash
uv run scripts/gen_tokenizer_oracle.py --model bge-m3 --corpus /path/to/78785.md \
  --out /tmp/tokenizer_oracle/bge-m3.json --fuzz 300 --spm-crosscheck
```

The flag adds `spm_count` per sample and a `crosscheck` note to the fixture, and prints the
number of samples where the two readers disagree. **The Go test never reads that field**, so
the cross-check cannot change a verdict - it is evidence, not an assertion.

Measured result on the 341-sample corpus the cross-check was run on (the corpus document
is sampled whole now, so a re-run is two samples short of that):

| class | samples | cause | what it means |
|---|---|---|---|
| both readers agree, count **and** piece sequence | **245/341** | - | the canonical implementation independently reproduces the segmentation; since our counter is token-identical to the served reader on this corpus, our ids equal Google's there too |
| count equal, piece sequence differs | 8/341 | combining marks (NFKC composes, the charmap does not) | the same hazard class as the `known_approx` flag |
| Google **lower** by 1 | 80/341 | `.model` declares `remove_extra_whitespaces = true`; the canonical reader honours it (trims leading/trailing whitespace), the served conversion does not | following Google here would **under-count** - the dangerous direction |
| Google lower by 2 | 2/341 | same | same |
| Google **higher** by 1 | 6/341 | combining-mark / charmap shapes | same class as the `known_approx` flag |

**Decision rule.** The served conversion decides. Google's implementation is an *independent
check of our modelling*: if our output matched neither reader, that would be a bug in `spm.go`.
A disagreement *between the two readers* is never by itself a reason to move `spm.go` towards
`.model` semantics - the table above is where such a change has to be argued, with numbers.

**Limits of the check**, so it is not over-read:

- It compares **counts** (plus the piece strings of the two readings). Comparing ids directly
  still needs two differences modelled first: the served conversion's renumbering, and the fact
  that the served pipeline runs one Viterbi *per pre-token* while SentencePiece runs one over
  the whole text.
- It covers `xlmr-spm` only - it is the one family with a `.model` file, and therefore the only
  one with a canonical implementation to check against.
- It is a second opinion, not an extra gate: only `TestCountersMatchOracle` and the property
  tier fail a build.

### The fuzz corpus

`--fuzz N` appends `N` deterministic random strings - seeded, with the texts stored in
the fixture so the Go side never has to reproduce the RNG - drawn from 17 alphabets:
ASCII words, CJK, Greek, Cyrillic, Hebrew, Arabic, emoji (ZWJ sequences and skin-tone
modifiers included), combining marks, zero-width characters, control characters,
full-width and compatibility forms. It also appends boundary cases (`a`×99, `a`×100,
`a`×101 for WordPiece's 100-character rule) and a printable-ASCII sweep.

The two halves catch different things: the hand-written samples found the first five bugs
in the table below, and the fuzz corpus found every one of the five at the end of it - on
its first run, after the hand-written corpus had been passing.

### What the verification stack has caught

Every one of these looked correct in review and passed a prose-only corpus. The
"caught by" column is the reason the tiers in the next section are not optional:
the oracle catches *modelling* mistakes, the property test catches *contract*
mistakes, and neither substitutes for the other.

| Bug | Symptom | Caught by |
|---|---|---|
| SPM followed `.model`'s `remove_extra_whitespaces` instead of the served HF conversion | one token short on 15/23 samples | oracle, `leading_trailing_space` |
| BERT `isPunctuation` included Unicode **Symbols** | every emoji cost an extra token | oracle, `emoji` |
| BERT `clean_text` turned control/format characters into spaces instead of deleting them | "a\u200bb" split into two words | oracle, `control_chars` / `zero_width_and_nbsp` |
| BERT `strip_accents` short-circuited when the input was already NFD | decomposed accents counted twice (the marks were the thing to remove) | oracle, `decomposed_accents` |
| SPM ignored the precompiled charmap's delete/space rules | control and zero-width characters were billed as bytes | oracle, `control_chars` |
| BPE merged **one occurrence** of the lowest-rank pair per evaluation, and the heap broke rank ties arbitrarily | 32 `a`s counted as 9 instead of 7 | oracle, `long_word` |
| `cl100k`'s trim cut the token list without snapping to a rune boundary | a trimmed emoji chunk was invalid UTF-8 (and would have gone into the index that way) | **property test**, `emoji` at `limit=1` |
| BERT WordPiece kept a word's already-matched prefix and then one `[UNK]` for the rest | Google's rule is that if **any** position of a word fails to match, the whole word becomes a single `[UNK]`; the old behaviour over-counted such words | **token-level oracle**, fuzz corpus |
| SPM ran one Viterbi over the whole text | the served pipeline runs one **per Metaspace pre-token**; on the incident document that is count-neutral (27,416 tokens either way) but a different segmentation at byte 46,673 | **token-level oracle**, `incident_full` |
| SPM billed a bare `▁` for text the charmap deletes entirely | `"\x7f\x01\x7f"` counted 1 token where the model sees 0 | **token-level oracle**, fuzz corpus |
| SPM emitted one token per unmatched character | the served model fuses a whole run into **one** unknown token (`🧑🧑`, five NULs, each one token) | **token-level oracle**, fuzz corpus |
| SPM accumulated Viterbi scores in float32 | ties between equal-scoring segmentations broke the other way (a run of 99 `a`s) | **token-level oracle**, fuzz corpus |

The BPE one is worth repeating for anyone touching `bpe.go`: the canonical rule is
"take the lowest-rank adjacent pair and merge **every** non-overlapping occurrence
of it", and when several occurrences share a rank the leftmost one goes first. Both
halves matter, and both are pinned by tests.

## How the implementation is kept honest

Two questions decide whether a counter may be trusted: **what is the normative
reference**, and **what has been proven against it**. Both are answered per counter,
because "it looked right in review" is how all twelve bugs above got in.

### 1. The normative reference is the model's own artifact, nothing else

Each counter is derived from a file that ships in the repository and is pinned by
SHA-1. The loader refuses a digest mismatch, so a test can never silently run
against a different vocabulary:

| counter | authoritative artifact (pinned) | format / algorithm reference | behavioural reference |
|---|---|---|---|
| `cl100k_base` | `ragflow_deps/cl100k_base.tiktoken` (`expectedBpeHashes`, `bpe_loader.go`) | tiktoken's table format (`<base64 token> <rank>` per line) | the table itself, plus the Python anchors in `TestNumTokensFromString_MatchesPythonAnchors` |
| `xlmr-spm` | `BAAI/bge-m3/sentencepiece.bpe.model` `7e88c49f…` | `sentencepiece/model.proto` wire format (decoded by the minimal protobuf reader in `spm.go`): `pieces`, `trainer_spec.model_type = UNIGRAM`, `normalizer_spec`; Unigram Viterbi; `BYTE` pieces for fallback | the HuggingFace conversion of the same repo (`tokenizer.json`) — it decides whitespace and pre-tokenization behaviour, and it **wins over the `.model` flags** when they disagree |
| `bert-wordpiece` | `BAAI/bge-large-en-v1.5/vocab.txt` `c3b41053…` | HuggingFace `BertNormalizer` / `BertPreTokenizer` / `WordPiece`, whose configuration is quoted from the model's `tokenizer.json` in `wordpiece.go`'s header; BERT's own `_is_punctuation` (category **P**, not S) | the same `tokenizer.json` |
| `qwen-bpe` | `Qwen/Qwen3-Embedding-0.6B/tokenizer.json` `e6592f4d…` | the artifact's `pre_tokenizer` regex, copied verbatim into `bpe.go`, plus GPT-2's `bytes_to_unicode` alphabet and canonical BPE merge order | the artifact's own `vocab` + `merges` (`NFC`, `byte_fallback=false`) |
| `llama-bpe` | `intfloat/e5-mistral-7b-instruct/tokenizer.json` `92cb22e1…` | the artifact's `normalizer` (`Prepend("▁")` + `Replace(" "→"▁")`) and `byte_fallback=true` | the same artifact |

Two consequences of taking the artifact as the spec, both learned the hard way:

- **A declaration inside the artifact is not evidence.** bge-m3's `.model` declares
  `remove_extra_whitespaces = true`; the tokenizer providers actually serve does not
  honour it. When the file and the served conversion disagree, the served conversion
  is what decides whether a request fits the window, so it is the reference we
  follow — and the oracle is what tells us they disagree.
- **What cannot be reproduced exactly is enumerated, not glossed over.** The
  `nmt_nfkc` charmap is a 232 KB Darts trie; `spm.go` implements the subset that
  NFKC does not already cover, and each rule in it was derived by **probing the
  model one character at a time** (delete `U+0001-0008`, `U+000B`, `U+000C`,
  `U+000E-001F`, `U+007F-0084`, `U+0086-009F`; `U+200B-200D`/`U+FEFF` to space;
  keep `U+0000`, `U+0085`, `U+2060`) rather than copied from a description. The
  same applies to the models with no public tokenizer at all (see §4).

### 2. What is proven, tier by tier

| Tier | Test | What it would catch |
|---|---|---|
| Artifact integrity | SHA-1 pins (`expectedSPMHashes`, `expectedWordPieceHashes`, `expectedBPEHashes`), plus `model_assets_test.go` / `asset_dir_test.go` for `MODEL_ASSETS_DIR` | a corrupt download, a swapped vocabulary, or an asset tree the process cannot see — the load fails instead of counting with the wrong table, and the startup report says which counters are usable |
| Model fidelity | `TestCountersMatchOracle` (fixtures from `scripts/gen_tokenizer_oracle.py`, `--fuzz 300`) | "our tokenizer is not the model's tokenizer" - per token, not per sample count, so a segmentation that happens to have the right length still fails |
| Counter contract | `TestCountersSatisfyTrimProperties`, `TestCountTrimFitsEveryLimit` | a counter that does not return a prefix, splits a rune, exceeds the limit it was given, is not idempotent, or shrinks when given a larger limit |
| Layer maths | `embedlimit_test.go` (margin, window resolution, calibration ratchet + over-limit inference, limiter, byte-level fallback) | an off-by-one in the margin, a ratio that should have ratcheted up, a fallback that reports invented counts |
| Ingest behaviour | `embedder_limit_test.go` (shrink ladder, per-input isolation, `20015` classification, token distribution, window honoured) | a document that fails instead of shrinking; token counts that do not sum to the provider's |
| Runtime oracle | `Calibration.ObserveUsage` / `ObserveOverLimit` | drift in a counter nobody can test offline — verification keeps running in production |
| End-to-end gate | re-ingest the incident document, compare MySQL vs ES, `covsweep` = 0 unserved words | everything above being green while content is still lost |

The oracle assertion is **equality**, deliberately not a tolerance: "within 2%"
would have hidden every row of the bug table, because the whole failure mode is a
2% disagreement in the unsafe direction (too few tokens counted ⇒ the provider
rejects what we sent).

### 3. One bug, one sample class

A bug is not fixed until the corpus can fail on it. The expectation is regenerated
from the model (`gen_tokenizer_oracle.py`), never typed by hand, so "expected" always
means "what the model does":

| Sample class | Exists because of |
|---|---|
| `emoji` (incl. ZWJ) | BERT treating Symbols as punctuation; the `cl100k` UTF-8 cut |
| `control_chars` | BERT `clean_text` and SPM charmap delete rules |
| `zero_width_and_nbsp` | BERT Cc/Cf deletion |
| `decomposed_accents`, `vietnamese` | BERT `strip_accents` short-circuit |
| `long_word` (a long run of one character) | BPE merge order and rank tie-breaking |
| `leading_trailing_space`, `tabs_and_crlf` | SPM whitespace semantics |
| `fullwidth_forms`, `compatibility_chars`, `cjk_compat`, `math_and_currency` | the charmap approximation (NFKC vs `nmt_nfkc`) |
| `prose`, `markdown_table`, `issue_numbers`, `digits`, `base64_like`, `incident_*` | the real corpus, including the span that caused this work |
| the fuzz corpus (`--fuzz N`, 17 alphabets) | everything the hand-written samples missed: the pre-token boundary, the run-of-unknowns rule, whitespace-only-after-deletion, the float32 tie, and WordPiece's whole-word `[UNK]` |

### 4. Rules that keep it from rotting

1. **No counter without a fixture.** A family whose tokenizer cannot be checked
   against the model is not tagged in the catalog; it falls back to the calibrated
   path. Guessing a tag is the unsafe direction.
2. **Nothing is hand-typed.** Fixtures and expected counts come from the model's
   tokenizer; assets come from the downloaders (`download_go_deps.py` for the tokenizer
   files, `download_deps.py` for the cl100k table) with a SHA-1 pin.
3. **Equality, not resemblance** (see above), and the dangerous direction
   (under-count) is the one the tests are shaped around.
4. **Every approximation is named** in §Known approximations together with the
   backstop that covers it — an unnamed approximation is a bug report waiting to
   happen.
5. **Verification continues at runtime.** 77 of the 215 catalog entries (name heuristic;
   §Test strategy across all embedding models) are hosted services with no downloadable
   tokenizer, so the provider's own `usage.prompt_tokens` is the only reference that exists
   for them. That is why `TokenCount` was implemented and why calibration is not optional.
6. **The gate is end-to-end.** Unit and oracle tests can all be green while a
   document still loses text; the acceptance check is the incident document plus
   `covsweep` reporting zero unserved words.

### 5. What is not provable, and what covers it instead

| Gap | Coverage |
|---|---|
| Hosted models with no tokenizer (Gemini, Cohere, voyage, Titan, …) | L2 calibration from `usage` + L3 shrink-and-retry |
| `nmt_nfkc` charmap outside the probed subset | L2/L3; the gap is enumerated in §Known approximations |
| A provider that reports no `usage` | falls back to our own count; L3 still guards the limit |
| Content classes nobody has sampled yet | the property tier holds for arbitrary input, and a mis-count shows up as a rejected request that L3 absorbs rather than as a failed document |
| Google's `sentencepiece` (canonical implementation of the `.model`) is cross-checked, but it disagrees with the served conversion on 88/341 samples | enumerated and explained in §The three-way cross-check (whitespace trimming, 80 × `-1`); the served conversion remains the reference because the other reading under-counts in the dangerous direction |
| Two shapes of combining marks, where NFKC and the served charmap provably differ | flagged per sample and held to ≤ 2 tokens (see §Known approximations); every other sample is compared exactly |
| A finite corpus, however adversarial: 339 samples per counter is evidence, not proof | the corpus is seeded and regenerated from the model on demand (`--fuzz N`), so coverage is a dial rather than a constant, and the runtime oracle covers the rest |

The BPE one is worth repeating for anyone touching `bpe.go`: the canonical rule is
"take the lowest-rank adjacent pair and merge **every** non-overlapping occurrence
of it", and when several occurrences share a rank the leftmost one goes first. Both
halves matter, and both are pinned by tests.

## Performance

- `spm.go` / `wordpiece.go` count in O(n) over the text (trie walk + Viterbi);
  `trimToLimit` binary-searches rune prefixes, so O(n log n).
- `bpe.go` keeps a min-heap over adjacent pairs keyed by `(rank, position)` and
  drains it per rank: O(n log n), plus a per-piece memo (pieces repeat far more than
  they differ). The first two implementations were O(n²) or wrong; the current one
  counts the 78 KB incident document in **0.09 s**, against **181 s** for the
  rescanning version.
- The whole oracle suite - five counters, 341 samples each, including the 78 KB
  incident document - runs in about 1.2 s.

## Known approximations (deliberate)

- `xlmr-spm` implements the **subset** of the model's 232 KB precompiled `nmt_nfkc`
  charmap that NFKC does not already cover, restricted to the rules verified
  character by character against the model (delete `U+0001-0008`, `U+000B`,
  `U+000C`, `U+000E-001F`, `U+007F-0084`, `U+0086-009F`; `U+200B-200D`, `U+FEFF` and
  the escaped space `U+2581` to space; keep `U+0000`, `U+0085`, `U+2060`).
- **Combining marks are where that subset is measurably short**, and the oracle test
  says exactly which samples are affected instead of hiding them. Our normalization
  starts from NFKC, which composes and reorders; the served charmap does neither, and
  it drops marks that cannot attach to a letter (measured: `②`+`U+0327` normalizes to
  `2`, a mark orphaned by a deleted control character disappears, `a`+`U+0301`+`U+0327`
  keeps three characters). Two shapes are therefore flagged per sample by
  `gen_tokenizer_oracle.py` (`has_combining_run`, `has_dangling_mark`): two or more
  marks on one base, and a mark that does not follow a letter. Those samples are held
  to a **bounded** difference (≤ 2 tokens) rather than to equality; every other sample
  is still compared exactly, which is 156/339 for `xlmr-spm` and 339/339 for
  `cl100k_base` in the current fixture. Without `--fuzz` no sample is flagged at all -
  the hazard needs combining marks, which real prose rarely has.
- Unusual Unicode outside that set can still be off by a token or two; L2 and L3 are
  the backstop.
- Untagged model families (e.g. `mistral-embed`, `gte-Qwen`) use the calibrated
  fallback on purpose.
- Truncation is silent by design here: the counter's job is to fit the window. Not
  sending an oversized chunk at all is the chunker's job, and oversized chunks still
  exist in the index (up to 104k characters) — see "Not covered".

## Not covered

- **Chunker hard cap.** Tables and other delimiter-free blocks are still emitted as
  chunks far above `chunk_token_size`. Today that costs an extra shrink-and-retry;
  it should be fixed in the chunker so the content is complete rather than truncated.
- **Other providers' usage.** Only the OpenAI-compatible and SiliconFlow embedding
  paths report `usage` today; other drivers leave `TokenCount` at 0 and the
  calibration simply gets no observation from them.
- Windows of models with declared limits of 32768/131072 are honoured as declared;
  nothing here validates that a provider accepts that much.

## Verification

```bash
# unit tests for the layer (margin, calibration, limiter, L3 ladder, distribution)
# plus the counter contract properties for every counter whose asset is present
./build.sh --test ./internal/tokenizer/ ./internal/ingestion/component/ \
             ./internal/ingestion/task/ ./internal/entity/models/

# counter fidelity against the models themselves (339 samples x 5 counters, token level)
mkdir -p /tmp/tokenizer_oracle
for m in bge-m3 bge-large-en qwen3-embedding e5-mistral cl100k; do
  uv run scripts/gen_tokenizer_oracle.py --model $m --corpus /path/to/78785.md \
    --out /tmp/tokenizer_oracle/$m.json --fuzz 300
done
TOKENIZER_ORACLE_DIR=/tmp/tokenizer_oracle \
  ./build.sh --test -run TestCountersMatchOracle ./internal/tokenizer/

# optional second opinion for xlmr-spm: Google's sentencepiece on the .model file.
# Expect disagreement on whitespace-shaped samples - see
# "The three-way cross-check (what it is, and how to read it)" above.
uv run scripts/gen_tokenizer_oracle.py --model bge-m3 --corpus /path/to/78785.md \
  --out /tmp/tokenizer_oracle/bge-m3.json --fuzz 300 --spm-crosscheck

# build, then end-to-end: re-ingest a document and check MySQL vs ES
bash build.sh --go
bash scripts/restart_ingestor.sh

# how much of the catalog the tags cover (regenerate the tables in the strategy section)
python3 scripts/tokenizer_coverage.py --markdown

# asset plumbing: download list vs runtime images vs SHA-1 pins
python3 -m pytest ragflow_deps/test_tokenizer_assets.py -q
```

Expected output today: all packages `ok`; the oracle test logs
`agreement: 341/341 samples compared by token texts` for `xlmr-spm`, `bert-wordpiece`,
`qwen-bpe` and `llama-bpe`, `341/341 samples compared by id sequences` for
`cl100k_base`, plus a line naming how many samples were held only to the bounded
combining-mark tolerance, and `counters exercised: [cl100k_base xlmr-spm
bert-wordpiece qwen-bpe llama-bpe]` for the property test. A counter that is skipped
there is missing an asset, not exempt from the contract.

Gate status at the time of writing (2026-09-16), so nobody reads a plan as a result:

| Gate | Status |
|---|---|
| Unit + property + oracle tiers | **observed green** |
| Incident document re-ingested, MySQL `chunk_num` == ES `_count` | **observed** (36 == 36) |
| `covsweep` reporting zero unserved words under the strict criterion | **not yet run** (the earlier sweep was interrupted); command: `covsweep --docs /tmp/cov_docs.tsv --shards 8 --workers 16 --out outputs/cov_after_20260916` |

## Background: the incident

`78785.md` (77 KB of Wikipedia tables) embedded fine until the parser's separator
fix changed how the issue-number tables are rendered. It then failed
deterministically with `400 … {"code":20015,"message":"The parameter is invalid"}`
at the Tokenizer's content-embedding step, and because one bad input failed the
whole batch, the document was left with **0 chunks** — 34 lost.

Measured, on the exact span that failed:

| | tokens |
|---|---|
| `cl100k_base` (what the old guard counted with) | 8,143 → below the 8,182 guard, so **truncation never fired** |
| `bge-m3`'s own tokenizer | **8,237** → above the 8192 window → provider rejects |

The same document now ingests in 2 s: `run=3`, `chunk_num=36`,
`token_num=43581`, ES `_count=36`, and the shrink-and-retry path did **not** have to
fire — the primary path is exact.
