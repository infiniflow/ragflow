# TTS engine — Phase 8b design decision

Status: **decision recorded, implementation pending**. The TTS
scaffold exists (internal/agent/audio/tts.go); this doc records
how a real engine should be wired in. The current scaffold's
shellSynthesizer uses an invented `--engine --text --voice --lang`
protocol that no real TTS binary matches — it must not be used
as-is.

## Context (what the Python side actually does)

The Python `agent/canvas.py:518-521` does NOT use `gtts` or
`edge-tts` shell-out. It looks up the tenant's default TTS model
via `get_tenant_default_model_by_type(self._tenant_id, LLMType.TTS)`
and creates an `LLMBundle(tenant, tts_model_config)`. The TTS
factory in `rag/llm/tts_model.py` dispatches to one of several
**HTTP-based** providers:

| Provider | Backend | Pure Go? |
|----------|---------|----------|
| FishAudioTTS | HTTP POST to `api.fish.audio/v1/tts` (msgpack body) | feasible |
| QwenTTS | DashScope SDK (over WebSocket) | heavy |
| OpenAITTS | HTTP POST to OpenAI-compatible `/audio/tts` | feasible |
| StepFunTTS | HTTP POST to vendor endpoint | feasible |
| RAGconTTS | LiteLLM proxy (HTTP) | feasible |
| XinferenceTTS | HTTP POST to xinference | feasible |
| TongyiTTS | DashScope SDK | heavy |

None of the production providers are gtts / edge-tts. The "gtts or
edge-tts shell-out" wording in the original plan was a placeholder
that didn't survive the v3.1 review; the production TTS layer is
HTTP / SDK all the way down.

## Options for the Go port

### A. Reimplement the HTTP clients in Go

Write Go HTTP clients for each provider (Fish / OpenAI / StepFun
/ Xinference / LiteLLM-proxy). Skip the DashScope SDK variants
for now (Qwen, Tongyi) — those are websocket-heavy and can
follow in a later phase.

Pros
: No Python dependency on the Go side.
: Lower latency, no subprocess overhead per call.
: Clean integration with the rest of the Go runtime.

Cons
: Five HTTP surfaces to maintain in lockstep with the Python
  ones. Every vendor release needs a Go update.
: The Python TTS layer is part of a multi-tenant
  `LLMBundle` abstraction (config, key rotation, retry
  policy) — reimplementing just the wire layer loses those
  cross-cutting concerns.

### B. Shell out to a Python subprocess that uses rag.llm.tts_model

Spawn `python3 -c "from rag.llm.tts_model import ...; ..."` and
pipe the audio bytes back.

Pros
: Reuses the verified Python TTS layer verbatim — including
  all providers, the LLMBundle config / key handling, and the
  retry / streaming logic.
: Plan §2.11.4-style "don't rewrite the vendor layer" applies
  here too: rag/llm/tts_model.py IS the vendor layer.
: One Python subprocess call covers all current providers.

Cons
: Per-call latency = Python interpreter startup + TTS module
  import + HTTP to vendor. ~hundreds of ms.
: Adds a Python dependency on the Go host.
: Stream chunks back from Python are awkward (binary audio).

### C. Hybrid — reimplement the simple HTTP ones, shell out for the rest

Reimplement OpenAI / Fish / Xinference / StepFun / LiteLLM-proxy
(5 providers, all straightforward HTTP). Shell out only for the
DashScope-SDK providers (Qwen, Tongyi) — those are websocket
clients whose cost / benefit doesn't justify a reimplementation
until the Go side has more DashScope users.

Pros
: Common case (OpenAI-compatible / LiteLLM proxy) has no
  Python dep.
: Avoids the worst of option B (latency + Python dep) for
  the providers most users actually deploy.

Cons
: Two code paths to maintain (Go HTTP + Python SDK).
: DashScope providers are exactly the ones several Chinese
  RAGFlow operators use — leaving them on the Python
  fallback is a real capability gap.

## Decision

**Option A (reimplement the HTTP clients in Go).** Reasoning:

1. All non-DashScope providers are straightforward HTTP POSTs
   with JSON / msgpack bodies. None of them need a streaming
   audio reader that's complex enough to justify a Python
   subprocess call.
2. The Go `rag/llm` package already has the same factory
   pattern for chat / embedding / rerank models. TTS slots
   into the same plumbing.
3. Option B's latency cost (~hundreds of ms for a Python
   interpreter + module import) is wasted — audio synthesis
   takes seconds anyway; the Python-startup overhead is
   noise compared to the network round-trip.
4. Option C's hybrid is the worst of both worlds: a new
   Go codebase that has to keep parity with a Python fallback
   for the DashScope providers.

For the DashScope SDK providers (Qwen, Tongyi), we accept
"not yet supported in Go" — the user can fall back to the
Python Canvas. Loud-fail via the existing
`ErrTTSEngineNotConfigured` pattern.

## Implementation sketch

1. New file `internal/rag/llm/tts_model.go` (Go side) with:
   - `TTSModel` interface: `Synthesize(ctx, text) (io.Reader, error)`
   - `FishTTS`, `OpenAITTS`, `XinferenceTTS`, `StepFunTTS`,
     `LiteLLMProxyTTS` implementations.
   - `TTSFactory` registry mirroring the Python one.
2. Wire `audio.Synthesizer` in the existing scaffold to
   delegate to `rag/llm/tts_model` (replacing the invented
   shell-out protocol in the current scaffold).
3. New file `internal/rag/llm/tts_model_test.go` with HTTP
   mock tests for each provider's request shape.

## What the existing scaffold needs to change

The current `shellSynthesizer.Synthesize` uses
`--engine --text --voice --lang` argv — no real TTS binary
matches that. Until option A lands, the existing scaffold
should be considered a placeholder. A subsequent commit
should either:
- (a) delete `shellSynthesizer` and `InstallShellSynthesizer`,
      leaving only the stub that returns
      `ErrTTSEngineNotConfigured`, or
- (b) replace them with a thin Python-subprocess client
      using the proven `rag.llm.tts_model` entry point (a
      safe interim — same pattern as CodeExec).

Option (a) is the safer default: it removes a footgun
(a non-functional shell-out) and lets the operator
discover the deferred state through the standard
"ErrTTSEngineNotConfigured" error. Option (b) is the
"ship a working path today" choice.

The choice between (a) and (b) is left to the implementer
of option A; until then, the scaffold's `shellSynthesizer`
is dead code.
