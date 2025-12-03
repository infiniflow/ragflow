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
Task management API endpoints for checkpoint/resume functionality.
"""

from flask import request
from flask_login import login_required
from api.db.services.checkpoint_service import CheckpointService
from api.db.services.task_service import TaskService
from api.utils.api_utils import server_error_response, get_data_error_result, get_json_result
from api.settings import RetCode
import logging


# This will be registered in the main app
def register_task_routes(app):
    """Register task management routes"""
    
    @app.route('/api/v1/task/<task_id>/pause', methods=['POST'])
    @login_required
    def pause_task(task_id):
        """
        Pause a running task.
        
        Only works for tasks that support checkpointing (RAPTOR, GraphRAG).
        The task will pause after completing the current document.
        
        Args:
            task_id: Task ID
            
        Returns:
            Success/error response
        """
        try:
            # Get task
            task = TaskService.query(id=task_id)
            if not task:
                return get_data_error_result(
                    message="Task not found",
                    code=RetCode.DATA_ERROR
                )
            
            # Check if task supports pause
            if not task[0].get("can_pause", False):
                return get_data_error_result(
                    message="This task does not support pause/resume",
                    code=RetCode.OPERATING_ERROR
                )
            
            # Get checkpoint
            checkpoint = CheckpointService.get_by_task_id(task_id)
            if not checkpoint:
                return get_data_error_result(
                    message="No checkpoint found for this task",
                    code=RetCode.DATA_ERROR
                )
            
            # Check if already paused
            if checkpoint.status == "paused":
                return get_data_error_result(
                    message="Task is already paused",
                    code=RetCode.OPERATING_ERROR
                )
            
            # Check if already completed
            if checkpoint.status in ["completed", "cancelled"]:
                return get_data_error_result(
                    message=f"Cannot pause a {checkpoint.status} task",
                    code=RetCode.OPERATING_ERROR
                )
            
            # Pause checkpoint
            success = CheckpointService.pause_checkpoint(checkpoint.id)
            if not success:
                return get_data_error_result(
                    message="Failed to pause task",
                    code=RetCode.OPERATING_ERROR
                )
            
            # Update task
            TaskService.update_by_id(task_id, {"is_paused": True})
            
            logging.info(f"Task {task_id} paused successfully")
            
            return get_json_result(data={
                "task_id": task_id,
                "status": "paused",
                "message": "Task will pause after completing current document"
            })
            
        except Exception as e:
            logging.error(f"Error pausing task {task_id}: {e}")
            return server_error_response(e)
    
    
    @app.route('/api/v1/task/<task_id>/resume', methods=['POST'])
    @login_required
    def resume_task(task_id):
        """
        Resume a paused task.
        
        The task will continue from where it left off, processing only
        the remaining documents.
        
        Args:
            task_id: Task ID
            
        Returns:
            Success/error response
        """
        try:
            # Get task
            task = TaskService.query(id=task_id)
            if not task:
                return get_data_error_result(
                    message="Task not found",
                    code=RetCode.DATA_ERROR
                )
            
            # Get checkpoint
            checkpoint = CheckpointService.get_by_task_id(task_id)
            if not checkpoint:
                return get_data_error_result(
                    message="No checkpoint found for this task",
                    code=RetCode.DATA_ERROR
                )
            
            # Check if paused
            if checkpoint.status != "paused":
                return get_data_error_result(
                    message=f"Cannot resume a {checkpoint.status} task",
                    code=RetCode.OPERATING_ERROR
                )
            
            # Resume checkpoint
            success = CheckpointService.resume_checkpoint(checkpoint.id)
            if not success:
                return get_data_error_result(
                    message="Failed to resume task",
                    code=RetCode.OPERATING_ERROR
                )
            
            # Update task
            TaskService.update_by_id(task_id, {"is_paused": False})
            
            # Get pending documents count
            pending_docs = CheckpointService.get_pending_documents(checkpoint.id)
            
            logging.info(f"Task {task_id} resumed successfully")
            
            return get_json_result(data={
                "task_id": task_id,
                "status": "running",
                "pending_documents": len(pending_docs),
                "message": f"Task resumed, {len(pending_docs)} documents remaining"
            })
            
        except Exception as e:
            logging.error(f"Error resuming task {task_id}: {e}")
            return server_error_response(e)
    
    
    @app.route('/api/v1/task/<task_id>/cancel', methods=['POST'])
    @login_required
    def cancel_task(task_id):
        """
        Cancel a running or paused task.
        
        The task will stop after completing the current document.
        All progress is preserved in the checkpoint.
        
        Args:
            task_id: Task ID
            
        Returns:
            Success/error response
        """
        try:
            # Get task
            task = TaskService.query(id=task_id)
            if not task:
                return get_data_error_result(
                    message="Task not found",
                    code=RetCode.DATA_ERROR
                )
            
            # Get checkpoint
            checkpoint = CheckpointService.get_by_task_id(task_id)
            if not checkpoint:
                return get_data_error_result(
                    message="No checkpoint found for this task",
                    code=RetCode.DATA_ERROR
                )
            
            # Check if already cancelled or completed
            if checkpoint.status in ["cancelled", "completed"]:
                return get_data_error_result(
                    message=f"Task is already {checkpoint.status}",
                    code=RetCode.OPERATING_ERROR
                )
            
            # Cancel checkpoint
            success = CheckpointService.cancel_checkpoint(checkpoint.id)
            if not success:
                return get_data_error_result(
                    message="Failed to cancel task",
                    code=RetCode.OPERATING_ERROR
                )
            
            logging.info(f"Task {task_id} cancelled successfully")
            
            return get_json_result(data={
                "task_id": task_id,
                "status": "cancelled",
                "message": "Task will stop after completing current document"
            })
            
        except Exception as e:
            logging.error(f"Error cancelling task {task_id}: {e}")
            return server_error_response(e)
    
    
    @app.route('/api/v1/task/<task_id>/checkpoint-status', methods=['GET'])
    @login_required
    def get_checkpoint_status(task_id):
        """
        Get detailed checkpoint status for a task.
        
        Returns progress, document counts, token usage, and timestamps.
        
        Args:
            task_id: Task ID
            
        Returns:
            Checkpoint status details
        """
        try:
            # Get checkpoint
            checkpoint = CheckpointService.get_by_task_id(task_id)
            if not checkpoint:
                return get_data_error_result(
                    message="No checkpoint found for this task",
                    code=RetCode.DATA_ERROR
                )
            
            # Get detailed status
            status = CheckpointService.get_checkpoint_status(checkpoint.id)
            if not status:
                return get_data_error_result(
                    message="Failed to retrieve checkpoint status",
                    code=RetCode.OPERATING_ERROR
                )
            
            # Get failed documents details
            failed_docs = CheckpointService.get_failed_documents(checkpoint.id)
            status["failed_documents_details"] = failed_docs
            
            return get_json_result(data=status)
            
        except Exception as e:
            logging.error(f"Error getting checkpoint status for task {task_id}: {e}")
            return server_error_response(e)
    
    
    @app.route('/api/v1/task/<task_id>/retry-failed', methods=['POST'])
    @login_required
    def retry_failed_documents(task_id):
        """
        Retry all failed documents in a task.
        
        Resets failed documents to pending status so they will be
        retried when the task is resumed or restarted.
        
        Args:
            task_id: Task ID
            
        Request body (optional):
            {
                "doc_ids": ["doc1", "doc2"]  // Specific docs to retry, or all if omitted
            }
            
        Returns:
            Success/error response with retry count
        """
        try:
            # Get checkpoint
            checkpoint = CheckpointService.get_by_task_id(task_id)
            if not checkpoint:
                return get_data_error_result(
                    message="No checkpoint found for this task",
                    code=RetCode.DATA_ERROR
                )
            
            # Get request data
            req = request.json or {}
            specific_docs = req.get("doc_ids", [])
            
            # Get failed documents
            failed_docs = CheckpointService.get_failed_documents(checkpoint.id)
            
            if not failed_docs:
                return get_data_error_result(
                    message="No failed documents to retry",
                    code=RetCode.DATA_ERROR
                )
            
            # Filter by specific docs if provided
            if specific_docs:
                failed_docs = [d for d in failed_docs if d["doc_id"] in specific_docs]
            
            # Reset each failed document
            retry_count = 0
            skipped_count = 0
            
            for doc in failed_docs:
                doc_id = doc["doc_id"]
                
                # Check if should retry (max retries)
                if CheckpointService.should_retry(checkpoint.id, doc_id, max_retries=3):
                    success = CheckpointService.reset_document_for_retry(checkpoint.id, doc_id)
                    if success:
                        retry_count += 1
                    else:
                        logging.warning(f"Failed to reset document {doc_id} for retry")
                else:
                    skipped_count += 1
                    logging.info(f"Document {doc_id} exceeded max retries, skipping")
            
            logging.info(f"Task {task_id}: Reset {retry_count} documents for retry, skipped {skipped_count}")
            
            return get_json_result(data={
                "task_id": task_id,
                "retried": retry_count,
                "skipped": skipped_count,
                "message": f"Reset {retry_count} documents for retry"
            })
            
        except Exception as e:
            logging.error(f"Error retrying failed documents for task {task_id}: {e}")
            return server_error_response(e)
