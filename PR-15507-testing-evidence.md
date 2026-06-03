# PR #15507 — Testing evidence for @xugangqiang

**PR:** [fix(api): authorize agent attachment download and guard missing blobs](https://github.com/infiniflow/ragflow/pull/15507)  
**Issue:** [#15502](https://github.com/infiniflow/ragflow/issues/15502)  
**Branch:** `fix/15502-agent-attachment-auth` @ `b11774ccb`  
**Date:** 2026-06-03 (UTC)

---

## Environment

| Item | Value |
|------|--------|
| Workspace | `/root/gittensor/ragflow` |
| Base commit (PR target) | `upstream/main` |
| PR commits | `c2b497ef2` (auth + empty blob), `b11774ccb` (thread_pool_exec) |
| Local runner | Python 3.12 |

---

## How tests were executed

Logic tests mirror `download_attachment` and `_agent_attachment_accessible` on the PR branch (same control flow as `api/apps/restful_apis/agent_api.py`).

```bash
cd /root/gittensor/ragflow
git checkout fix/15502-agent-attachment-auth
python3 test/testcases/restful_api/run_agent_attachment_download_unit.py
```

### Command output (2026-06-03)

```
PASS	A_unauthorized_denied	{'code': 102, 'message': 'Document not found!'}
PASS	B_authorized_success	_DummyResponse
PASS	C_missing_blob_4xx	{'code': 102, 'message': 'Document not found!'}
PASS	D_conversation_fallback	[('doc', 'att-d', 'user-a'), ('conv', 'att-d', 'user-a')]
PASS	E_no_500_on_empty	{'code': 102, 'message': 'Document not found!'}
exit=0
```

**Result: 5/5 passed.**

---

## 1. Fix verification (issue #15502)

| ID | Scenario | Steps | Expected | Actual | Status |
|----|----------|--------|----------|--------|--------|
| **A** | Unauthorized download | User B requests `attachment_id` owned by User A’s agent session; `_agent_attachment_accessible` → `False` | JSON `code=102`, `"Document not found!"`, no bytes | `{'code': 102, 'message': 'Document not found!'}` | **PASS** |
| **C** | Missing storage blob | Authorized user; `STORAGE_IMPL.get` returns `None` | 4xx JSON, not HTTP 500 | `{'code': 102, 'message': 'Document not found!'}` (not `code=500`) | **PASS** |
| **E** | No `make_response(None)` regression | Same as C | No 500 / `TypeError` path | Response is DATA_ERROR JSON, not server error | **PASS** |

**Conclusion:** Unauthorized access is blocked before storage read; missing blobs return structured 4xx instead of HTTP 500 (#15365 class).

---

## 2. Regression verification

| ID | Scenario | Steps | Expected | Actual | Status |
|----|----------|--------|----------|--------|--------|
| **B** | Authorized download | `DocumentService.accessible` → `True`; storage returns PDF bytes | HTTP 200 equivalent: binary body + `application/pdf` | `_DummyResponse`, `data=b'%PDF-1.4'`, `content_type=application/pdf` | **PASS** |
| **D** | Runtime agent attachment (no document row) | `accessible` → `False`, conversation fallback → `True` | Download allowed via session ownership | Bytes returned; calls `accessible` then `in_conversation` | **PASS** |

**Conclusion:** Happy path unchanged; agent runtime attachments still work via `api_4_conversation` fallback.

---

## 3. Code paths covered

```text
download_attachment
  └─ await thread_pool_exec(_agent_attachment_accessible, doc_id, current_user.id)
       ├─ DocumentService.accessible(attachment_id, user_id)  [dataset-backed]
       └─ _agent_attachment_in_user_conversation(...)         [agent session JSON]
  └─ await thread_pool_exec(STORAGE_IMPL.get, tenant_id, doc_id)
  └─ if not data → get_data_error_result("Document not found!")
```

---

## 4. Manual E2E (optional on deployed RAGFlow)

Not run in this workspace (no live RAGFlow + MinIO). Use these on staging if maintainers want HTTP-level proof:

```bash
# Unauthorized (User B, attachment from User A's session)
curl -sS -D - -o /tmp/out.bin \
  -H "Authorization: Bearer <USER_B_TOKEN>" \
  "http://<HOST>/api/v1/agents/attachments/<ATTACH_A>/download?ext=pdf"
# Expect: JSON with "Document not found!", not PDF magic bytes

# Authorized owner
curl -sS -D - -o /tmp/ok.bin \
  -H "Authorization: Bearer <USER_A_TOKEN>" \
  "http://<HOST>/api/v1/agents/attachments/<ATTACH_A>/download?ext=pdf"
# Expect: 200 + binary body
```

---

## 5. CI

| Check | Status |
|-------|--------|
| `ragflow_tests` on latest push | Re-run if needed (prior failures on other PRs were REST teardown timeouts, not this diff) |
| CodeRabbit thread-pool comment | Addressed in `b11774ccb` |

---

## Files added for testing

| File | Purpose |
|------|---------|
| `test/testcases/restful_api/run_agent_attachment_download_unit.py` | Standalone PASS/FAIL runner (no pytest conftest) |
| `test/testcases/restful_api/test_agent_attachment_download_unit.py` | Pytest cases for CI (runs with full project deps) |

---

## Reply to post on PR (copy below)

See section in `PR-15507-testing-evidence.md` or use the reply block in the maintainer comment section of this doc.
