# RAGFlow Test Architecture Refactoring

## Overview

This document describes the consolidated test architecture for RAGFlow, aligned with the modularization principles in [AGENTS.md](../AGENTS.md).

**Goal:** Reduce test fragmentation from 40+ endpoint-focused test files to ~15 service-layer tests, making the test suite more maintainable and resilient to API changes.

---

## Current State (Before Refactoring)

```
test/testcases/
├── test_http_api/              (20+ files)
│   ├── test_dataset_management/
│   ├── test_file_management_within_dataset/
│   ├── test_chunk_management_within_dataset/
│   ├── test_chat_assistant_management/
│   ├── test_session_management/
│   └── common.py
├── test_sdk_api/               (15+ files)
│   ├── test_dataset_management/
│   ├── test_chat_assistant_management/
│   ├── test_session_management/
│   └── common.py
├── test_web_api/               (15+ files)
│   ├── test_chunk_app/
│   ├── test_dialog_app/
│   ├── test_document_app/
│   ├── test_kb_app/
│   ├── test_memory_app/
│   └── test_message_app/
├── integration/
│   ├── conftest.py
│   ├── test_chat_workflow.py    (existing)
│   └── test_dataset_lifecycle.py (existing)
└── unit_test/
    ├── api_db/
    ├── common/
    └── utils/
```

**Problems:**
1. **Horizontal duplication**: Same tests repeated across 3 interfaces (HTTP, SDK, Web)
2. **Narrow focus**: Each file tests a single operation (create, delete, list, update)
3. **API-centric**: Tests mirror endpoint structure, not business logic
4. **Fragile**: Refactoring APIs requires updating 40+ test files
5. **Unclear intent**: Why are we testing the same thing 3 ways?

---

## Proposed State (After Refactoring)

```
test/
├── integration/                 (~12 files)
│   ├── conftest.py             # Shared fixtures, authentication
│   ├── helpers.py              # Test utilities
│   ├── test_dataset_lifecycle.py
│   ├── test_chat_workflow.py
│   ├── test_document_processing.py
│   ├── test_retrieval_workflow.py
│   ├── test_chat_assistant_workflow.py
│   └── ... (other workflows)
├── api_contract/               (~3 files)
│   ├── conftest.py             # Validation fixtures
│   ├── test_http_api_contracts.py
│   ├── test_sdk_api_contracts.py
│   └── test_web_api_contracts.py
├── unit_test/
│   ├── api_db/                 (unchanged - already modularized per PR #2)
│   ├── common/                 (unchanged)
│   ├── services/               (NEW - service layer unit tests)
│   │   ├── test_kb_service.py
│   │   ├── test_chat_service.py
│   │   ├── test_document_service.py
│   │   └── ...
│   └── utils/
└── testcases/ (legacy - located at test/testcases/)
    ├── libs/
    ├── utils/
    ├── conftest.py             (used by integration/ and api_contract/)
    └── ... (original test infrastructure)
```

**Benefits:**
1. **~62.5% fewer test files**: ~15 core files instead of 40+
2. **Service-centric**: Tests validate business logic, not HTTP routes
3. **Resilient**: Refactoring APIs doesn't require test changes
4. **Clear intent**: Each integration test documents a user workflow
5. **Aligned with AGENTS.md**: Service-layer focus, modularized components

---

## Test Levels (Pyramid)

```
API Contract Tests (3 files)       ← Validates interface stability
     ↑
Integration Tests (12 files)       ← Validates complete workflows
     ↑
Unit Tests (api/db, services)      ← Validates service logic
```

### Unit Tests: Service Layer

Per [AGENTS.md](../AGENTS.md#2-directory-structure), services are modularized in `api/db/services/`. Each service should have unit tests.

**Example:**

```python
# test/unit_test/services/test_kb_service.py
"""Knowledge base service unit tests."""

class TestKnowledgeBaseService:
    def test_create_knowledge_base(self):
        # Test service logic directly, no HTTP
        service = KnowledgeBaseService()
        kb = service.create("my_kb", ...)
        assert kb.id is not None
```

### Integration Tests: Workflows

Integration tests validate **complete workflows** across services, testing the same business logic through one interface (typically HTTP API).

**Example:**

```python
# test/integration/test_dataset_lifecycle.py
"""Dataset lifecycle: create → upload → parse → retrieve → delete."""

class TestDatasetLifecycle:
    def test_complete_workflow(self, api_client):
        # 1. Create dataset (kb_app service)
        dataset = api_client.create_dataset(...)
        
        # 2. Upload document (document_app service)
        api_client.upload_document(dataset.id, ...)
        
        # 3. Parse document (rag service)
        api_client.parse_documents(dataset.id, ...)
        
        # 4. Retrieve chunks (retrieval service)
        chunks = api_client.retrieve_chunks(dataset.id, ...)
        
        # 5. Verify workflow succeeded
        assert len(chunks) > 0
```

### API Contract Tests: Interface Validation

Contract tests verify that all interfaces (HTTP, SDK, Web) maintain consistent contracts—same requests produce consistent responses.

**Example:**

```python
# test/api_contract/test_http_api_contracts.py
"""HTTP API contract tests."""

class TestDatasetApiContract:
    def test_create_dataset_response_schema(self, http_client):
        res = http_client.create_dataset({...})
        
        # Contract: Response has these fields
        assert "code" in res
        assert "data" in res
        assert "dataset_id" in res["data"]
```

---

## Migration Strategy

### Phase 1: Establish New Test Home (Week 1)

1. **Create integration test structure:**
   - `test/integration/conftest.py` - Shared fixtures
   - `test/integration/helpers.py` - Test utilities to avoid circular dependencies
   - `test/integration/test_dataset_lifecycle.py` - First workflow
   - `test/integration/test_chat_workflow.py` - Second workflow

2. **Create API contract tests:**
   - `test/api_contract/conftest.py` - Contract fixtures
   - `test/api_contract/test_http_api_contracts.py` - HTTP validation
   - Light validation for critical paths only

3. **Verify both run successfully:**
   ```bash
   pytest test/integration/ -v
   pytest test/api_contract/ -v
   ```

### Phase 2: Port Critical Tests (Week 2)

Port the most-used tests from `testcases/test_http_api/` to integration tests:

- Dataset CRUD → `test_dataset_lifecycle.py`
- Chat assistant CRUD → `test_chat_assistant_workflow.py`
- Document upload/parse → `test_document_processing.py`

**Do NOT delete original tests yet.** Keep them running to ensure parity.

### Phase 3: Build Service Unit Tests (Week 3)

As you modularize services (per [AGENTS.md](../AGENTS.md#2-directory-structure)):

- Create `test/unit_test/services/` with per-service tests
- Move business logic validation from integration tests to unit tests where appropriate
- Keep integration tests focused on workflows

### Phase 3.5: Migrate Fixture Infrastructure (Week 3.5)

Before archiving `testcases/`, migrate shared fixture infrastructure to prevent circular dependencies:

1. **Copy fixture definitions** from `test/testcases/conftest.py` to new locations:
   - `test/integration/conftest.py` - fixtures like `api_client`, `token`, `cleanup`
   - `test/api_contract/conftest.py` - contract-specific fixtures

2. **Update imports** in all `test/integration/` and `test/api_contract/` tests:
   - Change `from test.testcases.conftest import ...` to local conftest imports
   - Ensure no remaining references to `testcases/conftest.py`

3. **Validate independence** - run tests without `testcases/`:
   ```bash
   # Temporarily rename testcases to verify independence
   mv test/testcases test/testcases_backup
   pytest test/integration/ -v
   pytest test/api_contract/ -v
   mv test/testcases_backup test/testcases
   ```

4. **Fix any failures** before proceeding to Phase 4

### Phase 4: Archive Original Tests (Week 4)

Once integration tests achieve 100% coverage parity with original tests:

1. Move `testcases/test_http_api/`, `testcases/test_sdk_api/`, `testcases/test_web_api/` → `test/testcases/archive/`
2. Update CI/CD to only run new tests
3. Keep archive for reference (document migration notes)

---

## What Gets Tested At Each Level

### Integration Tests: "User Workflows"

```python
test_dataset_lifecycle.py
├── Create dataset
├── Upload document
├── Parse document
├── Retrieve chunks
└── Delete dataset

test_chat_workflow.py
├── Create chat assistant
├── Create chat session
├── Send message
└── Verify response
```

### Unit Tests: "Service Logic"

```python
test_kb_service.py
├── Create knowledge base with validation
├── Update metadata
├── Query existence
└── Handle edge cases

test_document_service.py
├── Parse PDF with OCR
├── Extract text chunks
├── Index for retrieval
└── Handle corrupted files
```

### API Contract Tests: "Interface Stability"

```python
test_http_api_contracts.py
├── Response schemas (all endpoints)
├── Error response format
└── HTTP status codes

test_sdk_api_contracts.py
├── SDK method signatures match HTTP endpoints
└── Return types are consistent
```

---

## Test Discovery & Execution

RAGFlow uses pytest. Tests are discovered automatically:

```bash
# Run all tests
pytest

# Run only integration tests
pytest test/integration/

# Run only contracts
pytest test/api_contract/

# Run with markers (priority levels)
pytest -m p1  # High priority
pytest -m p2  # Medium priority

# Run specific file
pytest test/integration/test_dataset_lifecycle.py
```

---

## CI/CD Updates

Update your CI/CD configuration to run new tests:

```yaml
# .github/workflows/test.yml
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Set up Python
        uses: actions/setup-python@v4
        with:
          python-version: '3.12'
      
      # Start dependent services
      - name: Start services
        run: docker compose -f docker/docker-compose-base.yml up -d
      
      # Run new test suite
      - name: Run integration tests
        run: uv run pytest test/integration/ test/api_contract/ -v
      
      # Run unit tests
      - name: Run unit tests
        run: uv run pytest test/unit_test/ -v
```

---

## Writing New Integration Tests

### 1. Identify the workflow

What is a complete user workflow? Example:

- "I want to create a knowledge base, upload documents, and answer questions"
- "I want to create a chat assistant and have multi-turn conversations"

### 2. Create test file

```python
# test/integration/test_my_workflow.py
"""My workflow: step1 → step2 → step3."""

import pytest
from test.integration.helpers import create_dataset, upload_document, parse_documents

@pytest.mark.usefixtures("clear_datasets")  # Setup/teardown
class TestMyWorkflow:
    """Test complete workflow."""
    
    @pytest.mark.p1
    def test_workflow_success(self, api_client):
        """Test happy path."""
        # Step 1
        res = api_client.step1(...)
        assert res["code"] == 0
        
        # Step 2
        res = api_client.step2(...)
        assert res["code"] == 0
        
        # Step 3
        res = api_client.step3(...)
        assert res["code"] == 0
        assert res["data"]["expected_field"] is not None
```

### 3. Use existing fixtures

From `test/testcases/conftest.py`:

```python
@pytest.fixture
def api_client(token):
    """Authenticated API client."""
    return RAGFlowHttpApiAuth(token)

@pytest.fixture
def clear_datasets():
    """Clean up datasets after test."""
    yield
    # Cleanup happens here
```

### 4. Reuse stable integration helpers

From `test/integration/helpers.py` (stable module for integration tests):

```python
from test.integration.helpers import (
    create_dataset,
    upload_document,
    parse_documents,
)

# Use them directly
dataset = create_dataset(api_client, {"name": "my_dataset"})
upload_document(api_client, dataset["id"], file_path)
```

**Note:** These helpers are maintained in `test/integration/helpers.py` to avoid circular dependencies when archiving legacy test packages. They are re-implementations of the helpers from `testcases/test_http_api/common.py`, extracted during Phase 1 of the migration.

---

## References

- [AGENTS.md](../AGENTS.md) - Project architecture and modularization
- [CLAUDE.md](../CLAUDE.md) - Development guidelines
- [PR_STRATEGY.md](./testcases/archive/PR_STRATEGY.md) - Why this refactoring matters (PR #6) *(available after Phase 4 archival)*

---

## FAQ

**Q: Why consolidate tests if they're passing?**

A: Current tests are **interface-focused** (how to use HTTP/SDK), not **behavior-focused** (what the system does). This makes them fragile to API changes and harder to maintain. Service-layer testing ensures that APIs can be refactored without breaking tests.

**Q: Won't consolidation reduce test coverage?**

A: No. We're **moving tests**, not deleting them. One integration test replaces 3 duplicate endpoint tests while improving code clarity.

**Q: Can I still test all interfaces?**

A: Yes. Once integration tests are stable, add interface-specific tests in `test/api_contract/`. API contract tests are light (~10% of original volume) because business logic is tested once in integration tests.

**Q: What about SDK and Web API tests?**

A: For now, run them in `test/api_contract/` to verify they match HTTP API contracts. Eventually, add SDK-specific and Web-specific integration tests if needed (e.g., testing async operations in SDK).

**Q: How do I migrate an existing test?**

A: 1. Identify the workflow it tests
2. Create integration test file for that workflow
3. Port test logic (usually just copy-paste with minor changes)
4. Verify it passes
5. Delete old test file

---

## Success Criteria

✅ **Phase 1 (Initial Setup):**
- Create `test/integration/helpers.py` with stable implementations of `create_dataset`, `upload_document`, `parse_documents` ported from `testcases/test_http_api/common.py`
- `test/integration/` has at least 2 workflows using new helpers module
- `test/api_contract/` validates critical endpoints
- Both run successfully in CI/CD

✅ **Phase 2 (Migration):**
- Update all integration tests to import helpers from `test/integration/helpers.py` instead of `testcases/test_http_api/common.py`
- All critical paths from `testcases/test_http_api/` have integration test equivalents
- Coverage metrics match or exceed original tests
- Original tests still pass (backward compat)

✅ **Phase 3 (Consolidation):**
- Replace all imports in remaining code from `testcases.test_http_api.common` to `integration.helpers` where applicable
- Original test files and `testcases/test_http_api/common.py` archived (legacy package no longer imported by new code)
- CI/CD only runs new test suite
- Test execution time is equivalent or better
- Team consensus that new tests are easier to maintain

---

**Created:** January 2026 as part of PR #6 (Test Architecture Refactoring)
