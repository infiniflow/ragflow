# Project instructions for Copilot

Read [AGENTS.md](../AGENTS.md) before changing this repository. It is the shared operating guide for architecture, upstream preservation, code ownership and validation commands.

Use the [transition plan](../docs/develop/architecture-transition-ru.md) for implementation stages and developer/reviewer instructions, and the [rule catalog](../docs/develop/architecture-checks-ru.md) to select checks. New architecture runners and registries described there are planned until implemented; never report a planned check as passing.

The actual stack is Python/Quart under `api`, `rag`, `agent`, React/TypeScript/Vite under `web`, and Go under `cmd`/`internal`. Preserve the upstream structure, isolate owned business logic, and keep changes to standard code justified and local.

Use repository commands and prepared environments. Python checks are targeted pytest/Ruff; frontend commands come from `web/package.json`; Go tests use `build.sh` with its native dependencies. See [REGRESSION.md](../test/REGRESSION.md) for the existing lanes and their environment requirements. Do not substitute a generic app layout or run live suites against shared data.
