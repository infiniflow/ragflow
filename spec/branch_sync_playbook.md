# Branch Sync Playbook (three_u_0240)

This file documents the recommended sync workflow for the `three_u_0240` branch
and how Project Management AI should handle conflicts and pushes to Gitee.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                    Official Repository                          │
│            github.com/infiniflow/ragflow                        │
└─────────────────────┬───────────────────────────────────────────┘
                      │ auto-sync (GitHub Actions)
                      ▼
┌─────────────────────────────────────────────────────────────────┐
│                    origin (GitHub Fork)                         │
│            github.com/shaoqing404/ragflow                       │
│            Purpose: Track official updates (READ-ONLY)          │
└─────────────────────┬───────────────────────────────────────────┘
                      │ git fetch origin / git merge origin/main
                      ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Local Workstation                            │
│            Branch: three_u_0240                                 │
│            Purpose: Development & customization workspace       │
└─────────────────────┬───────────────────────────────────────────┘
                      │ git push gitee three_u_0231
                      ▼
┌─────────────────────────────────────────────────────────────────┐
│                    gitee (Deployment Target)                    │
│            gitee.com/GFCM/ragflow                               │
│            Purpose: Distribution to production servers          │
└─────────────────────────────────────────────────────────────────┘
```

## Goals
- Keep `three_u_0240` tracking origin (GitHub fork that syncs official) changes.
- Preserve local customizations (3U MEL parsers, etc.) in `three_u_0240`.
- Push updates **ONLY to gitee**, which handles server deployment.

## Git Remote Configuration
- **origin**: `https://github.com/shaoqing404/ragflow.git`
  - Personal GitHub fork that auto-syncs with official `infiniflow/ragflow`
  - **READ-ONLY** - only used for `git fetch` and `git merge`
  - Never push local changes to origin
- **gitee**: `git@gitee.com:GFCM/ragflow.git`
  - Deployment target repository
  - **WRITE** - all local modifications are pushed here
  - Gitee handles distribution to production servers

## Recommended Workflow (merge-based)
Use merge to keep history clear and reduce rebase risks.

```bash
# 1) Ensure clean working tree
git checkout three_u_0240
git status -sb

# 2) If local changes exist, either commit or stash
# commit:
git add -A
git commit -m "wip: local changes"

# OR stash:
git stash -u

# 3) Bring in official updates from origin (GitHub fork)
git fetch origin
git merge origin/main

# 4) Resolve conflicts if any, then commit the merge
git add <conflict-files>
git commit

# 5) Push to Gitee ONLY (not origin!)
git push gitee three_u_0240
```

## Conflict Resolution Checklist
1) `git status` to list conflicted files.
2) Open each file and resolve markers:
   - `<<<<<<<`
   - `=======`
   - `>>>>>>>`
3) Ensure the result keeps required 3U changes.
4) `git add <file>` for each resolved file.
5) `git commit` to finish the merge.

## Rebase Alternative (linear history, higher risk)
Only use rebase if you need a clean linear history and understand the risks.

```bash
git checkout three_u_0240
git fetch origin
git rebase origin/main

# resolve conflicts
git add <conflict-files>
git rebase --continue

# push updated history to Gitee ONLY
git push gitee three_u_0240 --force-with-lease
```

## Force-Sync to Gitee master (destructive)
Only run when approved. This overwrites `gitee/master` to match
`three_u_0240`.

```bash
git push gitee three_u_0240:master --force
```

## Project Management AI Handoff (role notes)
- Primary branch: `three_u_0240`
- Official source: `origin/main` (GitHub fork that auto-syncs with infiniflow/ragflow)
- Push target: `gitee/three_u_0240` **ONLY** (never push to origin)
- When instructed, force-sync `gitee/master` from `three_u_0240`
- If conflicts arise:
  - Prefer keeping 3U logic and recent local changes.
  - If unclear, request human confirmation before finalizing.

## Minimal Safety Checks
- `git status -sb` before and after merges.
- `git log --oneline -5` after merging to verify the merge commit.
- Never force-push unless explicitly approved.
- Never push to origin (it's read-only for tracking official updates).
 - Note: Quart reloader can trigger `execv` restarts and wrong interpreter; keep `QUART_RUN_RELOAD=0` until upstream addresses it.
