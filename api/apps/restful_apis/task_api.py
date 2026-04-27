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

from api.apps import login_required
from api.db.services.task_service import TaskService, CANVAS_DEBUG_DOC_ID, GRAPH_RAPTOR_FAKE_DOC_ID
from api.utils.api_utils import (
    get_data_error_result,
    get_json_result,
    get_request_json,
    validate_request,
)
from common.constants import RetCode, TaskStatus
from rag.utils.redis_conn import REDIS_CONN


@manager.route("/tasks/<task_id>/cancel", methods=["POST"])  # noqa: F821
@login_required
async def cancel_task(task_id):
    """Cancel a running task.

    Sets a Redis cancel flag, updates the task progress to -1 (cancelled),
    and marks the associated document's run status as CANCEL if applicable.
    """
    exists, task = TaskService.get_by_id(task_id)
    if not exists:
        return get_data_error_result(
            code=RetCode.NOT_FOUND,
            message=f"Task '{task_id}' not found.",
        )

    # A task is stoppable if it hasn't completed (progress < 1) and isn't already
    # in a failed/cancelled state (progress >= 0).  progress == -1 means the task
    # previously failed or was cancelled.
    if task.progress < 0:
        return get_data_error_result(
            message="Task is already in a cancelled or failed state.",
        )
    if task.progress >= 1:
        return get_data_error_result(
            message="Task has already completed and cannot be stopped.",
        )

    try:
        REDIS_CONN.set(f"{task_id}-cancel", "x")
    except Exception as e:
        logging.exception(f"Failed to set cancel flag for task {task_id}: %s", str(e))
        return get_json_result(
            code=RetCode.CONNECTION_ERROR,
            message=f"Failed to stop task",
        )

    # Append a cancellation message so the user can see it in progress_msg.
    try:
        cancel_msg = f"\n{datetime.now().strftime('%H:%M:%S')} Task stopped by user."
        TaskService.model.update(
            progress_msg=TaskService.model.progress_msg + cancel_msg,
            progress=-1,
        ).where(TaskService.model.id == task_id).execute()
    except Exception as e:
        logging.exception(f"Failed to update task {task_id} progress after cancellation, %s", str(e))

    # If the task belongs to a document, also mark the document's run status as
    # cancelled so that the UI reflects the state correctly.
    try:
        from api.db.services.document_service import DocumentService
        doc_id = task.doc_id
        if doc_id and doc_id not in (CANVAS_DEBUG_DOC_ID, GRAPH_RAPTOR_FAKE_DOC_ID):
            _, doc = DocumentService.get_by_id(doc_id)
            if doc and str(doc.run) in (TaskStatus.RUNNING.value, TaskStatus.SCHEDULE.value):
                DocumentService.update_by_id(doc_id, {"run": TaskStatus.CANCEL.value, "progress": 0})
    except Exception as e:
        logging.exception(f"Failed to update document run status for task {task_id}: %s", str(e))

    return get_json_result(data=True)


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

    return await cancel_task(task_id)
