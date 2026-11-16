---
name: react-doctor
description: Use when finishing a feature, fixing a bug, before committing React code, or when the user types `/doctor`, asks to scan, triage, or clean up React diagnostics. Covers lint, accessibility, bundle size, architecture. Includes a regression check and a full local-triage workflow that fetches the canonical playbook.
version: "1.2.0"
---

# React Doctor

Scans React codebases for security, performance, correctness, and architecture issues. Outputs a 0–100 health score.

## After making React code changes:

Run `npx react-doctor@latest --verbose --scope changed` and check the score did not regress.

If the score dropped, fix the regressions before committing.

## For general cleanup or code improvement:

Run `npx react-doctor@latest --verbose` (the default `--scope full`) to scan the full codebase. Fix issues by severity — errors first, then warnings.

## For a focused UI design audit:

Run `npx react-doctor@latest design --verbose`. This selects only design-tagged UI composition, typography, interaction, accessibility, and motion rules, including focused rules that remain opt-in during a general health scan.

## For runtime performance problems:

Run `npx react-doctor@latest scan <url> --format json` in an interactive terminal. React Doctor opens an isolated system Chrome profile, records a DevTools trace while the user reproduces the slow interaction, and flashes purple outlines with component names as React renders. It stops when they press Enter. Read the structured summary first, then inspect the returned local `.json.gz` trace for CPU, browser, and React component evidence.

If the user needs their authenticated browser state, use `--cdp <remote-debugging-url>`. This requires Chrome to already be running with remote debugging. Never ask for cookies or copy the user's browser profile. Treat the trace as sensitive local application data and never upload it without explicit permission.

## /doctor — full local triage workflow

When the user types `/doctor`, says "run react doctor", or asks for a full triage / cleanup pass (not just a regression check), fetch the canonical local-triage playbook and follow every step in it:

```bash
curl --fail --silent --show-error \
  --header 'Cache-Control: no-cache' \
  https://www.react.doctor/prompts/react-doctor-agent.md
```

The playbook is the single source of truth — a scan → filter → triage → fix → validate loop that edits the working tree directly (never commits, never opens PRs). Updating the prompt at its source updates every agent on its next fetch — no skill reinstall needed.

Pair it with the matching per-rule prompts at `https://www.react.doctor/prompts/rules/<plugin>/<rule>.md` (fetched on demand inside the playbook) so each fix uses the canonical, reviewer-tested recipe.

Use `--scope contextual --base origin/main` when a team wants existing findings
in the code it is editing. Rules can select an element, function, file, or an
explicit set of related nodes. For `exhaustive-deps`, edits in the hook call or a
resolved local callback can surface a finding on an unchanged dependency array.
Rules without wider ownership keep changed-line filtering.

## Configuring or explaining rules

When the user wants to understand a rule, disagrees with one, or wants to disable / tune which rules run (not fix code), read [references/explain.md](references/explain.md) and follow it. Start with `npx react-doctor@latest rules explain <rule>`, then apply the narrowest control via `npx react-doctor@latest rules disable|set|category|ignore-tag …`, which edits your `doctor.config.*` (or `package.json#reactDoctor`).

## Command

```bash
npx react-doctor@latest --verbose --scope changed
```

| Flag                 | Purpose                                                                                 |
| -------------------- | --------------------------------------------------------------------------------------- |
| `.`                  | Scan current directory                                                                  |
| `--verbose`          | Show affected files and line numbers per rule                                           |
| `--scope changed`    | Only report issues introduced vs the base branch (default: full)                        |
| `--scope contextual` | Report findings in modified functions, JSX elements, or files, as selected by each rule |
| `--scope lines`      | Only report issues on the changed lines                                                 |
| `--score`            | Output only the numeric score                                                           |
| `design`             | Run only the focused UI design diagnostics                                              |

### Reuse a base scan

Save a complete `--json` scan, then pass `--baseline base-report.json` to report only new findings. This also works with `--scope changed`. Use the same version and rules for both scans. Findings are grouped by rule and file path, following Git renames. Only increases in each group's count are reported; groups without an increase skip location matching. Fingerprints, message similarity, and Git-adjusted line distance select the added findings. Formatting and edits that keep the same counts report no new findings. Saved reports must use the same `textComponents`, `rawTextWrapperComponents`, and `respectInlineDisables` settings. Regenerate the base report after changing these settings, or use a Git comparison. Older reports without fingerprints or `sourceFilterConfigHash` must be regenerated.

### GitLab warning status

For GitLab CI, `--warning-exit-code 123` returns `123` for non-blocking warnings. Pair it with `allow_failure: { exit_codes: 123 }`. Errors still return `1`. Accepts integers from `2` to `255`. `--blocking warning` still returns `1`. `--blocking none` or `--score` skips this warning signal.
