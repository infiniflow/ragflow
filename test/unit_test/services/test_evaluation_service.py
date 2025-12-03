#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
Unit tests for RAG Evaluation Service

Tests cover:
- Dataset management (CRUD operations)
- Test case management
- Evaluation execution
- Metrics computation
- Recommendations generation
"""

import pytest
from unittest.mock import patch


class TestEvaluationDatasetManagement:
    """Tests for evaluation dataset management"""
    
    @pytest.fixture
    def mock_evaluation_service(self):
        """Create a mock EvaluationService"""
        with patch('api.db.services.evaluation_service.EvaluationService') as mock:
            yield mock
    
    @pytest.fixture
    def sample_dataset_data(self):
        """Sample dataset data for testing"""
        return {
            "name": "Customer Support QA",
            "description": "Test cases for customer support",
            "kb_ids": ["kb_123", "kb_456"],
            "tenant_id": "tenant_1",
            "user_id": "user_1"
        }
    
    def test_create_dataset_success(self, mock_evaluation_service, sample_dataset_data):
        """Test successful dataset creation"""
        mock_evaluation_service.create_dataset.return_value = (True, "dataset_123")
        
        success, dataset_id = mock_evaluation_service.create_dataset(**sample_dataset_data)
        
        assert success is True
        assert dataset_id == "dataset_123"
        mock_evaluation_service.create_dataset.assert_called_once()
    
    def test_create_dataset_with_empty_name(self, mock_evaluation_service):
        """Test dataset creation with empty name"""
        data = {
            "name": "",
            "description": "Test",
            "kb_ids": ["kb_123"],
            "tenant_id": "tenant_1",
            "user_id": "user_1"
        }
        
        mock_evaluation_service.create_dataset.return_value = (False, "Dataset name cannot be empty")
        success, error = mock_evaluation_service.create_dataset(**data)
        
        assert success is False
        assert "name" in error.lower() or "empty" in error.lower()
    
    def test_create_dataset_with_empty_kb_ids(self, mock_evaluation_service):
        """Test dataset creation with empty kb_ids"""
        data = {
            "name": "Test Dataset",
            "description": "Test",
            "kb_ids": [],
            "tenant_id": "tenant_1",
            "user_id": "user_1"
        }
        
        mock_evaluation_service.create_dataset.return_value = (False, "kb_ids cannot be empty")
        success, error = mock_evaluation_service.create_dataset(**data)
        
        assert success is False
    
    def test_get_dataset_success(self, mock_evaluation_service):
        """Test successful dataset retrieval"""
        expected_dataset = {
            "id": "dataset_123",
            "name": "Test Dataset",
            "kb_ids": ["kb_123"]
        }
        mock_evaluation_service.get_dataset.return_value = expected_dataset
        
        dataset = mock_evaluation_service.get_dataset("dataset_123")
        
        assert dataset is not None
        assert dataset["id"] == "dataset_123"
    
    def test_get_dataset_not_found(self, mock_evaluation_service):
        """Test getting non-existent dataset"""
        mock_evaluation_service.get_dataset.return_value = None
        
        dataset = mock_evaluation_service.get_dataset("nonexistent")
        
        assert dataset is None
    
    def test_list_datasets(self, mock_evaluation_service):
        """Test listing datasets"""
        expected_result = {
            "total": 2,
            "datasets": [
                {"id": "dataset_1", "name": "Dataset 1"},
                {"id": "dataset_2", "name": "Dataset 2"}
            ]
        }
        mock_evaluation_service.list_datasets.return_value = expected_result
        
        result = mock_evaluation_service.list_datasets(
            tenant_id="tenant_1",
            user_id="user_1",
            page=1,
            page_size=20
        )
        
        assert result["total"] == 2
        assert len(result["datasets"]) == 2
    
    def test_list_datasets_with_pagination(self, mock_evaluation_service):
        """Test listing datasets with pagination"""
        mock_evaluation_service.list_datasets.return_value = {
            "total": 50,
            "datasets": [{"id": f"dataset_{i}"} for i in range(10)]
        }
        
        result = mock_evaluation_service.list_datasets(
            tenant_id="tenant_1",
            user_id="user_1",
            page=2,
            page_size=10
        )
        
        assert result["total"] == 50
        assert len(result["datasets"]) == 10
    
    def test_update_dataset_success(self, mock_evaluation_service):
        """Test successful dataset update"""
        mock_evaluation_service.update_dataset.return_value = True
        
        success = mock_evaluation_service.update_dataset(
            "dataset_123",
            name="Updated Name",
            description="Updated Description"
        )
        
        assert success is True
    
    def test_update_dataset_not_found(self, mock_evaluation_service):
        """Test updating non-existent dataset"""
        mock_evaluation_service.update_dataset.return_value = False
        
        success = mock_evaluation_service.update_dataset(
            "nonexistent",
            name="Updated Name"
        )
        
        assert success is False
    
    def test_delete_dataset_success(self, mock_evaluation_service):
        """Test successful dataset deletion"""
        mock_evaluation_service.delete_dataset.return_value = True
        
        success = mock_evaluation_service.delete_dataset("dataset_123")
        
        assert success is True
    
    def test_delete_dataset_not_found(self, mock_evaluation_service):
        """Test deleting non-existent dataset"""
        mock_evaluation_service.delete_dataset.return_value = False
        
        success = mock_evaluation_service.delete_dataset("nonexistent")
        
        assert success is False


class TestEvaluationTestCaseManagement:
    """Tests for test case management"""
    
    @pytest.fixture
    def mock_evaluation_service(self):
        """Create a mock EvaluationService"""
        with patch('api.db.services.evaluation_service.EvaluationService') as mock:
            yield mock
    
    @pytest.fixture
    def sample_test_case(self):
        """Sample test case data"""
        return {
            "dataset_id": "dataset_123",
            "question": "How do I reset my password?",
            "reference_answer": "Click on 'Forgot Password' and follow the email instructions.",
            "relevant_doc_ids": ["doc_789"],
            "relevant_chunk_ids": ["chunk_101", "chunk_102"]
        }
    
    def test_add_test_case_success(self, mock_evaluation_service, sample_test_case):
        """Test successful test case addition"""
        mock_evaluation_service.add_test_case.return_value = (True, "case_123")
        
        success, case_id = mock_evaluation_service.add_test_case(**sample_test_case)
        
        assert success is True
        assert case_id == "case_123"
    
    def test_add_test_case_with_empty_question(self, mock_evaluation_service):
        """Test adding test case with empty question"""
        mock_evaluation_service.add_test_case.return_value = (False, "Question cannot be empty")
        
        success, error = mock_evaluation_service.add_test_case(
            dataset_id="dataset_123",
            question=""
        )
        
        assert success is False
        assert "question" in error.lower() or "empty" in error.lower()
    
    def test_add_test_case_without_reference_answer(self, mock_evaluation_service):
        """Test adding test case without reference answer (optional)"""
        mock_evaluation_service.add_test_case.return_value = (True, "case_123")
        
        success, case_id = mock_evaluation_service.add_test_case(
            dataset_id="dataset_123",
            question="Test question",
            reference_answer=None
        )
        
        assert success is True
    
    def test_get_test_cases(self, mock_evaluation_service):
        """Test getting all test cases for a dataset"""
        expected_cases = [
            {"id": "case_1", "question": "Question 1"},
            {"id": "case_2", "question": "Question 2"}
        ]
        mock_evaluation_service.get_test_cases.return_value = expected_cases
        
        cases = mock_evaluation_service.get_test_cases("dataset_123")
        
        assert len(cases) == 2
        assert cases[0]["id"] == "case_1"
    
    def test_get_test_cases_empty_dataset(self, mock_evaluation_service):
        """Test getting test cases from empty dataset"""
        mock_evaluation_service.get_test_cases.return_value = []
        
        cases = mock_evaluation_service.get_test_cases("dataset_123")
        
        assert len(cases) == 0
    
    def test_delete_test_case_success(self, mock_evaluation_service):
        """Test successful test case deletion"""
        mock_evaluation_service.delete_test_case.return_value = True
        
        success = mock_evaluation_service.delete_test_case("case_123")
        
        assert success is True
    
    def test_import_test_cases_success(self, mock_evaluation_service):
        """Test bulk import of test cases"""
        cases = [
            {"question": "Question 1", "reference_answer": "Answer 1"},
            {"question": "Question 2", "reference_answer": "Answer 2"},
            {"question": "Question 3", "reference_answer": "Answer 3"}
        ]
        mock_evaluation_service.import_test_cases.return_value = (3, 0)
        
        success_count, failure_count = mock_evaluation_service.import_test_cases(
            "dataset_123",
            cases
        )
        
        assert success_count == 3
        assert failure_count == 0
    
    def test_import_test_cases_with_failures(self, mock_evaluation_service):
        """Test bulk import with some failures"""
        cases = [
            {"question": "Question 1"},
            {"question": ""},  # Invalid
            {"question": "Question 3"}
        ]
        mock_evaluation_service.import_test_cases.return_value = (2, 1)
        
        success_count, failure_count = mock_evaluation_service.import_test_cases(
            "dataset_123",
            cases
        )
        
        assert success_count == 2
        assert failure_count == 1


class TestEvaluationExecution:
    """Tests for evaluation execution"""
    
    @pytest.fixture
    def mock_evaluation_service(self):
        """Create a mock EvaluationService"""
        with patch('api.db.services.evaluation_service.EvaluationService') as mock:
            yield mock
    
    def test_start_evaluation_success(self, mock_evaluation_service):
        """Test successful evaluation start"""
        mock_evaluation_service.start_evaluation.return_value = (True, "run_123")
        
        success, run_id = mock_evaluation_service.start_evaluation(
            dataset_id="dataset_123",
            dialog_id="dialog_456",
            user_id="user_1"
        )
        
        assert success is True
        assert run_id == "run_123"
    
    def test_start_evaluation_with_invalid_dialog(self, mock_evaluation_service):
        """Test starting evaluation with invalid dialog"""
        mock_evaluation_service.start_evaluation.return_value = (False, "Dialog not found")
        
        success, error = mock_evaluation_service.start_evaluation(
            dataset_id="dataset_123",
            dialog_id="nonexistent",
            user_id="user_1"
        )
        
        assert success is False
        assert "dialog" in error.lower()
    
    def test_start_evaluation_with_custom_name(self, mock_evaluation_service):
        """Test starting evaluation with custom name"""
        mock_evaluation_service.start_evaluation.return_value = (True, "run_123")
        
        success, run_id = mock_evaluation_service.start_evaluation(
            dataset_id="dataset_123",
            dialog_id="dialog_456",
            user_id="user_1",
            name="My Custom Evaluation"
        )
        
        assert success is True
    
    def test_get_run_results(self, mock_evaluation_service):
        """Test getting evaluation run results"""
        expected_results = {
            "run": {
                "id": "run_123",
                "status": "COMPLETED",
                "metrics_summary": {
                    "avg_precision": 0.85,
                    "avg_recall": 0.78
                }
            },
            "results": [
                {"case_id": "case_1", "metrics": {"precision": 0.9}},
                {"case_id": "case_2", "metrics": {"precision": 0.8}}
            ]
        }
        mock_evaluation_service.get_run_results.return_value = expected_results
        
        results = mock_evaluation_service.get_run_results("run_123")
        
        assert results["run"]["id"] == "run_123"
        assert len(results["results"]) == 2
    
    def test_get_run_results_not_found(self, mock_evaluation_service):
        """Test getting results for non-existent run"""
        mock_evaluation_service.get_run_results.return_value = {}
        
        results = mock_evaluation_service.get_run_results("nonexistent")
        
        assert results == {}


class TestEvaluationMetrics:
    """Tests for metrics computation"""
    
    @pytest.fixture
    def mock_evaluation_service(self):
        """Create a mock EvaluationService"""
        with patch('api.db.services.evaluation_service.EvaluationService') as mock:
            yield mock
    
    def test_compute_retrieval_metrics_perfect_match(self, mock_evaluation_service):
        """Test retrieval metrics with perfect match"""
        retrieved_ids = ["chunk_1", "chunk_2", "chunk_3"]
        relevant_ids = ["chunk_1", "chunk_2", "chunk_3"]
        
        expected_metrics = {
            "precision": 1.0,
            "recall": 1.0,
            "f1_score": 1.0,
            "hit_rate": 1.0,
            "mrr": 1.0
        }
        mock_evaluation_service._compute_retrieval_metrics.return_value = expected_metrics
        
        metrics = mock_evaluation_service._compute_retrieval_metrics(retrieved_ids, relevant_ids)
        
        assert metrics["precision"] == 1.0
        assert metrics["recall"] == 1.0
        assert metrics["f1_score"] == 1.0
    
    def test_compute_retrieval_metrics_partial_match(self, mock_evaluation_service):
        """Test retrieval metrics with partial match"""
        retrieved_ids = ["chunk_1", "chunk_2", "chunk_4", "chunk_5"]
        relevant_ids = ["chunk_1", "chunk_2", "chunk_3"]
        
        expected_metrics = {
            "precision": 0.5,  # 2 out of 4 retrieved are relevant
            "recall": 0.67,    # 2 out of 3 relevant were retrieved
            "f1_score": 0.57,
            "hit_rate": 1.0,   # At least one relevant was retrieved
            "mrr": 1.0         # First retrieved is relevant
        }
        mock_evaluation_service._compute_retrieval_metrics.return_value = expected_metrics
        
        metrics = mock_evaluation_service._compute_retrieval_metrics(retrieved_ids, relevant_ids)
        
        assert metrics["precision"] < 1.0
        assert metrics["recall"] < 1.0
        assert metrics["hit_rate"] == 1.0
    
    def test_compute_retrieval_metrics_no_match(self, mock_evaluation_service):
        """Test retrieval metrics with no match"""
        retrieved_ids = ["chunk_4", "chunk_5", "chunk_6"]
        relevant_ids = ["chunk_1", "chunk_2", "chunk_3"]
        
        expected_metrics = {
            "precision": 0.0,
            "recall": 0.0,
            "f1_score": 0.0,
            "hit_rate": 0.0,
            "mrr": 0.0
        }
        mock_evaluation_service._compute_retrieval_metrics.return_value = expected_metrics
        
        metrics = mock_evaluation_service._compute_retrieval_metrics(retrieved_ids, relevant_ids)
        
        assert metrics["precision"] == 0.0
        assert metrics["recall"] == 0.0
        assert metrics["hit_rate"] == 0.0
    
    def test_compute_summary_metrics(self, mock_evaluation_service):
        """Test summary metrics computation"""
        results = [
            {"metrics": {"precision": 0.9, "recall": 0.8}, "execution_time": 1.2},
            {"metrics": {"precision": 0.8, "recall": 0.7}, "execution_time": 1.5},
            {"metrics": {"precision": 0.85, "recall": 0.75}, "execution_time": 1.3}
        ]
        
        expected_summary = {
            "total_cases": 3,
            "avg_execution_time": 1.33,
            "avg_precision": 0.85,
            "avg_recall": 0.75
        }
        mock_evaluation_service._compute_summary_metrics.return_value = expected_summary
        
        summary = mock_evaluation_service._compute_summary_metrics(results)
        
        assert summary["total_cases"] == 3
        assert summary["avg_precision"] > 0.8


class TestEvaluationRecommendations:
    """Tests for configuration recommendations"""
    
    @pytest.fixture
    def mock_evaluation_service(self):
        """Create a mock EvaluationService"""
        with patch('api.db.services.evaluation_service.EvaluationService') as mock:
            yield mock
    
    def test_get_recommendations_low_precision(self, mock_evaluation_service):
        """Test recommendations for low precision"""
        recommendations = [
            {
                "issue": "Low Precision",
                "severity": "high",
                "suggestions": [
                    "Increase similarity_threshold",
                    "Enable reranking"
                ]
            }
        ]
        mock_evaluation_service.get_recommendations.return_value = recommendations
        
        recs = mock_evaluation_service.get_recommendations("run_123")
        
        assert len(recs) > 0
        assert any("precision" in r["issue"].lower() for r in recs)
    
    def test_get_recommendations_low_recall(self, mock_evaluation_service):
        """Test recommendations for low recall"""
        recommendations = [
            {
                "issue": "Low Recall",
                "severity": "high",
                "suggestions": [
                    "Increase top_k",
                    "Lower similarity_threshold"
                ]
            }
        ]
        mock_evaluation_service.get_recommendations.return_value = recommendations
        
        recs = mock_evaluation_service.get_recommendations("run_123")
        
        assert len(recs) > 0
        assert any("recall" in r["issue"].lower() for r in recs)
    
    def test_get_recommendations_slow_response(self, mock_evaluation_service):
        """Test recommendations for slow response time"""
        recommendations = [
            {
                "issue": "Slow Response Time",
                "severity": "medium",
                "suggestions": [
                    "Reduce top_k",
                    "Optimize embedding model"
                ]
            }
        ]
        mock_evaluation_service.get_recommendations.return_value = recommendations
        
        recs = mock_evaluation_service.get_recommendations("run_123")
        
        assert len(recs) > 0
        assert any("response" in r["issue"].lower() or "slow" in r["issue"].lower() for r in recs)
    
    def test_get_recommendations_no_issues(self, mock_evaluation_service):
        """Test recommendations when metrics are good"""
        mock_evaluation_service.get_recommendations.return_value = []
        
        recs = mock_evaluation_service.get_recommendations("run_123")
        
        assert len(recs) == 0


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
