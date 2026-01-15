# Pre-Code Rabbit #2 Audit Workflow

This document outlines the structured audit process before submitting changes to Code Rabbit for a second review.

**Context:** Substantial edits have been made post-Code Rabbit #1, including:
- Resolving Code Rabbit identified issues
- Expanding test architecture scope
- Refactoring fragmentary testing patterns

**Goal:** Get as clean as possible through Code Rabbit, then local testing → production testing → commit.

---

## Phase 1: Static Analysis ✅

**Status:** COMPLETE

**Completed Tasks:**
- ✅ Ruff unused imports check (F401) - 15 issues fixed in test files
- ✅ Additional ruff check on modified files - 1 issue fixed in `api/db/error_handlers.py`
- ✅ Pylance error check on modified files - 0 errors found

**Commands Run:**
```bash
# Check test files for unused imports
ruff check test/ --select F401

# Fix test files
ruff check test/ --select F401 --fix

# Check modified api/db/ files
ruff check api/db/ api/ragflow_server.py common/exceptions.py

# Fix modified files
ruff check api/db/ api/ragflow_server.py common/exceptions.py --fix
```

**Results:**
- Total issues fixed: 16
- No remaining code quality issues detected

**Next Step:** Proceed to Phase 2 - Test Integrity Audit

---

## Phase 2: Test Integrity Audit

**Status:** PENDING

**Purpose:** Validate test structure and fixtures before running tests.

**Tasks to Complete:**

### 2.1: Pytest Collection Validation
Verify all tests can be discovered without errors:
```bash
uv run pytest --collect-only -q
```
**Expected:** All tests collected without errors or warnings.

### 2.2: Fixture Validation
Check that all required fixtures are defined:
```bash
# Find all fixtures defined
grep -r "@pytest.fixture" test/

# Find all fixtures used
grep -r "def test_.*sample_" test/unit_test/services/
```
**Expected Fixtures in `test/unit_test/services/conftest.py`:**
- `sample_dialog` - Sample dialog object
- `sample_tenant` - Sample tenant object
- `sample_kb` - Sample knowledge base object
- Any others used by test methods

### 2.3: Duplicate Test Name Check
Ensure no duplicate test function names:
```bash
grep -r "def test_" test/ | cut -d: -f2 | sort | uniq -d
```
**Expected:** No output (no duplicates)

### 2.4: Python Compilation Check
Validate all test files compile without syntax errors:
```bash
uv run python -m py_compile test/**/*.py
```
**Expected:** Clean exit (code 0)

### 2.5: Import Chain Validation
Verify all imports in test files resolve:
```bash
# Try to import each test module
uv run python -c "import test.unit_test.services.test_dialog_service"
uv run python -c "import test.unit_test.services.test_document_service"
uv run python -c "import test.unit_test.services.test_knowledgebase_service"
uv run python -c "import test.unit_test.services.test_user_service"
```
**Expected:** No ImportError exceptions

**Completion Criteria:**
- ✓ All tests collect without errors
- ✓ All fixtures are defined
- ✓ No duplicate test names
- ✓ All files compile cleanly
- ✓ All imports resolve

**Next Step:** Proceed to Phase 3 - Local Unit Testing

---

## Phase 3: Local Unit Testing

**Status:** PENDING

**Purpose:** Run tests locally and verify functionality.

**Tasks to Complete:**

### 3.1: Run Unit Tests with Coverage
Test the modified service files:
```bash
# Run dialog service tests
uv run pytest test/unit_test/services/test_dialog_service.py -v

# Run all modified service tests
uv run pytest test/unit_test/services/ -v --tb=short

# Generate coverage report
uv run pytest test/unit_test/services/ --cov=api.db.services --cov-report=html --cov-report=term
```

**Expected Results:**
- All tests pass (or are properly skipped)
- No import errors
- Coverage metrics visible for modified code

### 3.2: Validate Test Isolation
Ensure mocks are properly isolated:
```bash
# Run tests in random order to catch state leakage
uv run pytest test/unit_test/services/ -v --random-order
```

**Expected:** All tests pass regardless of execution order

### 3.3: Check for Missing Implementations
Verify no incomplete test stubs:
```bash
grep -r "pass$" test/unit_test/services/*.py | grep "def test_"
```

**Expected:** Either empty or only in deliberately skipped tests

**Completion Criteria:**
- ✓ All active tests pass
- ✓ Skipped tests have documented reasons
- ✓ No test state leakage
- ✓ Code coverage ≥ 80% for modified services

**Next Step:** Proceed to Phase 4 - Code Rabbit #2 Review

---

## Phase 4: Code Rabbit #2 Review

**Status:** PENDING

**Purpose:** Submit clean code to Code Rabbit for automated review.

**Checklist Before Submission:**
- ✓ Phase 1 (Static Analysis) complete
- ✓ Phase 2 (Test Integrity) complete
- ✓ Phase 3 (Local Testing) complete
- ✓ All issues resolved
- ✓ Ready for automated review

**Submission Steps:**
1. Share modified files with Code Rabbit
2. Wait for review feedback
3. Address any remaining issues
4. Proceed to Phase 5 (Integration Testing)

**Next Step:** Proceed to Phase 5 - Integration & Production Testing

---

## Phase 5: Integration & Production Testing

**Status:** PENDING

**Purpose:** Validate changes in full environment before commit.

**Tasks to Include:**

### 5.1: Full Integration Test Suite
```bash
uv run pytest test/integration/ -v
```

### 5.2: Database Migration Tests (if applicable)
```bash
uv run pytest test/test_db_migrations.py -v
```

### 5.3: Docker Container Testing
Run full stack and validate:
```bash
docker compose -f docker/docker-compose-base.yml up -d
# Run health checks
# Run API contract tests
docker compose down
```

### 5.4: Production Simulation
- Test with production-like configuration
- Validate error handling
- Check logging output

**Completion Criteria:**
- ✓ All integration tests pass
- ✓ No regressions in existing functionality
- ✓ Production simulation successful

**Next Step:** Proceed to commit

---

## Final: Commit & Push

**Status:** PENDING

**Pre-Commit Checklist:**
- ✓ All audit phases complete
- ✓ Code Rabbit approved
- ✓ Production testing passed
- ✓ No regressions detected

**Commands:**
```bash
git add .
git commit -m "feat: [description] - Post Code Rabbit audit pass"
git push origin fix-postgres-connection-pool
```

---

## Phase Summary Table

| Phase | Status | Purpose | Key Commands |
|-------|--------|---------|--------------|
| 1: Static Analysis | ✅ COMPLETE | Code quality & linting | `ruff check`, `pylance` |
| 2: Test Integrity | ⏳ PENDING | Fixture & structure validation | `pytest --collect-only` |
| 3: Local Unit Testing | ⏳ PENDING | Run tests locally | `pytest test/unit_test/` |
| 4: Code Rabbit #2 | ⏳ PENDING | Automated review | Submit to Code Rabbit |
| 5: Integration Testing | ⏳ PENDING | Full environment validation | `pytest test/integration/` |
| Final: Commit | ⏳ PENDING | Push to repository | `git commit && git push` |

---

## Notes

- **Modified Files in Scope:**
  - `api/db/` (complete module refactor)
  - `api/ragflow_server.py`
  - `common/exceptions.py`
  - `test/unit_test/services/` (new & refactored tests)

- **Key Concerns to Monitor:**
  - Fixture availability for all test methods
  - Mock isolation and cleanup
  - Import chain integrity
  - Test execution order independence

- **Contact Point:** Start new chat for each phase with phase number in context

---

*Last Updated: 2026-01-12*
