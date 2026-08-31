# Chunker Python↔Go parity fixtures

Python (`rag/flow/chunker/`) is the reference implementation. Every case here is
run through both languages, and `golden_parity_test.go` asserts the Go port
reproduces what Python produced.

Chunking is deterministic — the same input always yields the same output — which
is what makes recorded baselines a valid oracle rather than a flaky snapshot.

## Layout

| Path | Contents |
|---|---|
| `cases/<id>.json` | Input fixture. Read verbatim by **both** languages. |
| `golden/<id>.json` | What Python produced for that case. |
| `known_diffs.json` | Accepted, documented divergences (see below). |
| `_manifest.json` | Capture provenance: commit, Python version, timestamp. |

The case filename stem must equal the `id` field; both the capture script and
the Go test enforce this.

## Why one input works for both languages

The Python upstream models (`rag/flow/chunker/schema.py`) declare
`populate_by_name=True` with aliases `json` / `markdown` / `text` / `html` —
the same short names the Go schema already uses as struct tags. Param keys line
up too: Python attribute names equal the Go json tags. So no translation layer
is needed, and there is no risk of the two sides drifting to different inputs.

Always write the short alias form (`"markdown"`, not `"markdown_result"`): only
the alias is accepted on both sides.

## Adding a case

1. Create `cases/<id>.json`:

   ```json
   {
     "id": "<id>",
     "component": "TokenChunker",
     "notes": "one line on what behavior this case locks",
     "param":  { },
     "input":  { "name": "t", "output_format": "json", "json": [] }
   }
   ```

   `component` is the registry name: `TokenChunker`, `TitleChunker`,
   `GroupTitleChunker`, or `HierarchyTitleChunker`.

2. Capture the Python baseline and run the Go side:

   ```bash
   source .venv/bin/activate && export PYTHONPATH=$(pwd)
   python internal/ingestion/component/chunker/tool-py/capture_golden.py --case <id>
   CGO_ENABLED=0 go test -count=1 ./internal/ingestion/component/chunker/ -run TestChunkerGoldenParity
   ```

3. If the case fails, that is a real divergence. Either fix the Go side or add a
   rule to `known_diffs.json` explaining why the difference stands.

## Regenerating after an intentional Python change

```bash
source .venv/bin/activate && export PYTHONPATH=$(pwd)
python internal/ingestion/component/chunker/tool-py/capture_golden.py --all
```

Check for stale baselines without writing anything:

```bash
python internal/ingestion/component/chunker/tool-py/capture_golden.py --all --check
```

Commit a regeneration on its own, with no unrelated changes — otherwise the
diff is too large to review by eye, which defeats the purpose of a baseline.

## known_diffs.json

A difference is either fixed or written down; the test never tolerates one
silently. Each rule carries:

| Field | Meaning |
|---|---|
| `id` | Stable rule name. |
| `tag` | `python_bug`, `go_bug`, `go_intentional`, or `unresolved`. |
| `kind` | Currently `extra_fields`. |
| `applies_to` | Case-id patterns (`path.Match` globs, e.g. `token__*`, `*`). |
| `fields` | For `extra_fields`: the Go-only chunk keys this rule permits. |
| `reason` | Why the difference exists, with file:line references. |
| `permanent` | `true` if the difference is by design and will not be fixed. |
| `owner_fix_side` | Which side should change: `python`, `go`, or `tbd`. |
| `tracking` | Issue link. Required unless `permanent` is true. |

Field comparison is deliberately asymmetric:

- a key Python emits that Go does not is **always** a failure — Go dropped data;
- a key Go emits that Python does not must be declared here, so every Go-only
  field is justified rather than assumed harmless.

The allowed set is derived per case rather than hardcoded, because it is not
constant across components: `TokenChunker` emits `ck_type`/`tk_nums`/`id` while
`TitleChunker` emits only `id`.

## No CGO required

The parity test is unit tier (no build tag) and passes under `CGO_ENABLED=0`.
The chunker's only native-backed file is `pdfcrop_cgo.go`, whose
`cropImageChunks` returns chunks untouched when the PDF engine is nil — which is
always the case here, since the harness passes no `*gorm.DB` and so no engine is
ever resolved. Skipping CGO therefore costs no coverage; the one field it cannot
populate is `ChunkDoc.Image`, a base64 preview.

Python has the matching seam: `restore_pdf_text_previews`
(`token_chunker.py:370`) runs just before `_finalize_json_chunks`, so image work
sits at the very end on both sides and the text-splitting logic under test
completes before it.
