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
Checkpoint service for managing task checkpoints and resume functionality.
"""

import logging
from datetime import datetime
from typing import Optional, Dict, List, Any
from api.db.db_models import TaskCheckpoint
from api.db.services.common_service import CommonService
from common.misc_utils import get_uuid


class CheckpointService(CommonService):
    """Service for managing task checkpoints"""
    
    model = TaskCheckpoint
    
    @classmethod
    def create_checkpoint(
        cls,
        task_id: str,
        task_type: str,
        doc_ids: List[str],
        config: Dict[str, Any]
    ) -> TaskCheckpoint:
        """
        Create a new checkpoint for a task.
        
        Args:
            task_id: Task ID
            task_type: Type of task ("raptor" or "graphrag")
            doc_ids: List of document IDs to process
            config: Task configuration
            
        Returns:
            Created TaskCheckpoint instance
        """
        checkpoint_id = get_uuid()
        
        # Initialize document states
        doc_states = {}
        for doc_id in doc_ids:
            doc_states[doc_id] = {
                "status": "pending",
                "token_count": 0,
                "chunks": 0,
                "retry_count": 0
            }
        
        checkpoint_data = {
            "doc_states": doc_states,
            "config": config,
            "metadata": {
                "created_at": datetime.now().isoformat()
            }
        }
        
        checkpoint = cls.model(
            id=checkpoint_id,
            task_id=task_id,
            task_type=task_type,
            status="pending",
            total_documents=len(doc_ids),
            completed_documents=0,
            failed_documents=0,
            pending_documents=len(doc_ids),
            overall_progress=0.0,
            token_count=0,
            checkpoint_data=checkpoint_data,
            started_at=datetime.now(),
            last_checkpoint_at=datetime.now()
        )
        checkpoint.save(force_insert=True)
        
        logging.info(f"Created checkpoint {checkpoint_id} for task {task_id} with {len(doc_ids)} documents")
        return checkpoint
    
    @classmethod
    def get_by_task_id(cls, task_id: str) -> Optional[TaskCheckpoint]:
        """Get checkpoint by task ID"""
        try:
            return cls.model.get(cls.model.task_id == task_id)
        except Exception:
            return None
    
    @classmethod
    def save_document_completion(
        cls,
        checkpoint_id: str,
        doc_id: str,
        token_count: int = 0,
        chunks: int = 0
    ) -> bool:
        """
        Save completion of a single document.
        
        Args:
            checkpoint_id: Checkpoint ID
            doc_id: Document ID
            token_count: Tokens consumed for this document
            chunks: Number of chunks generated
            
        Returns:
            True if successful
        """
        try:
            checkpoint = cls.model.get_by_id(checkpoint_id)
            
            # Update document state
            doc_states = checkpoint.checkpoint_data.get("doc_states", {})
            if doc_id in doc_states:
                doc_states[doc_id] = {
                    "status": "completed",
                    "token_count": token_count,
                    "chunks": chunks,
                    "completed_at": datetime.now().isoformat(),
                    "retry_count": doc_states[doc_id].get("retry_count", 0)
                }
            
            # Update counters
            completed = sum(1 for s in doc_states.values() if s["status"] == "completed")
            failed = sum(1 for s in doc_states.values() if s["status"] == "failed")
            pending = sum(1 for s in doc_states.values() if s["status"] == "pending")
            total_tokens = sum(s.get("token_count", 0) for s in doc_states.values())
            
            progress = completed / checkpoint.total_documents if checkpoint.total_documents > 0 else 0.0
            
            # Update checkpoint
            checkpoint.checkpoint_data["doc_states"] = doc_states
            checkpoint.completed_documents = completed
            checkpoint.failed_documents = failed
            checkpoint.pending_documents = pending
            checkpoint.overall_progress = progress
            checkpoint.token_count = total_tokens
            checkpoint.last_checkpoint_at = datetime.now()
            
            # Check if all documents are done
            if pending == 0:
                checkpoint.status = "completed"
                checkpoint.completed_at = datetime.now()
            
            checkpoint.save()
            
            logging.info(f"Checkpoint {checkpoint_id}: Document {doc_id} completed ({completed}/{checkpoint.total_documents})")
            return True
            
        except Exception as e:
            logging.error(f"Failed to save document completion: {e}")
            return False
    
    @classmethod
    def save_document_failure(
        cls,
        checkpoint_id: str,
        doc_id: str,
        error: str
    ) -> bool:
        """
        Save failure of a single document.
        
        Args:
            checkpoint_id: Checkpoint ID
            doc_id: Document ID
            error: Error message
            
        Returns:
            True if successful
        """
        try:
            checkpoint = cls.model.get_by_id(checkpoint_id)
            
            # Update document state
            doc_states = checkpoint.checkpoint_data.get("doc_states", {})
            if doc_id in doc_states:
                retry_count = doc_states[doc_id].get("retry_count", 0) + 1
                doc_states[doc_id] = {
                    "status": "failed",
                    "error": error,
                    "retry_count": retry_count,
                    "last_attempt": datetime.now().isoformat()
                }
            
            # Update counters
            completed = sum(1 for s in doc_states.values() if s["status"] == "completed")
            failed = sum(1 for s in doc_states.values() if s["status"] == "failed")
            pending = sum(1 for s in doc_states.values() if s["status"] == "pending")
            
            # Update checkpoint
            checkpoint.checkpoint_data["doc_states"] = doc_states
            checkpoint.completed_documents = completed
            checkpoint.failed_documents = failed
            checkpoint.pending_documents = pending
            checkpoint.last_checkpoint_at = datetime.now()
            checkpoint.save()
            
            logging.warning(f"Checkpoint {checkpoint_id}: Document {doc_id} failed: {error}")
            return True
            
        except Exception as e:
            logging.error(f"Failed to save document failure: {e}")
            return False
    
    @classmethod
    def get_pending_documents(cls, checkpoint_id: str) -> List[str]:
        """Get list of pending document IDs"""
        try:
            checkpoint = cls.model.get_by_id(checkpoint_id)
            doc_states = checkpoint.checkpoint_data.get("doc_states", {})
            return [doc_id for doc_id, state in doc_states.items() if state["status"] == "pending"]
        except Exception as e:
            logging.error(f"Failed to get pending documents: {e}")
            return []
    
    @classmethod
    def get_failed_documents(cls, checkpoint_id: str) -> List[Dict[str, Any]]:
        """Get list of failed documents with details"""
        try:
            checkpoint = cls.model.get_by_id(checkpoint_id)
            doc_states = checkpoint.checkpoint_data.get("doc_states", {})
            failed = []
            for doc_id, state in doc_states.items():
                if state["status"] == "failed":
                    failed.append({
                        "doc_id": doc_id,
                        "error": state.get("error", "Unknown error"),
                        "retry_count": state.get("retry_count", 0),
                        "last_attempt": state.get("last_attempt")
                    })
            return failed
        except Exception as e:
            logging.error(f"Failed to get failed documents: {e}")
            return []
    
    @classmethod
    def pause_checkpoint(cls, checkpoint_id: str) -> bool:
        """Mark checkpoint as paused"""
        try:
            checkpoint = cls.model.get_by_id(checkpoint_id)
            checkpoint.status = "paused"
            checkpoint.paused_at = datetime.now()
            checkpoint.save()
            logging.info(f"Checkpoint {checkpoint_id} paused")
            return True
        except Exception as e:
            logging.error(f"Failed to pause checkpoint: {e}")
            return False
    
    @classmethod
    def resume_checkpoint(cls, checkpoint_id: str) -> bool:
        """Mark checkpoint as resumed"""
        try:
            checkpoint = cls.model.get_by_id(checkpoint_id)
            checkpoint.status = "running"
            checkpoint.resumed_at = datetime.now()
            checkpoint.save()
            logging.info(f"Checkpoint {checkpoint_id} resumed")
            return True
        except Exception as e:
            logging.error(f"Failed to resume checkpoint: {e}")
            return False
    
    @classmethod
    def cancel_checkpoint(cls, checkpoint_id: str) -> bool:
        """Mark checkpoint as cancelled"""
        try:
            checkpoint = cls.model.get_by_id(checkpoint_id)
            checkpoint.status = "cancelled"
            checkpoint.save()
            logging.info(f"Checkpoint {checkpoint_id} cancelled")
            return True
        except Exception as e:
            logging.error(f"Failed to cancel checkpoint: {e}")
            return False
    
    @classmethod
    def is_paused(cls, checkpoint_id: str) -> bool:
        """Check if checkpoint is paused"""
        try:
            checkpoint = cls.model.get_by_id(checkpoint_id)
            return checkpoint.status == "paused"
        except Exception:
            return False
    
    @classmethod
    def is_cancelled(cls, checkpoint_id: str) -> bool:
        """Check if checkpoint is cancelled"""
        try:
            checkpoint = cls.model.get_by_id(checkpoint_id)
            return checkpoint.status == "cancelled"
        except Exception:
            return False
    
    @classmethod
    def should_retry(cls, checkpoint_id: str, doc_id: str, max_retries: int = 3) -> bool:
        """Check if document should be retried"""
        try:
            checkpoint = cls.model.get_by_id(checkpoint_id)
            doc_states = checkpoint.checkpoint_data.get("doc_states", {})
            if doc_id in doc_states:
                retry_count = doc_states[doc_id].get("retry_count", 0)
                return retry_count < max_retries
            return False
        except Exception:
            return False
    
    @classmethod
    def reset_document_for_retry(cls, checkpoint_id: str, doc_id: str) -> bool:
        """Reset a failed document to pending for retry"""
        try:
            checkpoint = cls.model.get_by_id(checkpoint_id)
            doc_states = checkpoint.checkpoint_data.get("doc_states", {})
            
            if doc_id in doc_states and doc_states[doc_id]["status"] == "failed":
                retry_count = doc_states[doc_id].get("retry_count", 0)
                doc_states[doc_id] = {
                    "status": "pending",
                    "token_count": 0,
                    "chunks": 0,
                    "retry_count": retry_count  # Keep retry count
                }
                
                # Update counters
                failed = sum(1 for s in doc_states.values() if s["status"] == "failed")
                pending = sum(1 for s in doc_states.values() if s["status"] == "pending")
                
                checkpoint.checkpoint_data["doc_states"] = doc_states
                checkpoint.failed_documents = failed
                checkpoint.pending_documents = pending
                checkpoint.save()
                
                logging.info(f"Reset document {doc_id} for retry (attempt {retry_count + 1})")
                return True
            return False
        except Exception as e:
            logging.error(f"Failed to reset document for retry: {e}")
            return False
    
    @classmethod
    def get_checkpoint_status(cls, checkpoint_id: str) -> Optional[Dict[str, Any]]:
        """Get detailed checkpoint status"""
        try:
            checkpoint = cls.model.get_by_id(checkpoint_id)
            return {
                "checkpoint_id": checkpoint.id,
                "task_id": checkpoint.task_id,
                "task_type": checkpoint.task_type,
                "status": checkpoint.status,
                "progress": checkpoint.overall_progress,
                "total_documents": checkpoint.total_documents,
                "completed_documents": checkpoint.completed_documents,
                "failed_documents": checkpoint.failed_documents,
                "pending_documents": checkpoint.pending_documents,
                "token_count": checkpoint.token_count,
                "started_at": checkpoint.started_at.isoformat() if checkpoint.started_at else None,
                "paused_at": checkpoint.paused_at.isoformat() if checkpoint.paused_at else None,
                "resumed_at": checkpoint.resumed_at.isoformat() if checkpoint.resumed_at else None,
                "completed_at": checkpoint.completed_at.isoformat() if checkpoint.completed_at else None,
                "last_checkpoint_at": checkpoint.last_checkpoint_at.isoformat() if checkpoint.last_checkpoint_at else None,
                "error_message": checkpoint.error_message
            }
        except Exception as e:
            logging.error(f"Failed to get checkpoint status: {e}")
            return None
