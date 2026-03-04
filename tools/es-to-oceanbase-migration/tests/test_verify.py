"""
Tests for migration verification.
"""

import pytest
from unittest.mock import Mock

from es_ob_migration.verify import MigrationVerifier, VerificationResult


class TestVerificationResult:
    """Test VerificationResult dataclass."""

    def test_create_basic_result(self):
        """Test creating a basic result."""
        result = VerificationResult(
            es_index="ragflow_test",
            ob_table="ragflow_test",
        )
        
        assert result.es_index == "ragflow_test"
        assert result.ob_table == "ragflow_test"
        assert result.es_count == 0
        assert result.ob_count == 0
        assert result.passed is False

    def test_result_default_values(self):
        """Test default values."""
        result = VerificationResult(
            es_index="test",
            ob_table="test",
        )
        
        assert result.count_match is False
        assert result.count_diff == 0
        assert result.sample_size == 0
        assert result.samples_verified == 0
        assert result.samples_matched == 0
        assert result.sample_match_rate == 0.0
        assert result.missing_in_ob == []
        assert result.data_mismatches == []
        assert result.message == ""

    def test_result_with_counts(self):
        """Test result with count data."""
        result = VerificationResult(
            es_index="test",
            ob_table="test",
            es_count=1000,
            ob_count=1000,
            count_match=True,
        )
        
        assert result.es_count == 1000
        assert result.ob_count == 1000
        assert result.count_match is True


class TestMigrationVerifier:
    """Test MigrationVerifier class."""

    @pytest.fixture
    def mock_es_client(self):
        """Create mock ES client."""
        client = Mock()
        client.count_documents = Mock(return_value=100)
        client.get_sample_documents = Mock(return_value=[])
        return client

    @pytest.fixture
    def mock_ob_client(self):
        """Create mock OB client."""
        client = Mock()
        client.count_rows = Mock(return_value=100)
        client.get_row_by_id = Mock(return_value=None)
        return client

    @pytest.fixture
    def verifier(self, mock_es_client, mock_ob_client):
        """Create verifier with mock clients."""
        return MigrationVerifier(mock_es_client, mock_ob_client)

    def test_verify_counts_match(self, mock_es_client, mock_ob_client):
        """Test verification when counts match."""
        mock_es_client.count_documents.return_value = 1000
        mock_ob_client.count_rows.return_value = 1000
        mock_es_client.get_sample_documents.return_value = []
        
        verifier = MigrationVerifier(mock_es_client, mock_ob_client)
        result = verifier.verify("ragflow_test", "ragflow_test", sample_size=0)
        
        assert result.es_count == 1000
        assert result.ob_count == 1000
        assert result.count_match is True
        assert result.count_diff == 0

    def test_verify_counts_mismatch(self, mock_es_client, mock_ob_client):
        """Test verification when counts don't match."""
        mock_es_client.count_documents.return_value = 1000
        mock_ob_client.count_rows.return_value = 950
        mock_es_client.get_sample_documents.return_value = []
        
        verifier = MigrationVerifier(mock_es_client, mock_ob_client)
        result = verifier.verify("ragflow_test", "ragflow_test", sample_size=0)
        
        assert result.es_count == 1000
        assert result.ob_count == 950
        assert result.count_match is False
        assert result.count_diff == 50

    def test_verify_samples_all_match(self, mock_es_client, mock_ob_client):
        """Test sample verification when all samples match."""
        # Setup ES samples
        es_samples = [
            {"_id": f"doc_{i}", "id": f"doc_{i}", "kb_id": "kb_001", "content_with_weight": f"content_{i}"}
            for i in range(10)
        ]
        mock_es_client.count_documents.return_value = 100
        mock_es_client.get_sample_documents.return_value = es_samples
        
        # Setup OB to return matching documents
        def get_row(table, doc_id):
            return {"id": doc_id, "kb_id": "kb_001", "content_with_weight": f"content_{doc_id.split('_')[1]}"}
        
        mock_ob_client.count_rows.return_value = 100
        mock_ob_client.get_row_by_id.side_effect = get_row
        
        verifier = MigrationVerifier(mock_es_client, mock_ob_client)
        result = verifier.verify("ragflow_test", "ragflow_test", sample_size=10)
        
        assert result.samples_verified == 10
        assert result.samples_matched == 10
        assert result.sample_match_rate == 1.0

    def test_verify_samples_some_missing(self, mock_es_client, mock_ob_client):
        """Test sample verification when some documents are missing."""
        es_samples = [
            {"_id": f"doc_{i}", "id": f"doc_{i}", "kb_id": "kb_001"}
            for i in range(10)
        ]
        mock_es_client.count_documents.return_value = 100
        mock_es_client.get_sample_documents.return_value = es_samples
        
        # Only return some documents
        def get_row(table, doc_id):
            idx = int(doc_id.split("_")[1])
            if idx < 7:  # Only return first 7
                return {"id": doc_id, "kb_id": "kb_001"}
            return None
        
        mock_ob_client.count_rows.return_value = 100
        mock_ob_client.get_row_by_id.side_effect = get_row
        
        verifier = MigrationVerifier(mock_es_client, mock_ob_client)
        result = verifier.verify("ragflow_test", "ragflow_test", sample_size=10)
        
        assert result.samples_verified == 10
        assert result.samples_matched == 7
        assert len(result.missing_in_ob) == 3

    def test_verify_samples_data_mismatch(self, mock_es_client, mock_ob_client):
        """Test sample verification when data doesn't match."""
        es_samples = [
            {"_id": "doc_1", "id": "doc_1", "kb_id": "kb_001", "available_int": 1}
        ]
        mock_es_client.count_documents.return_value = 100
        mock_es_client.get_sample_documents.return_value = es_samples
        
        # Return document with different data
        mock_ob_client.count_rows.return_value = 100
        mock_ob_client.get_row_by_id.return_value = {
            "id": "doc_1", "kb_id": "kb_002", "available_int": 0  # Different values
        }
        
        verifier = MigrationVerifier(mock_es_client, mock_ob_client)
        result = verifier.verify("ragflow_test", "ragflow_test", sample_size=1)
        
        assert result.samples_verified == 1
        assert result.samples_matched == 0
        assert len(result.data_mismatches) == 1

    def test_values_equal_none_values(self, verifier):
        """Test value comparison with None values."""
        assert verifier._values_equal("field", None, None) is True
        assert verifier._values_equal("field", "value", None) is False
        assert verifier._values_equal("field", None, "value") is False

    def test_values_equal_array_columns(self, verifier):
        """Test value comparison for array columns."""
        # Array stored as JSON string in OB
        assert verifier._values_equal(
            "important_kwd",
            ["key1", "key2"],
            '["key1", "key2"]'
        ) is True
        
        # Order shouldn't matter for arrays
        assert verifier._values_equal(
            "important_kwd",
            ["key2", "key1"],
            '["key1", "key2"]'
        ) is True

    def test_values_equal_json_columns(self, verifier):
        """Test value comparison for JSON columns."""
        assert verifier._values_equal(
            "metadata",
            {"author": "John"},
            '{"author": "John"}'
        ) is True

    def test_values_equal_kb_id_list(self, verifier):
        """Test kb_id comparison when ES has list."""
        # ES sometimes stores kb_id as list
        assert verifier._values_equal(
            "kb_id",
            ["kb_001", "kb_002"],
            "kb_001"
        ) is True

    def test_values_equal_content_with_weight_dict(self, verifier):
        """Test content_with_weight comparison when OB has JSON string."""
        assert verifier._values_equal(
            "content_with_weight",
            {"text": "content", "weight": 1.0},
            '{"text": "content", "weight": 1.0}'
        ) is True

    def test_determine_result_passed(self, mock_es_client, mock_ob_client):
        """Test result determination for passed verification."""
        mock_es_client.count_documents.return_value = 1000
        mock_ob_client.count_rows.return_value = 1000
        
        es_samples = [{"_id": f"doc_{i}", "id": f"doc_{i}", "kb_id": "kb_001"} for i in range(100)]
        mock_es_client.get_sample_documents.return_value = es_samples
        mock_ob_client.get_row_by_id.side_effect = lambda t, d: {"id": d, "kb_id": "kb_001"}
        
        verifier = MigrationVerifier(mock_es_client, mock_ob_client)
        result = verifier.verify("test", "test", sample_size=100)
        
        assert result.passed is True
        assert "PASSED" in result.message

    def test_determine_result_failed_count(self, mock_es_client, mock_ob_client):
        """Test result determination when count verification fails."""
        mock_es_client.count_documents.return_value = 1000
        mock_ob_client.count_rows.return_value = 500  # Big difference
        mock_es_client.get_sample_documents.return_value = []
        
        verifier = MigrationVerifier(mock_es_client, mock_ob_client)
        result = verifier.verify("test", "test", sample_size=0)
        
        assert result.passed is False
        assert "FAILED" in result.message

    def test_determine_result_failed_samples(self, mock_es_client, mock_ob_client):
        """Test result determination when sample verification fails."""
        mock_es_client.count_documents.return_value = 100
        mock_ob_client.count_rows.return_value = 100
        
        es_samples = [{"_id": f"doc_{i}", "id": f"doc_{i}"} for i in range(10)]
        mock_es_client.get_sample_documents.return_value = es_samples
        mock_ob_client.get_row_by_id.return_value = None  # All missing
        
        verifier = MigrationVerifier(mock_es_client, mock_ob_client)
        result = verifier.verify("test", "test", sample_size=10)
        
        assert result.passed is False

    def test_generate_report(self, verifier):
        """Test report generation."""
        result = VerificationResult(
            es_index="ragflow_test",
            ob_table="ragflow_test",
            es_count=1000,
            ob_count=1000,
            count_match=True,
            count_diff=0,
            sample_size=100,
            samples_verified=100,
            samples_matched=100,
            sample_match_rate=1.0,
            passed=True,
            message="Verification PASSED",
        )
        
        report = verifier.generate_report(result)
        
        assert "ragflow_test" in report
        assert "1,000" in report
        assert "PASSED" in report
        assert "100.00%" in report

    def test_generate_report_with_missing(self, verifier):
        """Test report generation with missing documents."""
        result = VerificationResult(
            es_index="test",
            ob_table="test",
            es_count=100,
            ob_count=95,
            count_match=False,
            count_diff=5,
            sample_size=10,
            samples_verified=10,
            samples_matched=8,
            sample_match_rate=0.8,
            missing_in_ob=["doc_1", "doc_2"],
            passed=False,
            message="Verification FAILED",
        )
        
        report = verifier.generate_report(result)
        
        assert "Missing in OceanBase" in report
        assert "doc_1" in report
        assert "FAILED" in report

    def test_generate_report_with_mismatches(self, verifier):
        """Test report generation with data mismatches."""
        result = VerificationResult(
            es_index="test",
            ob_table="test",
            es_count=100,
            ob_count=100,
            count_match=True,
            sample_size=10,
            samples_verified=10,
            samples_matched=8,
            sample_match_rate=0.8,
            data_mismatches=[
                {
                    "id": "doc_1",
                    "differences": [
                        {"field": "kb_id", "es_value": "kb_001", "ob_value": "kb_002"}
                    ]
                }
            ],
            passed=False,
            message="Verification FAILED",
        )
        
        report = verifier.generate_report(result)
        
        assert "Data Mismatches" in report
        assert "doc_1" in report
        assert "kb_id" in report


class TestValueComparison:
    """Test value comparison edge cases."""

    @pytest.fixture
    def verifier(self):
        """Create verifier with mock clients."""
        return MigrationVerifier(Mock(), Mock())

    def test_string_comparison(self, verifier):
        """Test string comparison."""
        assert verifier._values_equal("field", "value", "value") is True
        assert verifier._values_equal("field", "value1", "value2") is False

    def test_integer_comparison(self, verifier):
        """Test integer comparison (converted to string)."""
        assert verifier._values_equal("field", 123, "123") is True
        assert verifier._values_equal("field", "123", 123) is True

    def test_float_comparison(self, verifier):
        """Test float comparison."""
        assert verifier._values_equal("field", 1.5, "1.5") is True

    def test_boolean_comparison(self, verifier):
        """Test boolean comparison."""
        assert verifier._values_equal("field", True, "True") is True
        assert verifier._values_equal("field", False, "False") is True

    def test_empty_array_comparison(self, verifier):
        """Test empty array comparison."""
        assert verifier._values_equal("important_kwd", [], "[]") is True

    def test_nested_json_comparison(self, verifier):
        """Test nested JSON comparison."""
        es_value = {"nested": {"key": "value"}}
        ob_value = '{"nested": {"key": "value"}}'
        assert verifier._values_equal("metadata", es_value, ob_value) is True
