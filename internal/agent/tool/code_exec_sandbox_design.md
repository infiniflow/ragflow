# CodeExec sandbox — Phase 5d design decision

Status: **decision recorded, implementation pending**. This file captures
the trade-offs so the choice can be revisited without re-deriving it.

## Context (what the Python side actually does)

The Python agent's code_exec delegates to a **provider-based
sandbox subsystem** under `agent/sandbox/`. It is NOT a single
SDK and NOT a local subprocess — it is a thin abstraction over
several execution backends.

### Provider interface (`agent/sandbox/providers/base.py`)

```python
class SandboxProvider(ABC):
    def initialize(self, config) -> bool
    def create_instance(self, template: str) -> SandboxInstance
    def execute_code(self, instance_id, code, language,
                     timeout=10, arguments=None) -> ExecutionResult
    def destroy_instance(self, instance_id) -> bool
    def health_check(self) -> bool
    def get_supported_languages(self) -> List[str]
```

### Providers shipped in the Python repo

| Provider | File | Backend |
|----------|------|---------|
| `SelfManagedProvider` | `self_managed.py` | HTTP at `localhost:9385` (the `executor_manager`, which runs a Docker pool with gVisor) |
| `AliyunCodeInterpreterProvider` | `aliyun_codeinterpreter.py` | Alibaba Cloud sandbox (uses `agentrun` SDK / Function Compute) |
| `E2BProvider` | `e2b.py` | e2b cloud sandbox (SaaS) |

`ProviderManager` (`manager.py`) selects one provider at startup
based on configuration; the CodeExec tool talks only to the
manager, never to a specific provider.

### Subprocess flow on the Python side

A CodeExec call goes through `agent/sandbox/client.execute_code(...)`
which is the public entry point the CodeExec component uses
(`agent/tools/code_exec.py:365`):

```python
from agent.sandbox.client import execute_code as sandbox_execute_code
result = sandbox_execute_code(
    code=code, language=language,
    timeout=timeout_seconds, arguments=arguments,
)
```

That function:
1. Resolves the active provider via `ProviderManager` (which reads
   `SystemSettingsService.get_by_name("sandbox.provider_type")` —
   i.e. the choice is driven by the system admin panel, not the
   caller).
2. Calls `provider.create_instance(template=language)` →
   `provider.execute_code(...)` → `provider.destroy_instance(...)`.

So the CodeExec component **does** support all three providers —
the provider choice is invisible to it. If the provider system
is not configured, the CodeExec component falls back to a direct
HTTP POST to `http://{SANDBOX_HOST}:9385/run` (the executor_manager
endpoint) for backward compatibility. Both paths land in the
same `_process_execution_result` handler.

## Options for the Go port

### A. Shell out to a Python subprocess

Go spawns `python3 -c "..."` that:
- Imports `agent.sandbox.providers.ProviderManager`
- Picks up the same configuration the Python agent uses
- Returns the ExecutionResult over stdout (JSON)

Pros
: Reuses the full provider surface (self-managed + Aliyun + e2b).
  A single Python subprocess call covers all three.
: Plan §2.11.4 ("don't rewrite the sandbox") honored literally.
: Operators that already deploy Python RAGFlow have the
  provider configuration in place; Go inherits it for free.

Cons
: Per-call latency = Python interpreter startup + provider import
  + dispatch. ~hundreds of ms for the first call, similar for
  subsequent calls (no interpreter caching yet).
: Adds a Python dependency on the Go host.
: Pipe / stdout JSON serialisation is awkward for binary output
  (matplotlib plots, files written to the sandbox). Can be
  mitigated with file-based handoff for large payloads, but adds
  operational complexity.

### B. Reimplement the ProviderManager in Go

Read the three provider implementations and write Go equivalents:

- `SelfManagedProvider` → Go HTTP client to `localhost:9385`
  (the executor_manager). Smallest of the three.
- `AliyunCodeInterpreterProvider` → Go reimplementation of the
  `agentrun` SDK client. ~Vendor SDK surface to maintain.
- `E2BProvider` → Go reimplementation of the e2b SDK client.

Pros
: No Python dependency on the Go side.
: Lower per-call latency.
: Clean integration with the rest of the Go agent runtime.

Cons
: Three SDK / API surfaces to maintain in parallel with the
  Python ones. Every vendor release requires Go updates.
: Plan §2.11.4's intent is to avoid duplicating sandbox logic;
  reimplementing three providers arguably violates the spirit
  even if it doesn't violate the letter.
: The `agentrun` and `e2b` SDKs include auth, retry, pagination,
  and connection management — real ongoing work.

## Decision

**Option A (shell out to a Python subprocess that uses
`ProviderManager`)**. Reasoning:

1. The Python-side flow already supports all three providers via
   a single entry point. The Go port's job is the orchestrator +
   agent runtime, not duplicating three vendor SDKs.
2. The latency cost is real but acceptable — CodeExec is called
   sparingly (a script per LLM turn at most).
3. Plan §2.11.4 commits to NOT rewriting the sandbox. Option B
   pushes against that intent; option A doesn't.

The Python subprocess must go through `ProviderManager`, not
directly to any one provider, so configuration stays in one place.
**"Shell out to system python3 directly"** (without the agentrun
SDK or any sandbox) is NOT a valid implementation — it would
execute user-supplied code with the agent process's privileges,
violating the security model.

## Implementation sketch

1. Add a `PythonProviderManagerClient` implementing
   `SandboxClient` (`tool/code_exec_client.go`).
2. The subprocess command mirrors the Python CodeExec flow:

   ```go
   cmd := exec.CommandContext(ctx, "python3", "-c", `
   import json, sys
   from agent.sandbox.client import execute_code
   result = execute_code(
       code=sys.argv[1],
       language=sys.argv[2],
       timeout=int(sys.argv[3]),
       arguments=json.loads(sys.argv[4]) if sys.argv[4] else None,
   )
   json.dump({
       "stdout": result.stdout,
       "stderr": result.stderr,
       "returned": "",  # provider returns stdout; no REPL value
       "artifacts": (result.metadata or {}).get("artifacts", []),
   }, sys.stdout)
   `)
   cmd.Args = append(cmd.Args, code, language,
                     strconv.Itoa(timeout), argsJSON)
   ```

3. Parse the JSON response and map to `SandboxResponse`.

4. Add config knobs:
   - `code_exec_python_bin` (default `python3`)
   - `code_exec_provider_type` (read by Python — let the admin
     panel set `sandbox.provider_type` as today)

**Capability parity note**: by going through
`agent.sandbox.client.execute_code`, the Go CodeExec tool inherits
all three providers (self_managed / aliyun_codeinterpreter /
e2b) for the cost of one Python subprocess call. The provider
choice happens inside Python based on `SystemSettingsService`,
invisible to the Go side. This matches what the Python
`agent/tools/code_exec.py` does today (lines 358-381 of that
file).

## What this file is not

This is not an implementation task. It records the agreed-upon
direction so any future contributor (or this file's author, six
months from now) doesn't accidentally land a "shell out to
system python3" stub that bypasses the sandbox. If you find
yourself writing `exec.CommandContext("python3", "-c", ...)`
without going through `ProviderManager`, **stop** — you're
working against the plan and the security model.
