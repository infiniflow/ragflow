# PR Strategy Notes (Concise)

- Five independent PRs; none depend on each other.
- Submit order (low risk first):
  1) PostgreSQL DB auto-create (feature, high value, low risk)
  2) Migration logging noise fix (log-level only)
  3) Docling install caching (optional deps, perf only)
  4) Connection-layer modularization (pool/locks/diag/txn split; medium risk)
  5) Extract DatabaseCompat to api/db/compat.py (small refactor)
- Each PR: include Motivation → Changes → Testing → References → Notes for reviewers.
- Comms: mention alignment with AGENTS.md modularization; call out follow-up PRs.
- Testing per PR: auto-create verified on fresh Postgres; logging fix shows INFO not CRITICAL; Docling cache persists across restarts; connection refactor passes full suite; DatabaseCompat extraction leaves migrations working.
- Success criteria: minimum = auto-create accepted; good = +logging +Docling; excellent = all five.

# Notes pulled from Database Improvements Plan
- Phases in plan map to PRs: error-handling standardization → pool diagnostics → migration tracking → compatibility layer → transaction safety; plus integration tests and docs.
- Migration tracking: add migration_history table, record status/duration/error, skip reruns.
- Pool diagnostics: log utilization thresholds, warn at ~80%, critical at ~95%; optional background monitor.
- Transaction safety: wrap migrations in DB.atomic(); log rollback/commit state.
- Docs to include: migration guide, pool monitoring, database compatibility matrix.
