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
import logging
import os
import time

from quart import request
from common.constants import RetCode
from common.exceptions import ArgumentException, NotFoundException
from api.apps import login_required
from api.utils.api_utils import validate_request, get_request_json, get_error_argument_result, get_json_result
from api.apps.services import memory_api_service


@manager.route("/memories", methods=["POST"])  # noqa: F821
@login_required
@validate_request("name", "memory_type", "embd_id", "llm_id")
async def create_memory():
    timing_enabled = os.getenv("RAGFLOW_API_TIMING")
    t_start = time.perf_counter() if timing_enabled else None
    req = await get_request_json()
    t_parsed = time.perf_counter() if timing_enabled else None
    try:
        memory_info = {
            "name": req["name"],
            "memory_type": req["memory_type"],
            "embd_id": req["embd_id"],
            "llm_id": req["llm_id"]
        }
        success, res = await memory_api_service.create_memory(memory_info)
        if timing_enabled:
            logging.info(
                "api_timing create_memory parse_ms=%.2f validate_and_db_ms=%.2f total_ms=%.2f path=%s",
                (t_parsed - t_start) * 1000,
                (time.perf_counter() - t_parsed) * 1000,
                (time.perf_counter() - t_start) * 1000,
                request.path,
            )
        if success:
            return get_json_result(message=True, data=res)
        else:
            return get_json_result(message=res, code=RetCode.SERVER_ERROR)

    except ArgumentException as arg_error:
        logging.error(arg_error)
        if timing_enabled:
            logging.info(
                "api_timing create_memory error=%s parse_ms=%.2f total_ms=%.2f path=%s",
                str(arg_error),
                (t_parsed - t_start) * 1000,
                (time.perf_counter() - t_start) * 1000,
                request.path,
            )
        return get_error_argument_result(str(arg_error))

    except Exception as e:
        logging.error(e)
        if timing_enabled:
            logging.info(
                "api_timing create_memory error=%s parse_ms=%.2f total_ms=%.2f path=%s",
                str(e),
                (t_parsed - t_start) * 1000,
                (time.perf_counter() - t_start) * 1000,
                request.path,
            )
        return get_json_result(code=RetCode.SERVER_ERROR, message="Internal server error")


@manager.route("/memories/<memory_id>", methods=["PUT"])  # noqa: F821
@login_required
async def update_memory(memory_id):
    req = await get_request_json()
    new_settings = {k: req[k] for k in [
        "name", "permissions", "llm_id", "embd_id", "memory_type", "memory_size", "forgetting_policy", "temperature",
        "avatar", "description", "system_prompt", "user_prompt"
    ] if k in req}
    try:
        success, res = await memory_api_service.update_memory(memory_id, new_settings)
        if success:
            return get_json_result(message=True, data=res)
        else:
            return get_json_result(message=res, code=RetCode.SERVER_ERROR)
    except NotFoundException as not_found_exception:
        logging.error(not_found_exception)
        return get_json_result(code=RetCode.NOT_FOUND, message=str(not_found_exception))
    except ArgumentException as arg_error:
        logging.error(arg_error)
        return get_error_argument_result(str(arg_error))
    except Exception as e:
        logging.error(e)
        return get_json_result(code=RetCode.SERVER_ERROR, message="Internal server error")


@manager.route("/memories/<memory_id>", methods=["DELETE"])  # noqa: F821
@login_required
async def delete_memory(memory_id):
    try:
        await memory_api_service.delete_memory(memory_id)
        return get_json_result(message=True)
    except NotFoundException as not_found_exception:
        logging.error(not_found_exception)
        return get_json_result(code=RetCode.NOT_FOUND, message=str(not_found_exception))
    except Exception as e:
        logging.error(e)
        return get_json_result(code=RetCode.SERVER_ERROR, message="Internal server error")


@manager.route("/memories", methods=["GET"])  # noqa: F821
@login_required
async def list_memory():
    filter_params = {
        k: request.args.get(k) for k in ["memory_type", "tenant_id", "storage_type"] if k in request.args
    }
    keywords = request.args.get("keywords")
    page = int(request.args.get("page", 1))
    page_size = int(request.args.get("page_size", 50))
    try:
        res = await memory_api_service.list_memory(filter_params, keywords, page, page_size)
        return get_json_result(message=True, data=res)
    except Exception as e:
        logging.error(e)
        return get_json_result(code=RetCode.SERVER_ERROR, message="Internal server error")


@manager.route("/memories/<memory_id>/config", methods=["GET"])  # noqa: F821
@login_required
async def get_memory_config(memory_id):
    try:
        res = await memory_api_service.get_memory_config(memory_id)
        return get_json_result(message=True, data=res)
    except NotFoundException as not_found_exception:
        logging.error(not_found_exception)
        return get_json_result(code=RetCode.NOT_FOUND, message=str(not_found_exception))
    except Exception as e:
        logging.error(e)
        return get_json_result(code=RetCode.SERVER_ERROR, message="Internal server error")


@manager.route("/memories/<memory_id>", methods=["GET"])  # noqa: F821
@login_required
async def get_memory_messages(memory_id):
    args = request.args
    agent_ids = args.getlist("agent_id")
    if len(agent_ids) == 1 and ',' in agent_ids[0]:
        agent_ids = agent_ids[0].split(',')
    keywords = args.get("keywords", "")
    keywords = keywords.strip()
    page = int(args.get("page", 1))
    page_size = int(args.get("page_size", 50))
    try:
        res = await memory_api_service.get_memory_messages(
            memory_id, agent_ids, keywords, page, page_size
        )
        return get_json_result(message=True, data=res)
    except NotFoundException as not_found_exception:
        logging.error(not_found_exception)
        return get_json_result(code=RetCode.NOT_FOUND, message=str(not_found_exception))
    except Exception as e:
        logging.error(e)
        return get_json_result(code=RetCode.SERVER_ERROR, message="Internal server error")
