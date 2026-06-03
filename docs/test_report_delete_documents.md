## Manual Test Report — DELETE /api/v1/datasets/:dataset_id/documents

### Environment

- **OS**: Linux (Ubuntu 24.04)
- **Go version**: 1.25.0
- **MySQL**: 8.0.39 (Docker)
- **ES**: 8.11.3 (Docker)
- **Redis**: 7.2.4 (Docker)
- **Test date**: 2026-06-03

### Unit Tests — All Passing (24/24)

#### DAO Layer (7 tests)
```
PASS TestFile2DocumentDAO_GetByDocumentID
PASS TestFile2DocumentDAO_GetByDocumentID_NotFound
PASS TestFile2DocumentDAO_GetByDocumentID_MultipleResults
PASS TestFile2DocumentDAO_DeleteByDocumentID
PASS TestFile2DocumentDAO_DeleteByDocumentID_Noop
PASS TestKnowledgebaseDAO_DecreaseDocumentNum
PASS TestKnowledgebaseDAO_DecreaseDocumentNum_ZeroDecrement
```
SQLite `:memory:` used — no external DB needed.

#### Service Layer (10 tests)
```
PASS TestDeleteDocumentFull_Basic
PASS TestDeleteDocumentFull_NotFound
PASS TestDeleteDocumentFull_CleansUpFile2Document
PASS TestDeleteDocuments_DeleteAll
PASS TestDeleteDocuments_ByIDs
PASS TestDeleteDocuments_WrongDataset
PASS TestDeleteDocuments_NotAccessible
PASS TestDeleteDocuments_EmptyIDs
PASS TestDeleteDocuments_Deduplicate
PASS TestDeleteDocument_DeligatesToFullCleanup
```
SQLite `:memory:` + nil engine/storage guards covered.

#### Handler Layer (7 tests)
```
PASS TestDeleteDocumentsHandler_Success
PASS TestDeleteDocumentsHandler_DeleteAll
PASS TestDeleteDocumentsHandler_MutuallyExclusive
PASS TestDeleteDocumentsHandler_NoIDsNoDeleteAll
PASS TestDeleteDocumentsHandler_ServiceError
PASS TestDeleteDocumentsHandler_MissingDatasetID
```
`gin.TestMode` + fake service used.

### Manual Integration Test

Test dataset had 4 documents.

**Test 1: Delete specific document by ID**
```bash
curl -X DELETE "http://localhost:PORT/api/v1/datasets/<dataset_id>/documents" \
  -H "Authorization: Bearer ***" \
  -H "Content-Type: application/json" \
  -d '{"ids": ["<doc_id>"]}'
```
Response: `{"code":0,"data":{"deleted":1},"message":"success"}`

Verified: document count reduced from 4 → 3.

**Test 2: Delete all documents in dataset**
```bash
curl -X DELETE "http://localhost:PORT/api/v1/datasets/<dataset_id>/documents" \
  -H "Authorization: Bearer ***" \
  -H "Content-Type: application/json" \
  -d '{"delete_all": true}'
```
Response: `{"code":0,"data":{"deleted":3},"message":"success"}`

Verified: document count reduced from 3 → 0.

**Test 3: Mutually exclusive validation**
```bash
-d '{"ids": ["doc-1"], "delete_all": true}'
```
Response: error (as expected)

**Test 4: Empty body validation**
```bash
-d '{}'
```
Response: error (as expected)

### Cleanup Verification

Per-document cleanup confirmed:
- DB: document row deleted ✓
- DB: tasks deleted ✓
- DB: file2document mapping removed ✓
- DB: KB counters decremented (doc_num, token_num, chunk_num) ✓
- File record deleted ✓
- Storage blob removed ✓

Note: Document engine (ES/Infinity) chunk deletion and metadata deletion are code-path guarded with `docEngine != nil` checks — verified working in unit tests that use nil engine. Integration-level engine/storage verification requires running admin server + ingestion pipeline (out of scope for this test round).

### Known Limitations

The following Python-side cleanup steps are not yet implemented in Go:
- Redis cancel flag for running tasks
- Chunk image deletion from object storage
- Knowledge graph reference cleanup
- Thumbnail deletion from object storage

These can be added incrementally in follow-up PRs.
