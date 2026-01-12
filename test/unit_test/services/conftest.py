#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#
"""
Shared fixtures for service layer unit tests.

This module provides test fixtures for testing service classes in isolation.
Service tests should mock database operations and external dependencies to
ensure fast, independent test execution.

Key fixtures:
- mock_db: Mock database connection context
- mock_redis: Mock Redis connection
- sample_user: Test user data
- sample_tenant: Test tenant data
- sample_kb: Test knowledge base data
"""
import pytest
from unittest.mock import MagicMock, patch
from datetime import datetime


@pytest.fixture
def mock_db():
    """Mock database connection for service tests.

    This fixture mocks the database connection context manager to prevent
    actual database operations during unit tests. Use this for testing
    service methods that interact with the database.

    Yields:
        MagicMock: Mocked database connection context manager.

    Example:
        def test_kb_creation(mock_db):
            kb = KnowledgebaseService.save(name="test")
            assert kb is not None
    """
    with patch('api.db.db_models.DB.connection_context') as mock_ctx:
        mock_ctx.return_value.__enter__ = MagicMock()
        mock_ctx.return_value.__exit__ = MagicMock()
        yield mock_ctx


@pytest.fixture
def mock_redis():
    """Mock Redis connection for service tests.

    This fixture mocks Redis operations to avoid external dependencies
    during unit tests.

    Yields:
        MagicMock: Mocked Redis connection.

    Example:
        def test_with_cache(mock_redis):
            mock_redis.get.return_value = "cached_value"
            # Test service method that uses Redis
    """
    with patch('rag.utils.redis_conn.REDIS_CONN') as mock:
        yield mock


@pytest.fixture
def sample_user():
    """Provide sample user data for tests.

    Returns:
        dict: Sample user data with standard fields.

    Example:
        def test_user_operation(sample_user):
            result = UserService.create(**sample_user)
            assert result['email'] == sample_user['email']
    """
    # Use deterministic timestamps for reproducible tests
    TEST_DATETIME = datetime(2024, 1, 1, 12, 0, 0)
    return {
        'id': 'test-user-id-001',
        'email': 'test@example.com',
        'nickname': 'Test User',
        'status': 1,
        'create_time': TEST_DATETIME.timestamp(),
        'create_date': TEST_DATETIME.strftime('%Y-%m-%d %H:%M:%S'),
    }


@pytest.fixture
def sample_tenant():
    """Provide sample tenant data for tests.

    Returns:
        dict: Sample tenant data with standard fields.

    Example:
        def test_tenant_operation(sample_tenant):
            result = TenantService.create(**sample_tenant)
            assert result['name'] == sample_tenant['name']
    """
    return {
        'id': 'test-tenant-id-001',
        'name': 'Test Tenant',
        'llm_id': 'test-llm-001',
        'embd_id': 'test-embd-001',
        'asr_id': 'test-asr-001',
        'img2txt_id': 'test-img2txt-001',
        'parser_ids': 'naive:general',
        'credit': 10000,
        'status': 1,
    }


@pytest.fixture
def sample_kb(sample_tenant, sample_user):
    """Provide sample knowledge base data for tests.

    Args:
        sample_tenant: Tenant fixture (injected by pytest).
        sample_user: User fixture (injected by pytest).

    Returns:
        dict: Sample knowledge base data with standard fields.

    Example:
        def test_kb_creation(sample_kb):
            result = KnowledgebaseService.save(**sample_kb)
            assert result['name'] == sample_kb['name']
    """
    return {
        'id': 'test-kb-id-001',
        'tenant_id': sample_tenant['id'],
        'created_by': sample_user['id'],
        'name': 'Test Knowledge Base',
        'description': 'A test knowledge base',
        'language': 'English',
        'permission': 'me',
        'embd_id': 'BAAI/bge-large-zh-v1.5',
        'chunk_num': 0,
        'document_num': 0,
        'parser_id': 'naive',
        'parser_config': {},
        'status': 1,
    }


@pytest.fixture
def sample_dialog(sample_tenant, sample_user):
    """Provide sample dialog (chat assistant) data for tests.

    Args:
        sample_tenant: Tenant fixture (injected by pytest).
        sample_user: User fixture (injected by pytest).

    Returns:
        dict: Sample dialog data with standard fields.

    Example:
        def test_dialog_creation(sample_dialog):
            result = DialogService.save(**sample_dialog)
            assert result['name'] == sample_dialog['name']
    """
    return {
        'id': 'test-dialog-id-001',
        'tenant_id': sample_tenant['id'],
        'created_by': sample_user['id'],
        'name': 'Test Chat Assistant',
        'description': 'A test chat assistant',
        'language': 'English',
        'llm_id': 'qwen-plus',
        'prompt': 'You are a helpful assistant.',
        'llm_setting': {
            'temperature': 0.7,
            'max_tokens': 2048,
        },
        'status': 1,
    }


@pytest.fixture
def sample_document(sample_kb, sample_user):
    """Provide sample document data for tests.

    Args:
        sample_kb: Knowledge base fixture (injected by pytest).
        sample_user: User fixture (injected by pytest).

    Returns:
        dict: Sample document data with standard fields.

    Example:
        def test_document_creation(sample_document):
            result = DocumentService.save(**sample_document)
            assert result['name'] == sample_document['name']
    """
    return {
        'id': 'test-doc-id-001',
        'kb_id': sample_kb['id'],
        'created_by': sample_user['id'],
        'name': 'test_document.pdf',
        'type': 'pdf',
        'size': 1024000,
        'location': '/path/to/test_document.pdf',
        'parser_id': 'naive',
        'parser_config': {},
        'source_type': 'local',
        'status': 1,
        'progress': 0,
        'progress_msg': '',
        'chunk_num': 0,
        'token_num': 0,
    }



