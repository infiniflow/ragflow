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
from typing import Annotated

from api.apps import login_required, current_user

from api.db.db_models import APIToken
from api.db.services.api_service import APITokenService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.user_service import UserTenantService
from api.utils.api_utils import (
    get_json_result,
    get_data_error_result,
    server_error_response,
    generate_confirmation_token,
)
from common.versions import get_ragflow_version
from common.time_utils import current_timestamp, datetime_format
from timeit import default_timer as timer

from rag.utils.redis_conn import REDIS_CONN
from quart import jsonify
from api.utils.health_utils import run_health_checks
from common import settings
from pydantic import BaseModel, ConfigDict, Field
from quart_schema import validate_response, tag


# Pydantic Schemas for OpenAPI Documentation


class BaseSchema(BaseModel):
    """Base schema with common configuration."""
    model_config = ConfigDict(extra="allow", strict=False)


class VersionResponse(BaseModel):
    """Response schema for version endpoint."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[dict, Field(..., description="Version information")]
    message: Annotated[str, Field("Success", description="Response message")]


class SystemStatusResponse(BaseModel):
    """Response schema for system status endpoint."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[dict, Field(..., description="System status information including doc_engine, storage, database, redis, and task_executor_heartbeats")]
    message: Annotated[str, Field("Success", description="Response message")]


class NewTokenRequest(BaseSchema):
    """Request schema for creating a new API token."""
    name: Annotated[str | None, Field(None, description="Optional name for the token", max_length=255)]


class NewTokenResponse(BaseModel):
    """Response schema for new token endpoint."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[dict, Field(..., description="Token information including token, beta, tenant_id, and timestamps")]
    message: Annotated[str, Field("Success", description="Response message")]


class TokenListResponse(BaseModel):
    """Response schema for token list endpoint."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[list[dict], Field(..., description="List of API tokens with their details")]
    message: Annotated[str, Field("Success", description="Response message")]


class DeleteTokenResponse(BaseModel):
    """Response schema for delete token endpoint."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[bool, Field(..., description="Deletion status")]
    message: Annotated[str, Field("Success", description="Response message")]


class SystemConfigResponse(BaseModel):
    """Response schema for system configuration endpoint."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[dict, Field(..., description="System configuration with registerEnabled flag")]
    message: Annotated[str, Field("Success", description="Response message")]


class HealthzResponse(BaseModel):
    """Response schema for health check endpoint."""
    db: Annotated[str, Field(..., description="Database health status: 'ok' or 'nok'")]
    redis: Annotated[str, Field(..., description="Redis health status: 'ok' or 'nok'")]
    doc_engine: Annotated[str, Field(..., description="Document engine health status: 'ok' or 'nok'")]
    storage: Annotated[str, Field(..., description="Storage health status: 'ok' or 'nok'")]
    status: Annotated[str, Field(..., description="Overall health status: 'ok' if all checks pass, 'nok' otherwise")]


class PingResponse(BaseModel):
    """Response schema for ping endpoint."""
    message: Annotated[str, Field("pong", description="Ping response message")]


# API Tags
system_tag = tag(["system"])
api_token_tag = tag(["api_token"])


@manager.route("/version", methods=["GET"])  # noqa: F821
@login_required
@validate_response(200, VersionResponse)
@system_tag
def version():
    """
    Get the current version of the application.

    Returns version information for the RAGFlow system.
    """
    return get_json_result(data=get_ragflow_version())


@manager.route("/status", methods=["GET"])  # noqa: F821
@login_required
@validate_response(200, SystemStatusResponse)
@system_tag
def status():
    """
    Get the system status.

    Returns comprehensive health status information including:
    - doc_engine: Document store (Elasticsearch/Infinity) status
    - storage: Object storage (MinIO/S3) status
    - database: Database (MySQL) status
    - redis: Redis cache status
    - task_executor_heartbeats: Heartbeat status of task executors
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
        settings.STORAGE_IMPL.health()
        res["storage"] = {
            "storage": settings.STORAGE_IMPL_TYPE.lower(),
            "status": "green",
            "elapsed": "{:.1f}".format((timer() - st) * 1000.0),
        }
    except Exception as e:
        res["storage"] = {
            "storage": settings.STORAGE_IMPL_TYPE.lower(),
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
            heartbeats = REDIS_CONN.zrangebyscore(task_executor_id, now - 60 * 30, now)
            heartbeats = [json.loads(heartbeat) for heartbeat in heartbeats]
            task_executor_heartbeats[task_executor_id] = heartbeats
    except Exception:
        logging.exception("get task executor heartbeats failed!")
    res["task_executor_heartbeats"] = task_executor_heartbeats

    return get_json_result(data=res)


@manager.route("/healthz", methods=["GET"])  # noqa: F821
@validate_response(200, HealthzResponse)
@validate_response(500, HealthzResponse)
@system_tag
def healthz():
    """
    Health check endpoint for Kubernetes/liveness probes.

    Performs basic health checks on critical system components:
    - Database connectivity
    - Redis connectivity
    - Document engine (Elasticsearch/Infinity) connectivity
    - Storage (MinIO/S3) connectivity

    Returns HTTP 200 if all checks pass, HTTP 500 otherwise.
    """
    result, all_ok = run_health_checks()
    return jsonify(result), (200 if all_ok else 500)


@manager.route("/ping", methods=["GET"])  # noqa: F821
@validate_response(200, str)
@system_tag
def ping():
    """
    Simple ping endpoint for connectivity testing.

    Returns a plain "pong" response to verify the service is running.
    Used for basic connectivity checks and load balancer health probes.
    """
    return "pong", 200


@manager.route("/new_token", methods=["POST"])  # noqa: F821
@login_required
@validate_response(200, NewTokenResponse)
@api_token_tag
def new_token():
    """
    Generate a new API token.

    Creates a new API token for the current user's tenant.
    The token consists of a main token and a beta token (shorter version).
    Returns the complete token object including tenant_id and timestamps.
    """
    try:
        tenants = UserTenantService.query(user_id=current_user.id)
        if not tenants:
            return get_data_error_result(message="Tenant not found!")

        tenant_id = [tenant for tenant in tenants if tenant.role == "owner"][0].tenant_id
        obj = {
            "tenant_id": tenant_id,
            "token": generate_confirmation_token(),
            "beta": generate_confirmation_token().replace("ragflow-", "")[:32],
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
@validate_response(200, TokenListResponse)
@api_token_tag
def token_list():
    """
    List all API tokens for the current user.

    Retrieves all API tokens associated with the current user's tenant.
    Each token includes the main token, beta token, and creation timestamps.
    If a token is missing its beta token, it will be auto-generated.
    """
    try:
        tenants = UserTenantService.query(user_id=current_user.id)
        if not tenants:
            return get_data_error_result(message="Tenant not found!")

        tenant_id = [tenant for tenant in tenants if tenant.role == "owner"][0].tenant_id
        objs = APITokenService.query(tenant_id=tenant_id)
        objs = [o.to_dict() for o in objs]
        for o in objs:
            if not o["beta"]:
                o["beta"] = generate_confirmation_token().replace("ragflow-", "")[:32]
                APITokenService.filter_update([APIToken.tenant_id == tenant_id, APIToken.token == o["token"]], o)
        return get_json_result(data=objs)
    except Exception as e:
        return server_error_response(e)


@manager.route("/token/<token>", methods=["DELETE"])  # noqa: F821
@login_required
@validate_response(200, DeleteTokenResponse)
@api_token_tag
def rm(token):
    """
    Remove an API token.

    Deletes the specified API token from the current user's tenant.
    The token to delete is specified as a path parameter.
    Only tokens belonging to the current user's tenant can be deleted.
    """
    try:
        tenants = UserTenantService.query(user_id=current_user.id)
        if not tenants:
            return get_data_error_result(message="Tenant not found!")

        tenant_id = tenants[0].tenant_id
        APITokenService.filter_delete([APIToken.tenant_id == tenant_id, APIToken.token == token])
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)


@manager.route("/config", methods=["GET"])  # noqa: F821
@validate_response(200, SystemConfigResponse)
@system_tag
def get_config():
    """
    Get system configuration.

    Returns public system configuration settings including:
    - registerEnabled: Whether user self-registration is enabled (0=disabled, 1=enabled)
    This endpoint does not require authentication.
    """
    return get_json_result(data={"registerEnabled": settings.REGISTER_ENABLED})
