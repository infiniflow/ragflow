---
name: ragflow-go-docs
description: Audit and update RAGFlow Markdown/MDX documentation with Go as the formal user-facing implementation. Use when documenting, reviewing, or aligning Docker, source builds, CLI, services, DeepDoc, MCP, FAQ, or deployment behavior.
---

# RAGFlow Go Documentation

Use this skill for RAGFlow user and developer documentation work. Keep the documentation centered on the Go implementation and write only capabilities that users can use in the documented target configuration.

## Scope and safety

- Modify only `*.md` and `*.mdx` files unless the user explicitly expands the scope.
- Never modify `internal/development.md`.
- Do not modify Go, Python, shell, Dockerfile, Compose, YAML, or other code/configuration files while performing documentation work.
- If code or configuration is wrong, report it separately; do not silently change it as part of a documentation task.
- Preserve unrelated user changes in a dirty worktree.

## Required product stance

- Treat Go as RAGFlow's formal and primary server implementation.
- Do not present Python services, workers, or legacy mixed images as an alternative deployment path.
- Do not explain Go by comparing it with Python in user-facing documentation.
- Do not list unavailable, unfinished, or non-functional features in user-facing documentation. Omit them instead of writing "not implemented", "coming later", or similar development-status language.
- Keep factual operational limits when users need them. For example, state that DeepDoc in the open-source 1.0 release uses CPU inference, without framing it as a development-status comparison.
- Python may appear only when it is an explicit user-facing product (for example, a dedicated Python API reference), a client example, or a required build-time resource-preparation step. Do not describe those cases as the RAGFlow runtime implementation.

## Authority and conflict resolution

Use the following order when deciding what documentation should say:

1. An explicit target behavior specified by the user, including a stated future code alignment.
2. Verified Go runtime behavior and the target deployment contract.
3. Go source parser, dispatcher, implementation, and configuration behavior.
4. Rendered Docker Compose configuration, Dockerfiles, entrypoints, and templates.
5. Existing documentation.

If the user explicitly says that code will be changed to match a target port or interface, document the target value and do not preserve the stale current value. Otherwise, do not infer behavior from comments, help text, or a temporarily disabled entry; verify it against the owning Go code and a safe runtime check.

Project facts that must remain consistent are listed in [ragflow-go-facts.md](references/ragflow-go-facts.md).

## Workflow

### 1. Inspect before editing

- Identify all affected Markdown/MDX files and inspect their current content and local links.
- Check `git status` and avoid overwriting unrelated edits.
- Locate the owning Go parser, dispatcher, implementation, startup script, or configuration before asserting command behavior.

### 2. Audit content

Search for:

- Python-versus-Go implementation comparisons or migration language.
- Legacy Python server, worker, task-executor, or mixed-image startup commands.
- "not implemented", "not supported", "coming later", or equivalent statements about Go features.
- Stale ports, paths, image names, installer URLs, environment variables, and command syntax.
- Commands or parameters present in documentation but absent from the parser/dispatcher or rejected at runtime.
- Inconsistent terminology, defaults, prerequisites, and service ordering across documents.

### 3. Verify claims

- For CLI commands, read the parser, command dispatcher, and implementation. A parser entry alone is not proof that a command works.
- For installers, verify the checked-in script URL, platform naming, checksum behavior, install directory, PATH behavior, and `--version` or `--help` check.
- For Docker instructions, inspect the relevant Compose file and render or inspect the effective configuration when practical.
- Run safe local checks in an isolated environment. Use help, version, parsing, health, and read-only commands; do not run destructive commands against real data.
- Distinguish "syntax/help verified", "local behavior verified", "service end-to-end verified", and "not tested because a required dependency was unavailable".

### 4. Edit for users

- Lead with the supported Go workflow.
- Put Release binaries/installers before source builds for ordinary users; put source builds in a developer section.
- Give complete, copyable commands with prerequisites and expected verification.
- Remove unavailable features and stale alternatives instead of documenting their absence.
- Keep necessary limitations as direct usage constraints.
- Use consistent terminology and target ports in every affected document.

### 5. Review after editing

- Run `git diff --check` on changed documents.
- Re-scan changed and related documents for stale Python/Go comparisons, unavailable-feature wording, old ports, and wrong URLs.
- Confirm every changed file is Markdown or MDX and `internal/development.md` is unchanged.
- Report what was verified, what remains unverified, and any code/configuration issue that was intentionally not changed.

## Writing rules

- Prefer concise, user-facing Chinese or the document's existing language.
- Use `bash` for shell commands, `powershell` for PowerShell, `sql` for SQL-like CLI syntax, and `text` for terminal transcripts.
- Mark required values as `<value>` and optional segments as `[OPTION '<value>']`.
- Do not invent commands, options, ports, paths, outputs, or support claims.
- Do not expose internal function names, command IDs, or migration notes unless the document is explicitly an internal developer reference.

For the fixed ports, installer paths, DeepDoc constraint, and protected-file rule, read [ragflow-go-facts.md](references/ragflow-go-facts.md) before editing deployment or CLI documentation.
