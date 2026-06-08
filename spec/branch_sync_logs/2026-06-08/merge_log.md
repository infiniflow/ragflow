# Merge Log - 2026-06-08

- **Branch**: worktree-sync-2026-06-08 (ragflow proxy, 准备推 gitee)
- **Source**: upstream/main (infiniflow/ragflow @ 4bbd59823)
- **Base**: origin/main @ 074c331cd (shaoqing404/ragflow, READ-ONLY fork)
- **Method**: `git merge upstream/main --no-ff` on a fresh worktree at origin/main
- **Upstream HEAD at merge time**: 4bbd59823 — Addd OpenRouter OpenAI API compatible list models (#15764)
- **Upstream commits absorbed (this round)**: 17
- **Merge commit SHA**: b221625cb51e2dbbbc8aac5ec5e4d6e92b9d3a5a
- **Localization commit SHA**: b58e48e5c6444b6d2e488f87e6a22bb473c88e6b (docs: re-apply boot.md for gitee proxy workflow)
- **Conflicts**: none (17 commit delta merged cleanly into origin/main base)

## Conflict resolution
N/A — no conflict markers produced. The base was a clean origin/main, so the 17 upstream commits applied without overlap.

## 3U critical files verification
N/A — this round is a clean upstream merge; no prior 3U customizations on the base. The previously documented 3U markers (by_three_u, 3u_mel, page_number logic) only existed on the deprecated `three_u_0240` branch and were not carried into `ragflow0251/gitee-export`.

## Sensitive config
- Scanned 17 upstream commits (full diff: 19,843 lines across 121 files) for hardcoded API keys / secrets:
  AWS AKIA/ASIA, GCP AIza, Stripe sk_live_/pk_live_/rk_live_, GitHub PATs (ghp_/gho_/ghu_/ghs_/ghr_), OpenAI/Anthropic sk- / sk-ant-, private-key BEGIN blocks, Azure DefaultEndpointsProtocol connection strings, long base64/hex literals in *_KEY/*_SECRET/*_TOKEN context.
  **Result: zero hardcoded secrets introduced.** Only hits were:
  - Go test fixtures: `apiKey := "test-key"` in `internal/entity/models/xiaomi_test.go` and sibling test files
  - Runtime parameter plumbing: `f"Bearer {key}"` in `rag/llm/rerank_model.py` and `req.Header.Set("api-key", apiKey)` in `internal/entity/models/xiaomi.go`
  - Conf metadata: `conf/llm_factories.json` and `conf/models/xiaomi.json` (model registration only)
- `docker/.env` and `conf/service_conf.yaml` — not modified, not in this commit.

## Notable upstream changes
- `conf/models/xiaomi.json` (new) — Xiaomi model provider registration
- `internal/entity/models/xiaomi.go` + `xiaomi_test.go` — Xiaomi driver
- `internal/common/{json_types,libm_cgo,libm_purego}.go` — Go runtime helpers
- `internal/engine/{elasticsearch,infinity}/meta_filter.go` + tests — meta filtering on both backends
- `test/testcases/restful_api/test_search_datasets_consistency.py` (new)
- `test/unit_test/rag/llm/test_rerank_normalization.py` (new)
- Various Go API additions: `/api/v1/agents/<id>/sessions`, OpenRouter OpenAI list models, BM25 hybrid search fix, plugin handler/service
- 121 files changed, 14356 insertions(+), 1888 deletions(-)

## Notes
- Worktree: `/Users/mac/Developer/element_workspace/ragflow/.claude/worktrees/sync-2026-06-08` (branch: worktree-sync-2026-06-08)
- origin is READ-ONLY this round — no push to shaoqing404/ragflow
- gitee push is PENDING USER APPROVAL per the historical playbook gate
