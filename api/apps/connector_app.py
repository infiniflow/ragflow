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
import asyncio
import json
import logging
import time
import uuid
from html import escape
from typing import Any

from quart import request, make_response
from google_auth_oauthlib.flow import Flow

from api.db import InputType
from api.db.services.connector_service import ConnectorService, SyncLogsService
from api.utils.api_utils import get_data_error_result, get_json_result, get_request_json, validate_request
from common.constants import RetCode, TaskStatus
from common.data_source.config import GOOGLE_DRIVE_WEB_OAUTH_REDIRECT_URI, GMAIL_WEB_OAUTH_REDIRECT_URI, BOX_WEB_OAUTH_REDIRECT_URI, DocumentSource
from common.data_source.google_util.constant import WEB_OAUTH_POPUP_TEMPLATE, GOOGLE_SCOPES
from common.misc_utils import get_uuid
from rag.utils.redis_conn import REDIS_CONN
from api.apps import login_required, current_user
from box_sdk_gen import BoxOAuth, OAuthConfig, GetAuthorizeUrlOptions


@manager.route("/set", methods=["POST"])  # noqa: F821
@login_required
async def set_connector():
    req = await get_request_json()
    if req.get("id"):
        conn = {fld: req[fld] for fld in ["prune_freq", "refresh_freq", "config", "timeout_secs"] if fld in req}
        ConnectorService.update_by_id(req["id"], conn)
    else:
        req["id"] = get_uuid()
        conn = {
            "id": req["id"],
            "tenant_id": current_user.id,
            "name": req["name"],
            "source": req["source"],
            "input_type": InputType.POLL,
            "config": req["config"],
            "refresh_freq": int(req.get("refresh_freq", 5)),
            "prune_freq": int(req.get("prune_freq", 720)),
            "timeout_secs": int(req.get("timeout_secs", 60 * 29)),
            "status": TaskStatus.SCHEDULE,
        }
        ConnectorService.save(**conn)

    await asyncio.sleep(1)
    e, conn = ConnectorService.get_by_id(req["id"])

    return get_json_result(data=conn.to_dict())


@manager.route("/list", methods=["GET"])  # noqa: F821
@login_required
def list_connector():
    return get_json_result(data=ConnectorService.list(current_user.id))


@manager.route("/<connector_id>", methods=["GET"])  # noqa: F821
@login_required
def get_connector(connector_id):
    e, conn = ConnectorService.get_by_id(connector_id)
    if not e:
        return get_data_error_result(message="Can't find this Connector!")
    return get_json_result(data=conn.to_dict())


@manager.route("/<connector_id>/logs", methods=["GET"])  # noqa: F821
@login_required
def list_logs(connector_id):
    req = request.args.to_dict(flat=True)
    arr, total = SyncLogsService.list_sync_tasks(connector_id, int(req.get("page", 1)), int(req.get("page_size", 15)))
    return get_json_result(data={"total": total, "logs": arr})


@manager.route("/<connector_id>/resume", methods=["PUT"])  # noqa: F821
@login_required
async def resume(connector_id):
    req = await get_request_json()
    if req.get("resume"):
        ConnectorService.resume(connector_id, TaskStatus.SCHEDULE)
    else:
        ConnectorService.resume(connector_id, TaskStatus.CANCEL)
    return get_json_result(data=True)


@manager.route("/<connector_id>/rebuild", methods=["PUT"])  # noqa: F821
@login_required
@validate_request("kb_id")
async def rebuild(connector_id):
    req = await get_request_json()
    err = ConnectorService.rebuild(req["kb_id"], connector_id, current_user.id)
    if err:
        return get_json_result(data=False, message=err, code=RetCode.SERVER_ERROR)
    return get_json_result(data=True)


@manager.route("/<connector_id>/rm", methods=["POST"])  # noqa: F821
@login_required
def rm_connector(connector_id):
    ConnectorService.resume(connector_id, TaskStatus.CANCEL)
    ConnectorService.delete_by_id(connector_id)
    return get_json_result(data=True)


WEB_FLOW_TTL_SECS = 15 * 60


def _web_state_cache_key(flow_id: str, source_type: str | None = None) -> str:
    """Return Redis key for web OAuth state.

    The default prefix keeps backward compatibility for Google Drive.
    When source_type == "gmail", a different prefix is used so that
    Drive/Gmail flows don't clash in Redis.
    """
    prefix = f"{source_type}_web_flow_state"
    return f"{prefix}:{flow_id}"


def _web_result_cache_key(flow_id: str, source_type: str | None = None) -> str:
    """Return Redis key for web OAuth result.

    Mirrors _web_state_cache_key logic for result storage.
    """
    prefix = f"{source_type}_web_flow_result"
    return f"{prefix}:{flow_id}"


def _load_credentials(payload: str | dict[str, Any]) -> dict[str, Any]:
    if isinstance(payload, dict):
        return payload
    try:
        return json.loads(payload)
    except json.JSONDecodeError as exc:  # pragma: no cover - defensive
        raise ValueError("Invalid Google credentials JSON.") from exc


def _get_web_client_config(credentials: dict[str, Any]) -> dict[str, Any]:
    web_section = credentials.get("web")
    if not isinstance(web_section, dict):
        raise ValueError("Google OAuth JSON must include a 'web' client configuration to use browser-based authorization.")
    return {"web": web_section}


async def _render_web_oauth_popup(flow_id: str, success: bool, message: str, source="drive"):
    status = "success" if success else "error"
    auto_close = "window.close();" if success else ""
    escaped_message = escape(message)
    #   Drive: ragflow-google-drive-oauth
    #   Gmail: ragflow-gmail-oauth
    payload_type = f"ragflow-{source}-oauth"
    payload_json = json.dumps(
        {
            "type": payload_type,
            "status": status,
            "flowId": flow_id or "",
            "message": message,
        }
    )
    # TODO(google-oauth): title/heading/message may need to reflect drive/gmail based on cached type
    html = WEB_OAUTH_POPUP_TEMPLATE.format(
        title=f"Google {source.capitalize()} Authorization",
        heading="Authorization complete" if success else "Authorization failed",
        message=escaped_message,
        payload_json=payload_json,
        auto_close=auto_close,
    )
    response = await make_response(html, 200)
    response.headers["Content-Type"] = "text/html; charset=utf-8"
    return response


@manager.route("/google/oauth/web/start", methods=["POST"])  # noqa: F821
@login_required
@validate_request("credentials")
async def start_google_web_oauth():
    source = request.args.get("type", "google-drive")
    if source not in ("google-drive", "gmail"):
        return get_json_result(code=RetCode.ARGUMENT_ERROR, message="Invalid Google OAuth type.")

    if source == "gmail":
        redirect_uri = GMAIL_WEB_OAUTH_REDIRECT_URI
        scopes = GOOGLE_SCOPES[DocumentSource.GMAIL]
    else:
        redirect_uri = GOOGLE_DRIVE_WEB_OAUTH_REDIRECT_URI
        scopes = GOOGLE_SCOPES[DocumentSource.GOOGLE_DRIVE]

    if not redirect_uri:
        return get_json_result(
            code=RetCode.SERVER_ERROR,
            message="Google OAuth redirect URI is not configured on the server.",
        )

    req = await get_request_json()
    raw_credentials = req.get("credentials", "")

    try:
        credentials = _load_credentials(raw_credentials)
        print(credentials)
    except ValueError as exc:
        return get_json_result(code=RetCode.ARGUMENT_ERROR, message=str(exc))

    if credentials.get("refresh_token"):
        return get_json_result(
            code=RetCode.ARGUMENT_ERROR,
            message="Uploaded credentials already include a refresh token.",
        )

    try:
        client_config = _get_web_client_config(credentials)
    except ValueError as exc:
        return get_json_result(code=RetCode.ARGUMENT_ERROR, message=str(exc))

    flow_id = str(uuid.uuid4())
    try:
        flow = Flow.from_client_config(client_config, scopes=scopes)
        flow.redirect_uri = redirect_uri
        authorization_url, _ = flow.authorization_url(
            access_type="offline",
            include_granted_scopes="true",
            prompt="consent",
            state=flow_id,
        )
    except Exception as exc:  # pragma: no cover - defensive
        logging.exception("Failed to create Google OAuth flow: %s", exc)
        return get_json_result(
            code=RetCode.SERVER_ERROR,
            message="Failed to initialize Google OAuth flow. Please verify the uploaded client configuration.",
        )

    cache_payload = {
        "user_id": current_user.id,
        "client_config": client_config,
        "created_at": int(time.time()),
    }
    REDIS_CONN.set_obj(_web_state_cache_key(flow_id, source), cache_payload, WEB_FLOW_TTL_SECS)

    return get_json_result(
        data={
            "flow_id": flow_id,
            "authorization_url": authorization_url,
            "expires_in": WEB_FLOW_TTL_SECS,
        }
    )


@manager.route("/gmail/oauth/web/callback", methods=["GET"])  # noqa: F821
async def google_gmail_web_oauth_callback():
    state_id = request.args.get("state")
    error = request.args.get("error")
    source = "gmail"

    error_description = request.args.get("error_description") or error

    if not state_id:
        return await _render_web_oauth_popup("", False, "Missing OAuth state parameter.", source)

    state_cache = REDIS_CONN.get(_web_state_cache_key(state_id, source))
    if not state_cache:
        return await _render_web_oauth_popup(state_id, False, "Authorization session expired. Please restart from the main window.", source)

    state_obj = json.loads(state_cache)
    client_config = state_obj.get("client_config")
    if not client_config:
        REDIS_CONN.delete(_web_state_cache_key(state_id, source))
        return await _render_web_oauth_popup(state_id, False, "Authorization session was invalid. Please retry.", source)

    if error:
        REDIS_CONN.delete(_web_state_cache_key(state_id, source))
        return await _render_web_oauth_popup(state_id, False, error_description or "Authorization was cancelled.", source)

    code = request.args.get("code")
    if not code:
        return await _render_web_oauth_popup(state_id, False, "Missing authorization code from Google.", source)

    try:
        # TODO(google-oauth): branch scopes/redirect_uri based on source_type (drive vs gmail)
        flow = Flow.from_client_config(client_config, scopes=GOOGLE_SCOPES[DocumentSource.GMAIL])
        flow.redirect_uri = GMAIL_WEB_OAUTH_REDIRECT_URI
        flow.fetch_token(code=code)
    except Exception as exc:  # pragma: no cover - defensive
        logging.exception("Failed to exchange Google OAuth code: %s", exc)
        REDIS_CONN.delete(_web_state_cache_key(state_id, source))
        return await _render_web_oauth_popup(state_id, False, "Failed to exchange tokens with Google. Please retry.", source)

    creds_json = flow.credentials.to_json()
    result_payload = {
        "user_id": state_obj.get("user_id"),
        "credentials": creds_json,
    }
    REDIS_CONN.set_obj(_web_result_cache_key(state_id, source), result_payload, WEB_FLOW_TTL_SECS)
    REDIS_CONN.delete(_web_state_cache_key(state_id, source))

    return await _render_web_oauth_popup(state_id, True, "Authorization completed successfully.", source)


@manager.route("/google-drive/oauth/web/callback", methods=["GET"])  # noqa: F821
async def google_drive_web_oauth_callback():
    state_id = request.args.get("state")
    error = request.args.get("error")
    source = "google-drive"

    error_description = request.args.get("error_description") or error

    if not state_id:
        return await _render_web_oauth_popup("", False, "Missing OAuth state parameter.", source)

    state_cache = REDIS_CONN.get(_web_state_cache_key(state_id, source))
    if not state_cache:
        return await _render_web_oauth_popup(state_id, False, "Authorization session expired. Please restart from the main window.", source)

    state_obj = json.loads(state_cache)
    client_config = state_obj.get("client_config")
    if not client_config:
        REDIS_CONN.delete(_web_state_cache_key(state_id, source))
        return await _render_web_oauth_popup(state_id, False, "Authorization session was invalid. Please retry.", source)

    if error:
        REDIS_CONN.delete(_web_state_cache_key(state_id, source))
        return await _render_web_oauth_popup(state_id, False, error_description or "Authorization was cancelled.", source)

    code = request.args.get("code")
    if not code:
        return await _render_web_oauth_popup(state_id, False, "Missing authorization code from Google.", source)

    try:
        # TODO(google-oauth): branch scopes/redirect_uri based on source_type (drive vs gmail)
        flow = Flow.from_client_config(client_config, scopes=GOOGLE_SCOPES[DocumentSource.GOOGLE_DRIVE])
        flow.redirect_uri = GOOGLE_DRIVE_WEB_OAUTH_REDIRECT_URI
        flow.fetch_token(code=code)
    except Exception as exc:  # pragma: no cover - defensive
        logging.exception("Failed to exchange Google OAuth code: %s", exc)
        REDIS_CONN.delete(_web_state_cache_key(state_id, source))
        return await _render_web_oauth_popup(state_id, False, "Failed to exchange tokens with Google. Please retry.", source)

    creds_json = flow.credentials.to_json()
    result_payload = {
        "user_id": state_obj.get("user_id"),
        "credentials": creds_json,
    }
    REDIS_CONN.set_obj(_web_result_cache_key(state_id, source), result_payload, WEB_FLOW_TTL_SECS)
    REDIS_CONN.delete(_web_state_cache_key(state_id, source))

    return await _render_web_oauth_popup(state_id, True, "Authorization completed successfully.", source)

@manager.route("/google/oauth/web/result", methods=["POST"])  # noqa: F821
@login_required
@validate_request("flow_id")
async def poll_google_web_result():
    req = await request.json or {}
    source = request.args.get("type")
    if source not in ("google-drive", "gmail"):
        return get_json_result(code=RetCode.ARGUMENT_ERROR, message="Invalid Google OAuth type.")
    flow_id = req.get("flow_id")
    cache_raw = REDIS_CONN.get(_web_result_cache_key(flow_id, source))
    if not cache_raw:
        return get_json_result(code=RetCode.RUNNING, message="Authorization is still pending.")

    result = json.loads(cache_raw)
    if result.get("user_id") != current_user.id:
        return get_json_result(code=RetCode.PERMISSION_ERROR, message="You are not allowed to access this authorization result.")

    REDIS_CONN.delete(_web_result_cache_key(flow_id, source))
    return get_json_result(data={"credentials": result.get("credentials")})

@manager.route("/box/oauth/web/start", methods=["POST"])  # noqa: F821
@login_required
async def start_box_web_oauth():
    req = await get_request_json()

    client_id = req.get("client_id")
    client_secret = req.get("client_secret")    
    redirect_uri = req.get("redirect_uri", BOX_WEB_OAUTH_REDIRECT_URI)

    if not client_id or not client_secret:
        return get_json_result(code=RetCode.ARGUMENT_ERROR, message="Box client_id and client_secret are required.")

    flow_id = str(uuid.uuid4())

    box_auth = BoxOAuth(
        OAuthConfig(
            client_id=client_id,
            client_secret=client_secret,
        )
    )

    auth_url = box_auth.get_authorize_url(
        options=GetAuthorizeUrlOptions(
            redirect_uri=redirect_uri,
            state=flow_id,
        )
    )

    cache_payload = {
        "user_id": current_user.id,
        "auth_url": auth_url,
        "client_id": client_id,
        "client_secret": client_secret,
        "created_at": int(time.time()),
    }
    REDIS_CONN.set_obj(_web_state_cache_key(flow_id, "box"), cache_payload, WEB_FLOW_TTL_SECS)
    return get_json_result(
        data = {
            "flow_id": flow_id,
            "authorization_url": auth_url,
            "expires_in": WEB_FLOW_TTL_SECS,}
    )

@manager.route("/box/oauth/web/callback", methods=["GET"])  # noqa: F821
async def box_web_oauth_callback():
    flow_id = request.args.get("state")
    if not flow_id:
        return await _render_web_oauth_popup("", False, "Missing OAuth parameters.", "box")
    
    code = request.args.get("code")
    if not code:
        return await _render_web_oauth_popup(flow_id, False, "Missing authorization code from Box.", "box")

    cache_payload = json.loads(REDIS_CONN.get(_web_state_cache_key(flow_id, "box")))
    if not cache_payload:
        return get_json_result(code=RetCode.ARGUMENT_ERROR, message="Box OAuth session expired or invalid.")

    error = request.args.get("error")
    error_description = request.args.get("error_description") or error
    if error:
        REDIS_CONN.delete(_web_state_cache_key(flow_id, "box"))
        return await _render_web_oauth_popup(flow_id, False, error_description or "Authorization failed.", "box")
    
    auth = BoxOAuth(
        OAuthConfig(
            client_id=cache_payload.get("client_id"),
            client_secret=cache_payload.get("client_secret"),
        )
    )

    auth.get_tokens_authorization_code_grant(code)
    token = auth.retrieve_token()
    result_payload = {
        "user_id": cache_payload.get("user_id"),
        "client_id": cache_payload.get("client_id"),
        "client_secret": cache_payload.get("client_secret"),
        "access_token": token.access_token,
        "refresh_token": token.refresh_token,
    }

    REDIS_CONN.set_obj(_web_result_cache_key(flow_id, "box"), result_payload, WEB_FLOW_TTL_SECS)
    REDIS_CONN.delete(_web_state_cache_key(flow_id, "box"))

    return await _render_web_oauth_popup(flow_id, True, "Authorization completed successfully.", "box")

@manager.route("/box/oauth/web/result", methods=["POST"])  # noqa: F821
@login_required
@validate_request("flow_id")
async def poll_box_web_result():
    req = await get_request_json()
    flow_id = req.get("flow_id")

    cache_blob = REDIS_CONN.get(_web_result_cache_key(flow_id, "box"))
    if not cache_blob:
        return get_json_result(code=RetCode.RUNNING, message="Authorization is still pending.")

    cache_raw = json.loads(cache_blob)
    if cache_raw.get("user_id") != current_user.id:
        return get_json_result(code=RetCode.PERMISSION_ERROR, message="You are not allowed to access this authorization result.")
    
    REDIS_CONN.delete(_web_result_cache_key(flow_id, "box"))

    return get_json_result(data={"credentials": cache_raw})