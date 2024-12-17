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
#  limitations under the License
#
import logging
from datetime import datetime
import json

from flask_login import login_required, current_user

from api.db.db_models import APIToken
from api.db.services.api_service import APITokenService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.user_service import UserTenantService
from api import settings
from api.utils import current_timestamp, datetime_format
from api.utils.api_utils import (
    get_json_result,
    get_data_error_result,
    server_error_response,
    generate_confirmation_token,
)
from api.versions import get_ragflow_version
from rag.utils.storage_factory import STORAGE_IMPL, STORAGE_IMPL_TYPE
from timeit import default_timer as timer

from rag.utils.redis_conn import REDIS_CONN


@manager.route("/version", methods=["GET"])  # noqa: F821
@login_required
def version():
    """
    Get the current version of the application.
    ---
    tags:
      - System
    security:
      - ApiKeyAuth: []
    responses:
      200:
        description: Version retrieved successfully.
        schema:
          type: object
          properties:
            version:
              type: string
              description: Version number.
    """
    return get_json_result(data=get_ragflow_version())


@manager.route("/status", methods=["GET"])  # noqa: F821
@login_required
def status():
    """
    Get the system status.
    ---
    tags:
      - System
    security:
      - ApiKeyAuth: []
    responses:
      200:
        description: System is operational.
        schema:
          type: object
          properties:
            es:
              type: object
              description: Elasticsearch status.
            storage:
              type: object
              description: Storage status.
            database:
              type: object
              description: Database status.
      503:
        description: Service unavailable.
        schema:
          type: object
          properties:
            error:
              type: string
              description: Error message.
    """
    res = {}
    st = timer()
    try:
        res["doc_engine"] = settings.docStoreConn.health()
        res["doc_engine"]["elapsed"] = "{:.1f}".format((timer() - st) * 1000.0)
    except Exception as e:
        res["doc_engine"] = {
            "type": "unknown",
            "status": "red",
            "elapsed": "{:.1f}".format((timer() - st) * 1000.0),
            "error": str(e),
        }

    st = timer()
    try:
        STORAGE_IMPL.health()
        res["storage"] = {
            "storage": STORAGE_IMPL_TYPE.lower(),
            "status": "green",
            "elapsed": "{:.1f}".format((timer() - st) * 1000.0),
        }
    except Exception as e:
        res["storage"] = {
            "storage": STORAGE_IMPL_TYPE.lower(),
            "status": "red",
            "elapsed": "{:.1f}".format((timer() - st) * 1000.0),
            "error": str(e),
        }

    st = timer()
    try:
        KnowledgebaseService.get_by_id("x")
        res["database"] = {
            "database": settings.DATABASE_TYPE.lower(),
            "status": "green",
            "elapsed": "{:.1f}".format((timer() - st) * 1000.0),
        }
    except Exception as e:
        res["database"] = {
            "database": settings.DATABASE_TYPE.lower(),
            "status": "red",
            "elapsed": "{:.1f}".format((timer() - st) * 1000.0),
            "error": str(e),
        }

    st = timer()
    try:
        if not REDIS_CONN.health():
            raise Exception("Lost connection!")
        res["redis"] = {
            "status": "green",
            "elapsed": "{:.1f}".format((timer() - st) * 1000.0),
        }
    except Exception as e:
        res["redis"] = {
            "status": "red",
            "elapsed": "{:.1f}".format((timer() - st) * 1000.0),
            "error": str(e),
        }

    task_executor_heartbeats = {}
    try:
        task_executors = REDIS_CONN.smembers("TASKEXE")
        now = datetime.now().timestamp()
        for task_executor_id in task_executors:
            heartbeats = REDIS_CONN.zrangebyscore(task_executor_id, now - 60*30, now)
            heartbeats = [json.loads(heartbeat) for heartbeat in heartbeats]
            task_executor_heartbeats[task_executor_id] = heartbeats
    except Exception:
        logging.exception("get task executor heartbeats failed!")
    res["task_executor_heartbeats"] = task_executor_heartbeats

    return get_json_result(data=res)


@manager.route("/new_token", methods=["POST"])  # noqa: F821
@login_required
def new_token():
    """
    Generate a new API token.
    ---
    tags:
      - API Tokens
    security:
      - ApiKeyAuth: []
    parameters:
      - in: query
        name: name
        type: string
        required: false
        description: Name of the token.
    responses:
      200:
        description: Token generated successfully.
        schema:
          type: object
          properties:
            token:
              type: string
              description: The generated API token.
    """
    try:
        tenants = UserTenantService.query(user_id=current_user.id)
        if not tenants:
            return get_data_error_result(message="Tenant not found!")

        tenant_id = tenants[0].tenant_id
        obj = {
            "tenant_id": tenant_id,
            "token": generate_confirmation_token(tenant_id),
            "beta": generate_confirmation_token(generate_confirmation_token(tenant_id)).replace("ragflow-", "")[:32],
            "create_time": current_timestamp(),
            "create_date": datetime_format(datetime.now()),
            "update_time": None,
            "update_date": None,
        }

        if not APITokenService.save(**obj):
            return get_data_error_result(message="Fail to new a dialog!")

        return get_json_result(data=obj)
    except Exception as e:
        return server_error_response(e)


@manager.route("/token_list", methods=["GET"])  # noqa: F821
@login_required
def token_list():
    """
    List all API tokens for the current user.
    ---
    tags:
      - API Tokens
    security:
      - ApiKeyAuth: []
    responses:
      200:
        description: List of API tokens.
        schema:
          type: object
          properties:
            tokens:
              type: array
              items:
                type: object
                properties:
                  token:
                    type: string
                    description: The API token.
                  name:
                    type: string
                    description: Name of the token.
                  create_time:
                    type: string
                    description: Token creation time.
    """
    try:
        tenants = UserTenantService.query(user_id=current_user.id)
        if not tenants:
            return get_data_error_result(message="Tenant not found!")

        tenant_id = tenants[0].tenant_id
        objs = APITokenService.query(tenant_id=tenant_id)
        objs = [o.to_dict() for o in objs]
        for o in objs:
            if not o["beta"]:
                o["beta"] = generate_confirmation_token(generate_confirmation_token(tenants[0].tenant_id)).replace("ragflow-", "")[:32]
                APITokenService.filter_update([APIToken.tenant_id == tenant_id, APIToken.token == o["token"]], o)
        return get_json_result(data=objs)
    except Exception as e:
        return server_error_response(e)


@manager.route("/token/<token>", methods=["DELETE"])  # noqa: F821
@login_required
def rm(token):
    """
    Remove an API token.
    ---
    tags:
      - API Tokens
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: token
        type: string
        required: true
        description: The API token to remove.
    responses:
      200:
        description: Token removed successfully.
        schema:
          type: object
          properties:
            success:
              type: boolean
              description: Deletion status.
    """
    APITokenService.filter_delete(
        [APIToken.tenant_id == current_user.id, APIToken.token == token]
    )
    return get_json_result(data=True)
