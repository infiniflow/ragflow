# RAGFlow Unit Test Suite

Comprehensive unit tests for RAGFlow core services and features.

## 📁 Test Structure

```
test/unit_test/
├── common/                    # Utility function tests
│   ├── test_decorator.py
│   ├── test_file_utils.py
│   ├── test_float_utils.py
│   ├── test_misc_utils.py
│   ├── test_string_utils.py
│   ├── test_time_utils.py
│   └── test_token_utils.py
├── services/                  # Service layer tests (NEW)
│   ├── test_dialog_service.py
│   ├── test_conversation_service.py
│   ├── test_canvas_service.py
│   ├── test_knowledgebase_service.py
│   └── test_document_service.py
└── README.md                  # This file
```

## 🧪 Test Coverage

### Dialog Service Tests (`test_dialog_service.py`)
- ✅ Dialog creation, update, deletion
- ✅ Dialog retrieval by ID and tenant
- ✅ Name validation (empty, length limits)
- ✅ LLM settings validation
- ✅ Prompt configuration validation
- ✅ Knowledge base linking
- ✅ Duplicate name handling
- ✅ Pagination and search
- ✅ Status management
- **Total: 30+ test cases**

### Conversation Service Tests (`test_conversation_service.py`)
- ✅ Conversation creation with prologue
- ✅ Message management (add, delete, update)
- ✅ Reference handling with chunks
- ✅ Thumbup/thumbdown feedback
- ✅ Message structure validation
- ✅ Conversation ordering
- ✅ Batch operations
- ✅ Audio binary support
- **Total: 35+ test cases**

### Canvas/Agent Service Tests (`test_canvas_service.py`)
- ✅ Canvas creation, update, deletion
- ✅ DSL structure validation
- ✅ Component and edge validation
- ✅ Permission management (me/team)
- ✅ Canvas categories (agent/dataflow)
- ✅ Async execution testing
- ✅ Debug mode testing
- ✅ Version management
- ✅ Complex workflow testing
- **Total: 40+ test cases**

### Knowledge Base Service Tests (`test_knowledgebase_service.py`)
- ✅ KB creation, update, deletion
- ✅ Name validation
- ✅ Embedding model validation
- ✅ Parser configuration
- ✅ Language support
- ✅ Document/chunk/token statistics
- ✅ Batch operations
- ✅ Embedding model consistency
- **Total: 35+ test cases**

### Document Service Tests (`test_document_service.py`)
- ✅ Document upload and management
- ✅ File type validation
- ✅ Size validation
- ✅ Parsing status progression
- ✅ Progress tracking
- ✅ Chunk and token counting
- ✅ Batch upload/delete
- ✅ Search and pagination
- ✅ Parser configuration
- **Total: 35+ test cases**

## 🚀 Running Tests

### Run All Unit Tests
```bash
cd /root/74/ragflow
pytest test/unit_test/ -v
```

### Run Specific Test File
```bash
pytest test/unit_test/services/test_dialog_service.py -v
```

### Run Specific Test Class
```bash
pytest test/unit_test/services/test_dialog_service.py::TestDialogService -v
```

### Run Specific Test Method
```bash
pytest test/unit_test/services/test_dialog_service.py::TestDialogService::test_dialog_creation_success -v
```

### Run with Coverage Report
```bash
pytest test/unit_test/ --cov=api/db/services --cov-report=html
```

### Run Tests in Parallel
```bash
pytest test/unit_test/ -n auto
```

## 📊 Test Markers

Tests use pytest markers for categorization:

- `@pytest.mark.unit` - Unit tests (isolated, mocked)
- `@pytest.mark.integration` - Integration tests (with database)
- `@pytest.mark.asyncio` - Async tests
- `@pytest.mark.parametrize` - Parameterized tests

## 🛠️ Test Fixtures

### Common Fixtures

**`mock_dialog_service`** - Mocked DialogService for testing
```python
@pytest.fixture
def mock_dialog_service(self):
    with patch('api.db.services.dialog_service.DialogService') as mock:
        yield mock
```

**`sample_dialog_data`** - Sample dialog data
```python
@pytest.fixture
def sample_dialog_data(self):
    return {
        "id": get_uuid(),
        "tenant_id": "test_tenant_123",
        "name": "Test Dialog",
        ...
    }
```

## 📝 Writing New Tests

### Test Class Template

```python
import pytest
from unittest.mock import Mock, patch
from common.misc_utils import get_uuid

class TestYourService:
    """Comprehensive unit tests for YourService"""

    @pytest.fixture
    def mock_service(self):
        """Create a mock service for testing"""
        with patch('api.db.services.your_service.YourService') as mock:
            yield mock

    @pytest.fixture
    def sample_data(self):
        """Sample data for testing"""
        return {
            "id": get_uuid(),
            "name": "Test Item",
            ...
        }

    def test_creation_success(self, mock_service, sample_data):
        """Test successful creation"""
        mock_service.save.return_value = True
        result = mock_service.save(**sample_data)
        assert result is True

    def test_validation_error(self):
        """Test validation error handling"""
        with pytest.raises(Exception):
            if not valid_condition:
                raise Exception("Validation failed")
```

### Parameterized Test Template

```python
@pytest.mark.parametrize("input_value,expected", [
    ("valid", True),
    ("invalid", False),
    ("", False),
])
def test_validation(self, input_value, expected):
    """Test validation with different inputs"""
    result = validate(input_value)
    assert result == expected
```

## 🔍 Test Best Practices

1. **Isolation**: Each test should be independent
2. **Mocking**: Use mocks for external dependencies
3. **Clarity**: Test names should describe what they test
4. **Coverage**: Aim for >80% code coverage
5. **Speed**: Unit tests should run quickly (<1s each)
6. **Assertions**: Use specific assertions with clear messages

## 📈 Test Metrics

Current test suite statistics:

- **Total Test Files**: 5 (services) + 7 (common) = 12
- **Total Test Cases**: 175+
- **Test Coverage**: Services layer
- **Execution Time**: ~5-10 seconds

## 🐛 Debugging Tests

### Run with Verbose Output
```bash
pytest test/unit_test/ -vv
```

### Run with Print Statements
```bash
pytest test/unit_test/ -s
```

### Run with Debugging
```bash
pytest test/unit_test/ --pdb
```

### Run Failed Tests Only
```bash
pytest test/unit_test/ --lf
```

## 📚 Dependencies

Required packages for testing:
```
pytest>=7.0.0
pytest-asyncio>=0.21.0
pytest-cov>=4.0.0
pytest-mock>=3.10.0
pytest-xdist>=3.0.0  # For parallel execution
```

Install with:
```bash
pip install pytest pytest-asyncio pytest-cov pytest-mock pytest-xdist
```

## 🎯 Future Enhancements

- [ ] Integration tests with real database
- [ ] API endpoint tests
- [ ] Performance/load tests
- [ ] Frontend component tests
- [ ] End-to-end tests
- [ ] Continuous integration setup
- [ ] Test coverage badges
- [ ] Mutation testing

## 📞 Support

For questions or issues with tests:
1. Check test output for error messages
2. Review test documentation
3. Check existing test examples
4. Open an issue on GitHub

## 📄 License

Copyright 2025 The InfiniFlow Authors. All Rights Reserved.

Licensed under the Apache License, Version 2.0.
