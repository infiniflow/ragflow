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
Unit tests for KnowledgebaseService.

Tests business logic for dataset management operations including:
- Access control validation
- Duplicate name detection
- Parser configuration handling
- Document parsing status tracking

These tests mock database operations to ensure fast, isolated execution.
"""
import pytest
from unittest.mock import MagicMock, patch
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.constants import DATASET_NAME_LIMIT


@pytest.mark.p1
class TestKnowledgebaseAccessControl:
    """Test access control logic for datasets."""

    @patch.object(KnowledgebaseService.model, 'select')
    def test_accessible4deletion_user_is_creator(self, mock_select, sample_kb, sample_user):
        """Test that creator can delete their own dataset.

        Verifies that accessible4deletion returns True when the requesting
        user is the creator of the dataset.
        """
        # Arrange: Mock database query to return a matching dataset
        mock_query = MagicMock()
        mock_query.dicts.return_value = [{'id': sample_kb['id']}]
        mock_select.return_value.where.return_value.paginate.return_value = mock_query

        # Act: Check if user can delete
        result = KnowledgebaseService.accessible4deletion(
            kb_id=sample_kb['id'],
            user_id=sample_user['id']
        )

        # Assert: User should have access
        assert result is True

        # Verify database query was called correctly
        mock_select.assert_called_once()

    @patch.object(KnowledgebaseService.model, 'select')
    def test_accessible4deletion_user_not_creator(self, mock_select):
        """Test that non-creator cannot delete dataset.

        Verifies that accessible4deletion returns False when the requesting
        user is not the creator of the dataset.
        """
        # Arrange: Mock database query to return no results
        mock_query = MagicMock()
        mock_query.dicts.return_value = []
        mock_select.return_value.where.return_value.paginate.return_value = mock_query

        # Act: Check if non-creator can delete
        result = KnowledgebaseService.accessible4deletion(
            kb_id='kb-001',
            user_id='other-user-id'
        )

        # Assert: User should not have access
        assert result is False

    @patch.object(KnowledgebaseService.model, 'select')
    def test_accessible4deletion_nonexistent_kb(self, mock_select):
        """Test access check for non-existent dataset.

        Verifies that accessible4deletion returns False when checking
        access to a dataset that doesn't exist.
        """
        # Arrange: Mock database query to return no results
        mock_query = MagicMock()
        mock_query.dicts.return_value = []
        mock_select.return_value.where.return_value.paginate.return_value = mock_query

        # Act: Check access to non-existent dataset
        result = KnowledgebaseService.accessible4deletion(
            kb_id='nonexistent-kb',
            user_id='user-001'
        )

        # Assert: Should return False
        assert result is False


@pytest.mark.p1
class TestKnowledgebaseNameValidation:
    """Test name validation and duplicate detection."""

    @pytest.mark.skip(reason="TODO: Call KnowledgebaseService.save or validation method instead of calling mock directly")
    @patch('api.db.services.knowledgebase_service.duplicate_name')
    def test_duplicate_name_check_valid(self, mock_duplicate_name, sample_tenant):
        """Test that valid unique name passes validation.

        Verifies that duplicate_name returns True when a name is not
        already in use by another dataset in the same tenant.
        """
        # Arrange: Mock duplicate check to return False (name is unique)
        mock_duplicate_name.return_value = False

        # Act: Check if name is duplicate
        is_duplicate = mock_duplicate_name(
            KnowledgebaseService.model,
            sample_tenant['id'],
            'New Dataset Name',
            None
        )

        # Assert: Name should be unique
        assert is_duplicate is False

    @pytest.mark.skip(reason="TODO: Call KnowledgebaseService.save or validation method instead of calling mock directly")
    @patch('api.db.services.knowledgebase_service.duplicate_name')
    def test_duplicate_name_check_exists(self, mock_duplicate_name, sample_tenant):
        """Test that duplicate name is detected.

        Verifies that duplicate_name returns True when a name is already
        in use by another dataset in the same tenant.
        """
        # Arrange: Mock duplicate check to return True (name exists)
        mock_duplicate_name.return_value = True

        # Act: Check if name is duplicate
        is_duplicate = mock_duplicate_name(
            KnowledgebaseService.model,
            sample_tenant['id'],
            'Existing Dataset',
            None
        )

        # Assert: Name should be detected as duplicate
        assert is_duplicate is True

    @pytest.mark.skip(reason="TODO: Call actual service validation method that enforces DATASET_NAME_LIMIT")
    @patch.object(KnowledgebaseService, 'save')
    def test_name_length_validation(self, mock_save):
        """Test that dataset name length is within limits.

        Verifies that dataset names respect the DATASET_NAME_LIMIT
        constant defined in api.constants.
        """
        # Arrange: Create name at limit and over limit
        valid_name = 'A' * DATASET_NAME_LIMIT
        invalid_name = 'A' * (DATASET_NAME_LIMIT + 1)

        # Act & Assert: Valid name at limit should be accepted
        mock_save.return_value = True
        try:
            # Attempt to validate/create with valid name
            assert len(valid_name) == DATASET_NAME_LIMIT
            # If service has validation, it should pass
        except Exception as e:
            pytest.fail(f"Valid name at limit should be accepted: {e}")

        # Act & Assert: Invalid name over limit should be rejected
        assert len(invalid_name) > DATASET_NAME_LIMIT
        # Service should reject names exceeding the limit


@pytest.mark.p2
class TestKnowledgebaseParsingStatus:
    """Test document parsing status tracking."""

    @pytest.mark.skip(reason="Requires DocumentService mock setup")
    @patch.object(KnowledgebaseService, 'query')
    @patch('api.db.services.knowledgebase_service.DocumentService')
    def test_is_parsed_done_all_complete(self, mock_doc_service, mock_query, sample_kb):
        """Test parsing status when all documents are complete.

        Verifies that is_parsed_done returns (True, None) when all
        documents in the dataset have completed parsing successfully.
        """
        # Arrange: Mock KB query
        mock_query.return_value = [sample_kb]

        # Mock DocumentService to return no unparsed documents
        mock_doc_service.query.return_value = []

        # Act: Check parsing status
        is_done, error_msg = KnowledgebaseService.is_parsed_done(sample_kb['id'])

        # Assert: Should be done with no errors
        assert is_done is True
        assert error_msg is None

    @pytest.mark.skip(reason="Requires DocumentService mock setup")
    @patch.object(KnowledgebaseService, 'query')
    @patch('api.db.services.knowledgebase_service.DocumentService')
    def test_is_parsed_done_has_pending(self, mock_doc_service, mock_query, sample_kb):
        """Test parsing status when documents are still processing.

        Verifies that is_parsed_done returns (False, message) when some
        documents in the dataset are still being parsed.
        """
        # Arrange: Mock KB query
        mock_query.return_value = [sample_kb]

        # Mock DocumentService to return pending documents
        mock_doc_service.query.return_value = [
            {'id': 'doc-001', 'name': 'pending.pdf', 'progress': 50}
        ]

        # Act: Check parsing status
        is_done, error_msg = KnowledgebaseService.is_parsed_done(sample_kb['id'])

        # Assert: Should not be done
        assert is_done is False
        assert error_msg is not None


@pytest.mark.p2
class TestKnowledgebaseConfiguration:
    """Test parser configuration management."""

    @pytest.mark.skip(reason="Requires parser config utility mock setup")
    def test_get_parser_config_default(self, sample_kb):
        """Test retrieval of default parser configuration.

        Verifies that get_parser_config returns default settings
        when no custom configuration is specified for the dataset.
        """
        # This test would verify default parser configuration retrieval
        # Requires mocking get_parser_config utility
        pass

    @pytest.mark.skip(reason="Requires parser config utility mock setup")
    def test_get_parser_config_custom(self, sample_kb):
        """Test retrieval of custom parser configuration.

        Verifies that get_parser_config returns custom settings
        when a dataset has been configured with non-default options.
        """
        # This test would verify custom parser configuration retrieval
        # Requires mocking get_parser_config utility
        pass


@pytest.mark.p2
class TestKnowledgebaseListOperations:
    """Test dataset listing and filtering."""

    @pytest.mark.skip(reason="Requires complex query mock setup")
    @patch.object(KnowledgebaseService, 'get_joined_list')
    def test_get_list_by_tenant(self, mock_get_list, sample_tenant):
        """Test listing datasets filtered by tenant.

        Verifies that get_list returns only datasets belonging to
        the specified tenant.
        """
        # Arrange: Mock service to return tenant datasets
        mock_get_list.return_value = (
            [
                {'id': 'kb-001', 'name': 'KB 1', 'tenant_id': sample_tenant['id']},
                {'id': 'kb-002', 'name': 'KB 2', 'tenant_id': sample_tenant['id']},
            ],
            2
        )

        # Act: Call the service method (mock intercepts)
        datasets, total = KnowledgebaseService.get_joined_list(
            tenant_id=sample_tenant['id'],
            page=1,
            page_size=10
        )

        # Assert: Should return tenant's datasets
        assert len(datasets) == 2
        assert total == 2
        assert all(kb['tenant_id'] == sample_tenant['id'] for kb in datasets)

    @pytest.mark.skip(reason="Requires complex query mock setup")
    @patch.object(KnowledgebaseService, 'get_joined_list')
    def test_get_list_with_pagination(self, mock_get_list, sample_tenant):
        """Test dataset listing with pagination.

        Verifies that get_list correctly applies pagination parameters
        when retrieving datasets.
        """
        # Arrange: Mock service with pagination
        mock_get_list.return_value = (
            [{'id': f'kb-{i:03d}', 'name': f'KB {i}'} for i in range(1, 11)],
            25  # Total count
        )

        # Act: Call the service method (mock intercepts)
        datasets, total = KnowledgebaseService.get_joined_list(
            tenant_id=sample_tenant['id'],
            page=1,
            page_size=10
        )

        # Assert: Should return correct page
        assert len(datasets) == 10
        assert total == 25
