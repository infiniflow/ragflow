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
RAG Evaluation Service

Provides functionality for evaluating RAG system performance including:
- Dataset management
- Test case management
- Evaluation execution
- Metrics computation
- Configuration recommendations
"""

import asyncio
import logging
import queue
import threading
from typing import List, Dict, Any, Optional, Tuple
from datetime import datetime
from timeit import default_timer as timer

from api.db.db_models import EvaluationDataset, EvaluationCase, EvaluationRun, EvaluationResult
from api.db.services.common_service import CommonService
from api.db.services.dialog_service import DialogService
from common.misc_utils import get_uuid
from common.time_utils import current_timestamp
from common.constants import StatusEnum


class EvaluationService(CommonService):
    """Service for managing RAG evaluations"""

    model = EvaluationDataset

    # ==================== Dataset Management ====================

    @classmethod
    def create_dataset(cls, name: str, description: str, kb_ids: List[str],
                      tenant_id: str, user_id: str) -> Tuple[bool, str]:
        """
        Create a new evaluation dataset.

        Args:
            name: Dataset name
            description: Dataset description
            kb_ids: List of knowledge base IDs to evaluate against
            tenant_id: Tenant ID
            user_id: User ID who creates the dataset

        Returns:
            (success, dataset_id or error_message)
        """
        try:
            timestamp= current_timestamp()
            dataset_id = get_uuid()
            dataset = {
                "id": dataset_id,
                "tenant_id": tenant_id,
                "name": name,
                "description": description,
                "kb_ids": kb_ids,
                "created_by": user_id,
                "create_time": timestamp,
                "update_time": timestamp,
                "status": StatusEnum.VALID.value
            }

            if not EvaluationDataset.create(**dataset):
                return False, "Failed to create dataset"

            return True, dataset_id
        except Exception as e:
            logging.error(f"Error creating evaluation dataset: {e}")
            return False, str(e)

    @classmethod
    def get_dataset(cls, dataset_id: str) -> Optional[Dict[str, Any]]:
        """Get dataset by ID"""
        try:
            dataset = EvaluationDataset.get_by_id(dataset_id)
            if dataset:
                return dataset.to_dict()
            return None
        except Exception as e:
            logging.error(f"Error getting dataset {dataset_id}: {e}")
            return None

    @classmethod
    def list_datasets(cls, tenant_id: str, user_id: str,
                     page: int = 1, page_size: int = 20) -> Dict[str, Any]:
        """List datasets for a tenant"""
        try:
            query = EvaluationDataset.select().where(
                (EvaluationDataset.tenant_id == tenant_id) &
                (EvaluationDataset.status == StatusEnum.VALID.value)
            ).order_by(EvaluationDataset.create_time.desc())

            total = query.count()
            datasets = query.paginate(page, page_size)

            return {
                "total": total,
                "datasets": [d.to_dict() for d in datasets]
            }
        except Exception as e:
            logging.error(f"Error listing datasets: {e}")
            return {"total": 0, "datasets": []}

    @classmethod
    def update_dataset(cls, dataset_id: str, **kwargs) -> bool:
        """Update dataset"""
        try:
            kwargs["update_time"] = current_timestamp()
            return EvaluationDataset.update(**kwargs).where(
                EvaluationDataset.id == dataset_id
            ).execute() > 0
        except Exception as e:
            logging.error(f"Error updating dataset {dataset_id}: {e}")
            return False

    @classmethod
    def delete_dataset(cls, dataset_id: str) -> bool:
        """Soft delete dataset"""
        try:
            return EvaluationDataset.update(
                status=StatusEnum.INVALID.value,
                update_time=current_timestamp()
            ).where(EvaluationDataset.id == dataset_id).execute() > 0
        except Exception as e:
            logging.error(f"Error deleting dataset {dataset_id}: {e}")
            return False

    # ==================== Test Case Management ====================

    @classmethod
    def add_test_case(cls, dataset_id: str, question: str,
                     reference_answer: Optional[str] = None,
                     relevant_doc_ids: Optional[List[str]] = None,
                     relevant_chunk_ids: Optional[List[str]] = None,
                     metadata: Optional[Dict[str, Any]] = None) -> Tuple[bool, str]:
        """
        Add a test case to a dataset.

        Args:
            dataset_id: Dataset ID
            question: Test question
            reference_answer: Optional ground truth answer
            relevant_doc_ids: Optional list of relevant document IDs
            relevant_chunk_ids: Optional list of relevant chunk IDs
            metadata: Optional additional metadata

        Returns:
            (success, case_id or error_message)
        """
        try:
            case_id = get_uuid()
            case = {
                "id": case_id,
                "dataset_id": dataset_id,
                "question": question,
                "reference_answer": reference_answer,
                "relevant_doc_ids": relevant_doc_ids,
                "relevant_chunk_ids": relevant_chunk_ids,
                "metadata": metadata,
                "create_time": current_timestamp()
            }

            if not EvaluationCase.create(**case):
                return False, "Failed to create test case"

            return True, case_id
        except Exception as e:
            logging.error(f"Error adding test case: {e}")
            return False, str(e)

    @classmethod
    def get_test_cases(cls, dataset_id: str) -> List[Dict[str, Any]]:
        """Get all test cases for a dataset"""
        try:
            cases = EvaluationCase.select().where(
                EvaluationCase.dataset_id == dataset_id
            ).order_by(EvaluationCase.create_time)

            return [c.to_dict() for c in cases]
        except Exception as e:
            logging.error(f"Error getting test cases for dataset {dataset_id}: {e}")
            return []

    @classmethod
    def delete_test_case(cls, case_id: str) -> bool:
        """Delete a test case"""
        try:
            return EvaluationCase.delete().where(
                EvaluationCase.id == case_id
            ).execute() > 0
        except Exception as e:
            logging.error(f"Error deleting test case {case_id}: {e}")
            return False

    @classmethod
    def import_test_cases(cls, dataset_id: str, cases: List[Dict[str, Any]]) -> Tuple[int, int]:
        """
        Bulk import test cases from a list.

        Args:
            dataset_id: Dataset ID
            cases: List of test case dictionaries

        Returns:
            (success_count, failure_count)
        """
        success_count = 0
        failure_count = 0
        case_instances = []
        
        if not cases:
            return success_count, failure_count
        
        cur_timestamp = current_timestamp()

        try:
            for case_data in cases:
                case_id = get_uuid()
                case_info = {
                    "id": case_id,
                    "dataset_id": dataset_id,
                    "question": case_data.get("question", ""),
                    "reference_answer": case_data.get("reference_answer"),
                    "relevant_doc_ids": case_data.get("relevant_doc_ids"),
                    "relevant_chunk_ids": case_data.get("relevant_chunk_ids"),
                    "metadata": case_data.get("metadata"),
                    "create_time": cur_timestamp
                }

                case_instances.append(EvaluationCase(**case_info))
            EvaluationCase.bulk_create(case_instances, batch_size=300)
            success_count = len(case_instances)
            failure_count = 0

        except Exception as e:
            logging.error(f"Error bulk importing test cases: {str(e)}")
            failure_count = len(cases)
            success_count = 0

        return success_count, failure_count

    # ==================== Evaluation Execution ====================

    @classmethod
    def start_evaluation(cls, dataset_id: str, dialog_id: str,
                        user_id: str, name: Optional[str] = None) -> Tuple[bool, str]:
        """
        Start an evaluation run.

        Args:
            dataset_id: Dataset ID
            dialog_id: Dialog configuration to evaluate
            user_id: User ID who starts the run
            name: Optional run name

        Returns:
            (success, run_id or error_message)
        """
        try:
            # Get dialog configuration
            success, dialog = DialogService.get_by_id(dialog_id)
            if not success:
                return False, "Dialog not found"

            # Create evaluation run
            run_id = get_uuid()
            if not name:
                name = f"Evaluation Run {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}"

            run = {
                "id": run_id,
                "dataset_id": dataset_id,
                "dialog_id": dialog_id,
                "name": name,
                "config_snapshot": dialog.to_dict(),
                "metrics_summary": None,
                "status": "RUNNING",
                "created_by": user_id,
                "create_time": current_timestamp(),
                "complete_time": None
            }

            if not EvaluationRun.create(**run):
                return False, "Failed to create evaluation run"

            # Execute evaluation asynchronously (in production, use task queue)
            # For now, we'll execute synchronously
            cls._execute_evaluation(run_id, dataset_id, dialog)

            return True, run_id
        except Exception as e:
            logging.error(f"Error starting evaluation: {e}")
            return False, str(e)

    @classmethod
    def _execute_evaluation(cls, run_id: str, dataset_id: str, dialog: Any):
        """
        Execute evaluation for all test cases.

        This method runs the RAG pipeline for each test case and computes metrics.
        """
        try:
            # Get all test cases
            test_cases = cls.get_test_cases(dataset_id)

            if not test_cases:
                EvaluationRun.update(
                    status="FAILED",
                    complete_time=current_timestamp()
                ).where(EvaluationRun.id == run_id).execute()
                return

            # Execute each test case
            results = []
            for case in test_cases:
                result = cls._evaluate_single_case(run_id, case, dialog)
                if result:
                    results.append(result)

            # Compute summary metrics
            metrics_summary = cls._compute_summary_metrics(results)

            # Update run status
            EvaluationRun.update(
                status="COMPLETED",
                metrics_summary=metrics_summary,
                complete_time=current_timestamp()
            ).where(EvaluationRun.id == run_id).execute()

        except Exception as e:
            logging.error(f"Error executing evaluation {run_id}: {e}")
            EvaluationRun.update(
                status="FAILED",
                complete_time=current_timestamp()
            ).where(EvaluationRun.id == run_id).execute()

    @classmethod
    def _evaluate_single_case(cls, run_id: str, case: Dict[str, Any],
                             dialog: Any) -> Optional[Dict[str, Any]]:
        """
        Evaluate a single test case.

        Args:
            run_id: Evaluation run ID
            case: Test case dictionary
            dialog: Dialog configuration

        Returns:
            Result dictionary or None if failed
        """
        try:
            # Prepare messages
            messages = [{"role": "user", "content": case["question"]}]

            # Execute RAG pipeline
            start_time = timer()
            answer = ""
            retrieved_chunks = []


            def _sync_from_async_gen(async_gen):
                result_queue: queue.Queue = queue.Queue()

                def runner():
                    loop = asyncio.new_event_loop()
                    asyncio.set_event_loop(loop)

                    async def consume():
                        try:
                            async for item in async_gen:
                                result_queue.put(item)
                        except Exception as e:
                            result_queue.put(e)
                        finally:
                            result_queue.put(StopIteration)

                    loop.run_until_complete(consume())
                    loop.close()

                threading.Thread(target=runner, daemon=True).start()

                while True:
                    item = result_queue.get()
                    if item is StopIteration:
                        break
                    if isinstance(item, Exception):
                        raise item
                    yield item


            def chat(dialog, messages, stream=True, **kwargs):
                from api.db.services.dialog_service import async_chat

                return _sync_from_async_gen(async_chat(dialog, messages, stream=stream, **kwargs))

            for ans in chat(dialog, messages, stream=False):
                if isinstance(ans, dict):
                    answer = ans.get("answer", "")
                    retrieved_chunks = ans.get("reference", {}).get("chunks", [])
                    break

            execution_time = timer() - start_time

            # Compute metrics
            metrics = cls._compute_metrics(
                question=case["question"],
                generated_answer=answer,
                reference_answer=case.get("reference_answer"),
                retrieved_chunks=retrieved_chunks,
                relevant_chunk_ids=case.get("relevant_chunk_ids"),
                dialog=dialog
            )

            # Save result
            result_id = get_uuid()
            result = {
                "id": result_id,
                "run_id": run_id,
                "case_id": case["id"],
                "generated_answer": answer,
                "retrieved_chunks": retrieved_chunks,
                "metrics": metrics,
                "execution_time": execution_time,
                "token_usage": None,  # TODO: Track token usage
                "create_time": current_timestamp()
            }

            EvaluationResult.create(**result)

            return result
        except Exception as e:
            logging.error(f"Error evaluating case {case.get('id')}: {e}")
            return None

    @classmethod
    def _compute_metrics(cls, question: str, generated_answer: str,
                        reference_answer: Optional[str],
                        retrieved_chunks: List[Dict[str, Any]],
                        relevant_chunk_ids: Optional[List[str]],
                        dialog: Any) -> Dict[str, float]:
        """
        Compute evaluation metrics for a single test case.

        Returns:
            Dictionary of metric names to values
        """
        metrics = {}

        # Retrieval metrics (if ground truth chunks provided)
        if relevant_chunk_ids:
            retrieved_ids = [c.get("chunk_id") for c in retrieved_chunks]
            metrics.update(cls._compute_retrieval_metrics(retrieved_ids, relevant_chunk_ids))

        # Generation metrics
        if generated_answer:
            # Basic metrics
            metrics["answer_length"] = len(generated_answer)
            metrics["has_answer"] = 1.0 if generated_answer.strip() else 0.0

            # TODO: Implement advanced metrics using LLM-as-judge
            # - Faithfulness (hallucination detection)
            # - Answer relevance
            # - Context relevance
            # - Semantic similarity (if reference answer provided)

        return metrics

    @classmethod
    def _compute_retrieval_metrics(cls, retrieved_ids: List[str],
                                   relevant_ids: List[str]) -> Dict[str, float]:
        """
        Compute retrieval metrics.

        Args:
            retrieved_ids: List of retrieved chunk IDs
            relevant_ids: List of relevant chunk IDs (ground truth)

        Returns:
            Dictionary of retrieval metrics
        """
        if not relevant_ids:
            return {}

        retrieved_set = set(retrieved_ids)
        relevant_set = set(relevant_ids)

        # Precision: proportion of retrieved that are relevant
        precision = len(retrieved_set & relevant_set) / len(retrieved_set) if retrieved_set else 0.0

        # Recall: proportion of relevant that were retrieved
        recall = len(retrieved_set & relevant_set) / len(relevant_set) if relevant_set else 0.0

        # F1 score
        f1 = 2 * (precision * recall) / (precision + recall) if (precision + recall) > 0 else 0.0

        # Hit rate: whether any relevant chunk was retrieved
        hit_rate = 1.0 if (retrieved_set & relevant_set) else 0.0

        # MRR (Mean Reciprocal Rank): position of first relevant chunk
        mrr = 0.0
        for i, chunk_id in enumerate(retrieved_ids, 1):
            if chunk_id in relevant_set:
                mrr = 1.0 / i
                break

        return {
            "precision": precision,
            "recall": recall,
            "f1_score": f1,
            "hit_rate": hit_rate,
            "mrr": mrr
        }

    @classmethod
    def _compute_summary_metrics(cls, results: List[Dict[str, Any]]) -> Dict[str, Any]:
        """
        Compute summary metrics across all test cases.

        Args:
            results: List of result dictionaries

        Returns:
            Summary metrics dictionary
        """
        if not results:
            return {}

        # Aggregate metrics
        metric_sums = {}
        metric_counts = {}

        for result in results:
            metrics = result.get("metrics", {})
            for key, value in metrics.items():
                if isinstance(value, (int, float)):
                    metric_sums[key] = metric_sums.get(key, 0) + value
                    metric_counts[key] = metric_counts.get(key, 0) + 1

        # Compute averages
        summary = {
            "total_cases": len(results),
            "avg_execution_time": sum(r.get("execution_time", 0) for r in results) / len(results)
        }

        for key in metric_sums:
            summary[f"avg_{key}"] = metric_sums[key] / metric_counts[key]

        return summary

    # ==================== Results & Analysis ====================

    @classmethod
    def get_run_results(cls, run_id: str) -> Dict[str, Any]:
        """Get results for an evaluation run"""
        try:
            run = EvaluationRun.get_by_id(run_id)
            if not run:
                return {}

            results = EvaluationResult.select().where(
                EvaluationResult.run_id == run_id
            ).order_by(EvaluationResult.create_time)

            return {
                "run": run.to_dict(),
                "results": [r.to_dict() for r in results]
            }
        except Exception as e:
            logging.error(f"Error getting run results {run_id}: {e}")
            return {}

    @classmethod
    def get_recommendations(cls, run_id: str) -> List[Dict[str, Any]]:
        """
        Analyze evaluation results and provide configuration recommendations.

        Args:
            run_id: Evaluation run ID

        Returns:
            List of recommendation dictionaries
        """
        try:
            run = EvaluationRun.get_by_id(run_id)
            if not run or not run.metrics_summary:
                return []

            metrics = run.metrics_summary
            recommendations = []

            # Low precision: retrieving irrelevant chunks
            if metrics.get("avg_precision", 1.0) < 0.7:
                recommendations.append({
                    "issue": "Low Precision",
                    "severity": "high",
                    "description": "System is retrieving many irrelevant chunks",
                    "suggestions": [
                        "Increase similarity_threshold to filter out less relevant chunks",
                        "Enable reranking to improve chunk ordering",
                        "Reduce top_k to return fewer chunks"
                    ]
                })

            # Low recall: missing relevant chunks
            if metrics.get("avg_recall", 1.0) < 0.7:
                recommendations.append({
                    "issue": "Low Recall",
                    "severity": "high",
                    "description": "System is missing relevant chunks",
                    "suggestions": [
                        "Increase top_k to retrieve more chunks",
                        "Lower similarity_threshold to be more inclusive",
                        "Enable hybrid search (keyword + semantic)",
                        "Check chunk size - may be too large or too small"
                    ]
                })

            # Slow response time
            if metrics.get("avg_execution_time", 0) > 5.0:
                recommendations.append({
                    "issue": "Slow Response Time",
                    "severity": "medium",
                    "description": f"Average response time is {metrics['avg_execution_time']:.2f}s",
                    "suggestions": [
                        "Reduce top_k to retrieve fewer chunks",
                        "Optimize embedding model selection",
                        "Consider caching frequently asked questions"
                    ]
                })

            return recommendations
        except Exception as e:
            logging.error(f"Error generating recommendations for run {run_id}: {e}")
            return []
