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
import json
import logging
from quart import request, make_response
from api.db.services.canvas_service import UserCanvasService, API4ConversationService
from api.db.services.file_service import FileService
from api.db.services.user_canvas_version import UserCanvasVersionService
from common.constants import RetCode
from common.misc_utils import get_uuid
from api.utils.api_utils import (
    get_json_result,
    server_error_response,
    validate_request,
    get_data_error_result,
    get_request_json,
)
from agent.canvas import Canvas
from rag.utils.redis_conn import REDIS_CONN
from api.apps import login_required, current_user


@manager.route('/cancel/<task_id>', methods=['PUT'])  # noqa: F821
@login_required
def cancel(task_id):
    try:
        REDIS_CONN.set(f"{task_id}-cancel", "x")
    except Exception as e:
        logging.exception(e)
    return get_json_result(data=True)
    
    
@manager.route('/<canvas_id>/sessions', methods=['GET'])  # noqa: F821
@login_required
def sessions(canvas_id):
    tenant_id = current_user.id
    if not UserCanvasService.accessible(canvas_id, tenant_id):
        return get_json_result(
            data=False, message='Only owner of canvas authorized for this operation.',
            code=RetCode.OPERATING_ERROR)

    user_id = request.args.get("user_id")
    page_number = int(request.args.get("page", 1))
    items_per_page = int(request.args.get("page_size", 30))
    keywords = request.args.get("keywords")
    from_date = request.args.get("from_date")
    to_date = request.args.get("to_date")
    orderby = request.args.get("orderby", "update_time")
    exp_user_id = request.args.get("exp_user_id")
    if request.args.get("desc") == "False" or request.args.get("desc") == "false":
        desc = False
    else:
        desc = True

    if exp_user_id:
        sess = API4ConversationService.get_names(canvas_id, exp_user_id)
        return get_json_result(data={"total": len(sess), "sessions": sess})
    
    # dsl defaults to True in all cases except for False and false
    include_dsl = request.args.get("dsl") != "False" and request.args.get("dsl") != "false"
    total, sess = API4ConversationService.get_list(canvas_id, tenant_id, page_number, items_per_page, orderby, desc,
                                             None, user_id, include_dsl, keywords, from_date, to_date, exp_user_id=exp_user_id)
    try:
        return get_json_result(data={"total": total, "sessions": sess})
    except Exception as e:
        return server_error_response(e)


@manager.route('/<canvas_id>/sessions', methods=['PUT'])  # noqa: F821
@login_required
async def set_session(canvas_id):
    req = await get_request_json()
    tenant_id = current_user.id
    e, cvs = UserCanvasService.get_by_id(canvas_id)
    assert e, "Agent not found."
    if not isinstance(cvs.dsl, str):
        cvs.dsl = json.dumps(cvs.dsl, ensure_ascii=False)
    session_id=get_uuid()
    canvas = Canvas(cvs.dsl, tenant_id, canvas_id, canvas_id=cvs.id)
    canvas.reset()
    # Get the version title for this canvas (using latest, not necessarily released)
    version_title = UserCanvasVersionService.get_latest_version_title(cvs.id, release_mode=False)
    conv = {
        "id": session_id,
        "name": req.get("name", ""),
        "dialog_id": cvs.id,
        "user_id": tenant_id,
        "exp_user_id": tenant_id,
        "message": [],
        "source": "agent",
        "dsl": cvs.dsl,
        "reference": [],
        "version_title": version_title
    }
    API4ConversationService.save(**conv)
    return get_json_result(data=conv)


@manager.route('/<canvas_id>/sessions/<session_id>', methods=['GET'])  # noqa: F821
@login_required
def get_session(canvas_id, session_id):
    tenant_id = current_user.id
    if not UserCanvasService.accessible(canvas_id, tenant_id):
        return get_json_result(
            data=False, message='Only owner of canvas authorized for this operation.',
            code=RetCode.OPERATING_ERROR)
    _, conv = API4ConversationService.get_by_id(session_id)
    return get_json_result(data=conv.to_dict())


@manager.route('/<canvas_id>/sessions/<session_id>', methods=['DELETE'])  # noqa: F821
@login_required
def del_session(canvas_id, session_id):
    tenant_id = current_user.id
    if not UserCanvasService.accessible(canvas_id, tenant_id):
        return get_json_result(
            data=False, message='Only owner of canvas authorized for this operation.',
            code=RetCode.OPERATING_ERROR)
    return get_json_result(data=API4ConversationService.delete_by_id(session_id))


@manager.route('/prompts', methods=['GET'])  # noqa: F821
@login_required
def prompts():
    from rag.prompts.generator import ANALYZE_TASK_SYSTEM, ANALYZE_TASK_USER, NEXT_STEP, REFLECT, CITATION_PROMPT_TEMPLATE

    return get_json_result(data={
        "task_analysis": ANALYZE_TASK_SYSTEM +"\n\n"+ ANALYZE_TASK_USER,
        "plan_generation": NEXT_STEP,
        "reflection": REFLECT,
        #"context_summary": SUMMARY4MEMORY,
        #"context_ranking": RANK_MEMORY,
        "citation_guidelines": CITATION_PROMPT_TEMPLATE
    })


@manager.route('/download', methods=['GET'])  # noqa: F821
async def download():
    id = request.args.get("id")
    created_by = request.args.get("created_by")
    blob = FileService.get_blob(created_by, id)
    return await make_response(blob)
