# Feishu Wiki ingestion workflow

## Outcome

Add a first-class `feishu_wiki` data source to RAGFlow so an operator can link
an approved-materials Wiki tree to a dataset, trigger an immediate import, or
schedule incremental imports. Only files that pass the configured screening
rules enter the existing RAGFlow upload, parsing, chunking, and indexing path.

## Scope

- Source: Feishu Wiki file nodes created by the existing approval workflow.
- Root Wiki node: `E1uywCuOGiytfXkdUHdc6PROnMg`.
- Wiki space: `7677869716296813542`.
- Supported source objects in the first release: downloadable `file` nodes.
- Native Feishu Doc/Sheet/Slides export is explicitly deferred.
- Deletion reconciliation is disabled. Missing source files never cause a
  RAGFlow document deletion.
- Screening is rule based (extension and title keywords). A separate human
  per-file approval queue is not part of this release because the upstream
  approval workflow already governs admission into this Wiki tree.

## Trigger and ownership

| Mode | Trigger | Owner | Expected behavior |
| --- | --- | --- | --- |
| Manual | Dataset settings, linked source, rebuild action | Dataset editor | Scan from the root and import every matching current file |
| Automatic | Existing connector scheduler and `refresh_freq` | RAGFlow sync worker | Import matching files changed after the last successful window |
| Validation | Test connection action | Data-source editor | Authenticate and verify that the root Wiki node can be listed |

## Inputs

The connector configuration contains:

- `credentials.app_id`: Feishu custom-app App ID.
- `credentials.app_secret`: Feishu custom-app App Secret; rendered as a
  password field and never written to logs.
- `space_id`: Wiki space ID.
- `root_node_token`: root Wiki node token.
- `include_extensions`: optional allow-list such as `pdf`, `docx`, `xlsx`,
  `pptx`, `jpg`, and `png`. Empty means every RAGFlow-supported file extension.
- `include_keywords`: optional title keywords; when non-empty, at least one
  must match case-insensitively.
- `exclude_keywords`: optional title keywords; any match rejects the node.
- `batch_size`: documents yielded to the RAGFlow sync pipeline per batch.
- `refresh_freq`: existing RAGFlow scheduler interval in minutes.

## Processing

1. Exchange App ID and App Secret for a tenant access token.
2. Recursively list child nodes beneath `root_node_token`, following Feishu
   pagination at every level.
3. Reject non-file nodes as content while still traversing their children.
4. Apply extension, include-keyword, and exclude-keyword filters before any
   file download.
5. For incremental sync, reject nodes whose `obj_edit_time` is not within the
   fixed RAGFlow sync window. A missing or invalid timestamp is included so it
   cannot be silently lost.
6. Download the file by object token.
7. Emit a normalized RAGFlow document with a stable source ID based on the Wiki
   node token, the full SHA-256 in metadata, and a 32-character SHA-256-derived
   fingerprint compatible with RAGFlow's `content_hash` column.
8. Let the existing RAGFlow sink skip identical fingerprints, update changed
   documents, copy metadata, and start parsing when `auto_parse` is enabled.

## Metadata

Each emitted document contains:

- `source=feishu_wiki`
- `wiki_space_id`
- `wiki_node_token`
- `wiki_object_token`
- `wiki_object_type`
- `wiki_parent_node_token`
- `wiki_url`
- `source_updated_at`
- `content_sha256`

## Failure handling

- Authentication, listing, pagination, and download failures are surfaced as
  connector failures and recorded by the existing sync log.
- HTTP 401/403 is treated as an authorization/permission error.
- Feishu API `code != 0` is treated as a connector validation/runtime error.
- Network calls have bounded timeouts.
- A failed task does not advance the successful sync window.
- No source deletion or RAGFlow deletion endpoint is called.

## Security

- The App Secret is accepted only as a password field.
- Request/response logging must never include credentials or bearer tokens.
- The connector calls only the fixed Feishu Open Platform host.
- Required application permissions are read-only Wiki and Drive permissions.

## Acceptance criteria

1. Test connection succeeds for a readable root and fails clearly for invalid
   credentials or an inaccessible space.
2. A manual rebuild imports matching file nodes into a linked dataset.
3. With auto-parse enabled, imported documents enter the normal RAGFlow parse
   path and produce chunks.
4. An automatic run imports a newly added matching file.
5. Re-running without changes does not create a duplicate document.
6. A changed file updates the stable document and its fingerprint.
7. Filtered files are not downloaded or imported.
8. Removing a file from Feishu does not delete it from RAGFlow.
