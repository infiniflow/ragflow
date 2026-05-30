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
import logging
from datetime import datetime

from api.apps import current_user, login_required
from api.db.services.api_service import API4ConversationService
from api.db.services.document_service import DocumentService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.task_service import TaskService, CANVAS_DEBUG_DOC_ID, GRAPH_RAPTOR_FAKE_DOC_ID
from api.utils.api_utils import (
    get_json_result,
    get_request_json,
    validate_request,
)
from common.constants import RetCode, TaskStatus
from rag.utils.redis_conn import REDIS_CONN

LOGGER = logging.getLogger(__name__)

_INDEX_TASK_FIELDS = ("graphrag_task_id", "raptor_task_id", "mindmap_task_id")


def _is_document_task(doc_id):
    return doc_id and doc_id not in (CANVAS_DEBUG_DOC_ID, GRAPH_RAPTOR_FAKE_DOC_ID)


def _is_index_task_accessible(task_id, user_id):
    for task_field in _INDEX_TASK_FIELDS:
        kb = KnowledgebaseService.get_or_none(**{task_field: task_id})
        if kb:
            accessible = KnowledgebaseService.accessible(kb.id, user_id)
            LOGGER.debug(
                "Resolved index task authorization: task_id=%s user_id=%s task_field=%s kb_id=%s accessible=%s",
                task_id,
                user_id,
                task_field,
                kb.id,
                accessible,
            )
            return accessible
    return False


def _is_canvas_task_accessible(task_id, user_id):
    try:
        conversations = API4ConversationService.get_workflow_conversations_by_message_id(user_id, task_id)
        for conversation in conversations:
            messages = conversation.message or []
            if any(isinstance(message, dict) and message.get("id") == task_id for message in messages):
                LOGGER.debug(
                    "Resolved canvas task authorization: task_id=%s user_id=%s accessible=%s",
                    task_id,
                    user_id,
                    True,
                )
                return True
        LOGGER.debug(
            "Resolved canvas task authorization: task_id=%s user_id=%s accessible=%s",
            task_id,
            user_id,
            False,
        )
    except Exception as e:
        LOGGER.warning(
            "Failed to resolve canvas task authorization: task_id=%s user_id=%s error=%s",
            task_id,
            user_id,
            str(e),
            exc_info=True,
        )
    return False


def _can_cancel_task(task_id, task, user_id):
    if _is_document_task(task.doc_id):
        return DocumentService.accessible(task.doc_id, user_id)
    if task.doc_id == GRAPH_RAPTOR_FAKE_DOC_ID:
        return _is_index_task_accessible(task_id, user_id)
    if task.doc_id == CANVAS_DEBUG_DOC_ID:
        return _is_canvas_task_accessible(task_id, user_id)
    return False


@manager.route("/tasks/<task_id>/cancel", methods=["POST"])  # noqa: F821
@login_required
async def cancel_task(task_id):
    """Cancel a running task."""
    return await _cancel_task(task_id)


@manager.route("/tasks/<task_id>", methods=["PATCH"])  # noqa: F821
@login_required
@validate_request("action")
async def patch_task(task_id):
    req = await get_request_json()
    action = req.get("action")

    if action != "stop":
        return get_json_result(
            code=RetCode.ARGUMENT_ERROR,
            message=f"Invalid action '{action}'. Only 'stop' is supported.",
        )

    return await _cancel_task(task_id)


async def _cancel_task(task_id):
    """
    Sets a Redis cancel flag, updates the task progress to -1 (cancelled),
        and marks the associated document's run status as CANCEL if applicable.
    """
    exists, task = TaskService.get_by_id(task_id)
    if not exists:
        return get_json_result(data=True)

    doc_id = task.doc_id
    if not _can_cancel_task(task_id, task, current_user.id):
        logging.warning(
            "Unauthorized task cancellation attempt: task_id=%s doc_id=%s user_id=%s",
            task_id,
            doc_id,
            current_user.id,
        )
        return get_json_result(data=False, message="No authorization.", code=RetCode.AUTHENTICATION_ERROR)

    try:
        REDIS_CONN.set(f"{task_id}-cancel", "x")
    except Exception as e:
        logging.exception("Failed to set cancel flag for task %s: %s", task_id, str(e))
        return get_json_result(
            code=RetCode.CONNECTION_ERROR,
            message="Failed to stop task",
        )

    # Append a cancellation message so the user can see it in progress_msg.
    try:
        cancel_msg = f"\n{datetime.now().strftime('%H:%M:%S')} Task stopped by user."
        # Only transition to -1 if the task is still in a non-terminal state,
        # mirroring TaskService.update_progress semantics.
        TaskService.model.update(
            progress_msg=TaskService.model.progress_msg + cancel_msg,
            progress=-1,
        ).where((TaskService.model.id == task_id) & (TaskService.model.progress >= 0) & (TaskService.model.progress < 1)).execute()
    except Exception as e:
        logging.warning("Failed to update task %s progress after cancellation: %s", task_id, str(e))

    # If the task belongs to a document, also mark the document's run status as
    # cancelled so that the UI reflects the state correctly.
    try:
        if _is_document_task(doc_id):
            _, doc = DocumentService.get_by_id(doc_id)
            if doc and str(doc.run) in (TaskStatus.RUNNING.value, TaskStatus.SCHEDULE.value):
                DocumentService.update_by_id(doc_id, {"run": TaskStatus.CANCEL.value, "progress": 0})
    except Exception as e:
        logging.warning("Failed to update document run status for task %s: %s", task_id, str(e))

    logging.info(f"Cancel task succeeded: task_id={task_id} doc_id={task.doc_id}")
    return get_json_result(data=True)
