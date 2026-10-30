# PR ASCII check

`ragflow_preflight` rejects non-ASCII bytes on added lines in a PR's diff
from its merge base. Unchanged lines and deleted lines do not fail the check.
The preflight keeps its existing `ci` label requirement for non-draft PRs,
including documentation-only changes.
This is an ASCII rule, so it also rejects smart quotes, accented letters,
emoji, and typographic dashes. Use ASCII punctuation and Unicode escapes
for required string literals in application source code.

Run locally from the repository root:

```sh
go run ./tools/checkascii origin/main HEAD
bash build.sh --test ./tools/checkascii
```

The checker reads committed content rather than working-tree files. Failures
produce GitHub Actions annotations with the file and new line number. Invalid
Git references and other Git errors fail the check. Git binary patches are
skipped. Renamed files are treated as additions, including moves out of
excluded paths.

Exclusions:

- Go `*_test.go` files.
- Frontend JavaScript/TypeScript `*.test.*` and `*.spec.*` files under `web/`,
  files in `web/**/__tests__/` or `web/**/__mocks__/`, and `web/jest-setup.ts`.
- `web/src/locales/*.ts`, except `web/src/locales/en.ts`.
- `README_*.md` directly in the repository root.
- `internal/agent/templates/*.json`: localized agent names and descriptions.
- `internal/rag/res/ner.json` and `internal/rag/res/synonym.json`: multilingual
  entity and synonym dictionaries.
- Data files under a `testdata`, `fixtures`, or `golden` directory with extension
  `.json`, `.txt`, `.md`, `.html`, `.csv`, `.tsv`, `.xml`, `.svg`, or `.b64`:
  multilingual parser inputs, DSL examples, and expected results.

Other source files remain checked, including fixture generators. English
documentation, prompts, and regular application code have no blanket exclusion.
