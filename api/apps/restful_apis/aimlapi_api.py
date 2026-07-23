#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
"""AIMLAPI agent-authorization endpoints (OAuth 2.0 Device Authorization Grant).

Lets a signed-in RAGFlow user obtain an AIMLAPI key without leaving the provider
dialog. The user's AIMLAPI login credentials never touch RAGFlow: it creates a
device-authorization request, the user approves it on the AIMLAPI consent page
(opened by the browser in a popup), and RAGFlow polls the token endpoint until
the key is issued. The device code is held server-side (Redis); only the issued
API key is returned to the browser, to be filled into the provider form.

Configuration (all via environment):
  AIMLAPI_APP_URL                  agent-auth host (default https://app.aimlapi.com)
  AIMLAPI_PARTNER_ID               partner id (defaults to RAGFlow's; override to change)
  AIMLAPI_PARTNER_NAME             display name shown on the consent page
  AIMLAPI_VERIFICATION_BASE_URL    base of the consent page (default https://aimlapi.com)
  AIMLAPI_REQUESTED_USD_LIMIT_MINOR requested spend cap in USD minor units (default 1000)
"""

import asyncio
import json
import logging
import os

import requests

from api.apps import login_required, current_user
from api.utils.api_utils import get_data_error_result, get_json_result, get_request_json, server_error_response, validate_request
from rag.utils.redis_conn import REDIS_CONN

LOGGER = logging.getLogger(__name__)

# OAuth 2.0 Device Authorization Grant (RFC 8628)
_DEVICE_CODE_GRANT = "urn:ietf:params:oauth:grant-type:device_code"
_REDIS_KEY_PREFIX = "aimlapi_authz:"
_HTTP_TIMEOUT = 15


def _app_url() -> str:
    return os.environ.get("AIMLAPI_APP_URL", "https://app.aimlapi.com").rstrip("/")


def _partner_id() -> str:
    return os.environ.get("AIMLAPI_PARTNER_ID", "part_yNkcOvbGLtgxWLjy4sRysaer").strip()


def _partner_name() -> str:
    return os.environ.get("AIMLAPI_PARTNER_NAME", "RAGFlow").strip()


def _verification_base_url() -> str:
    return os.environ.get("AIMLAPI_VERIFICATION_BASE_URL", "https://aimlapi.com").rstrip("/")


def _requested_usd_limit_minor() -> int:
    try:
        return int(os.environ.get("AIMLAPI_REQUESTED_USD_LIMIT_MINOR", "1000"))
    except (TypeError, ValueError):
        return 1000


@manager.route("/llm/aimlapi/authorize/start", methods=["POST"])  # noqa: F821
@login_required
async def aimlapi_authorize_start():
    """Create an AIMLAPI device-authorization request and return the consent URL."""
    partner_id = _partner_id()
    if not partner_id:
        return get_data_error_result(message="AIMLAPI partner id is not configured. Set AIMLAPI_PARTNER_ID.")

    # returnUrl only controls where AIMLAPI sends the browser after consent. This
    # flow never depends on it — the key arrives via authenticated polling and the
    # popup is closed on success — so we always use the trusted verification host
    # rather than a client-supplied value, leaving no open-redirect surface.
    return_url = _verification_base_url()

    payload = {
        "partnerId": partner_id,
        "partnerName": _partner_name(),
        "agentName": "RAGFlow",
        "returnUrl": return_url,
        "requestedUsdLimitMinor": _requested_usd_limit_minor(),
    }
    try:
        resp = await asyncio.to_thread(
            requests.post,
            f"{_app_url()}/v3/agent-auth/authorizations",
            json=payload,
            timeout=_HTTP_TIMEOUT,
        )
    except Exception as e:
        return server_error_response(e)

    if resp.status_code not in (200, 201):
        LOGGER.warning("AIMLAPI authorize start failed: status=%s body=%s", resp.status_code, resp.text[:300])
        return get_data_error_result(message=f"AIMLAPI authorization request failed (HTTP {resp.status_code}).")

    data = resp.json()
    request_id = data.get("requestId")
    device_code = data.get("deviceCode")
    if not request_id or not device_code:
        return get_data_error_result(message="AIMLAPI authorization response is missing requestId/deviceCode.")

    try:
        interval = int(data.get("interval", 5))
    except (TypeError, ValueError):
        interval = 5
    try:
        expires_in = int(data.get("expiresIn", 900))
    except (TypeError, ValueError):
        expires_in = 900

    # Hold the device code server-side, scoped to this user; it never reaches the browser.
    REDIS_CONN.set_obj(
        f"{_REDIS_KEY_PREFIX}{request_id}",
        {"device_code": device_code, "user_id": current_user.id},
        expires_in,
    )
    LOGGER.info("AIMLAPI authorize start ok: request_id=%s", request_id)

    # Rebuild the consent URL from AIMLAPI_VERIFICATION_BASE_URL so an env override applies; the create response returns an absolute URL.
    verification_uri = f"{_verification_base_url()}/agent/authorize?request={request_id}"
    return get_json_result(
        data={
            "request_id": request_id,
            "verification_uri": verification_uri,
            "interval": interval,
            "expires_in": expires_in,
        }
    )


@manager.route("/llm/aimlapi/authorize/poll", methods=["POST"])  # noqa: F821
@login_required
@validate_request("request_id")
async def aimlapi_authorize_poll():
    """Poll the AIMLAPI token endpoint; return the issued key once the user approves."""
    req = await get_request_json()
    request_id = req["request_id"]

    raw = REDIS_CONN.get(f"{_REDIS_KEY_PREFIX}{request_id}")
    if not raw:
        return get_json_result(data={"status": "expired"})
    try:
        cached = json.loads(raw)
    except (TypeError, ValueError):
        cached = None
    if not cached or cached.get("user_id") != current_user.id:
        return get_data_error_result(message="Authorization request not found for the current user.")

    payload = {
        "partnerId": _partner_id(),
        "deviceCode": cached["device_code"],
        "grant_type": _DEVICE_CODE_GRANT,
    }
    try:
        resp = await asyncio.to_thread(
            requests.post,
            f"{_app_url()}/v3/agent-auth/token",
            json=payload,
            timeout=_HTTP_TIMEOUT,
        )
    except Exception as e:
        return server_error_response(e)

    if resp.status_code not in (200, 201):
        LOGGER.warning("AIMLAPI authorize poll failed: status=%s body=%s", resp.status_code, resp.text[:300])
        return get_data_error_result(message=f"AIMLAPI token poll failed (HTTP {resp.status_code}).")

    data = resp.json()
    status = str(data.get("status", "")).lower()
    # The success field name is not contractually fixed yet; accept the common variants.
    api_key = data.get("apiKey") or data.get("api_key") or data.get("access_token") or data.get("key")

    if api_key:
        REDIS_CONN.delete(f"{_REDIS_KEY_PREFIX}{request_id}")
        LOGGER.info("AIMLAPI authorize poll ready: request_id=%s", request_id)
        return get_json_result(data={"status": "ready", "api_key": api_key})
    if status in ("denied", "expired", "cancelled", "canceled", "rejected"):
        REDIS_CONN.delete(f"{_REDIS_KEY_PREFIX}{request_id}")
        return get_json_result(data={"status": status})
    return get_json_result(data={"status": status or "pending"})
