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
import os
import secrets

from core.logger import logger
from fastapi import HTTPException, Request, status

API_TOKEN_ENV_VAR = "SANDBOX_EXECUTOR_MANAGER_API_TOKEN"
ALLOW_UNAUTHENTICATED_ENV_VAR = "SANDBOX_EXECUTOR_MANAGER_ALLOW_UNAUTHENTICATED"

_TRUTHY_FLAG_VALUES = {"1", "true", "yes", "on"}

_UNAUTH_OPT_IN_HINT = (
    "Set SANDBOX_EXECUTOR_MANAGER_API_TOKEN to a strong shared secret "
    "(and configure RAGFlow with the same value), or explicitly accept the risk by "
    "setting SANDBOX_EXECUTOR_MANAGER_ALLOW_UNAUTHENTICATED=true."
)

_UNAUTHENTICATED_WARNING = (
    "********************************************************************************\n"
    "\u26a0\ufe0f  SECURITY WARNING: SANDBOX_EXECUTOR_MANAGER_ALLOW_UNAUTHENTICATED=true and\n"
    "    no API token is configured. The executor manager accepts unauthenticated\n"
    "    requests on /run: anyone who can reach this endpoint can execute code in\n"
    "    the sandbox pool. Remove the flag and set SANDBOX_EXECUTOR_MANAGER_API_TOKEN\n"
    "    to a strong shared secret.\n"
    "********************************************************************************"
)

_no_token_or_opt_in_warned = False


def get_configured_api_token() -> str:
    """Return the configured shared-secret API token, or an empty string."""
    return (os.getenv(API_TOKEN_ENV_VAR) or "").strip()


def unauthenticated_access_explicitly_allowed() -> bool:
    """Whether the operator explicitly opted into unauthenticated /run.

    Authentication is fail-closed by default: when no API token is configured,
    /run refuses to serve instead of silently staying open. Deployments that
    genuinely cannot provide a token must set
    SANDBOX_EXECUTOR_MANAGER_ALLOW_UNAUTHENTICATED=true, making the insecure
    state an explicit operator decision rather than the default.
    """
    return (os.getenv(ALLOW_UNAUTHENTICATED_ENV_VAR) or "").strip().lower() in _TRUTHY_FLAG_VALUES


def warn_if_unauthenticated_opt_in() -> None:
    """Log a prominent warning (once) while the explicit insecure opt-in is active."""
    global _no_token_or_opt_in_warned
    if _no_token_or_opt_in_warned:
        return
    _no_token_or_opt_in_warned = True
    logger.warning(_UNAUTHENTICATED_WARNING)


def log_authentication_startup_state() -> None:
    """Log the effective /run authentication posture once at startup.

    Fail-closed with no token (503 for every /run request), the explicit
    insecure opt-in, or token-enforced authentication.
    """
    if get_configured_api_token():
        logger.info("Sandbox executor /run authentication: shared-secret token configured")
        return
    if unauthenticated_access_explicitly_allowed():
        warn_if_unauthenticated_opt_in()
        return
    logger.error(
        "SANDBOX_EXECUTOR_MANAGER_API_TOKEN is not set: /run will refuse requests with 503. %s",
        _UNAUTH_OPT_IN_HINT,
    )


def _extract_request_token(request: Request) -> str:
    """Extract the caller-supplied token from supported headers.

    Supported formats:
    - Authorization: Bearer <token>
    - X-Sandbox-Token: <token>
    """
    authorization = request.headers.get("Authorization", "")
    if authorization[:7].lower() == "bearer ":
        return authorization[7:].strip()
    return (request.headers.get("X-Sandbox-Token") or "").strip()


async def require_api_token(request: Request) -> None:
    """FastAPI dependency enforcing shared-secret authentication.

    Fail-closed by default (CWE-306): when SANDBOX_EXECUTOR_MANAGER_API_TOKEN
    is unset or blank, /run refuses requests with 503 instead of staying open.
    Legacy open behaviour survives only through the explicit operator opt-in
    SANDBOX_EXECUTOR_MANAGER_ALLOW_UNAUTHENTICATED=true.

    When a token is configured, every request must present the same secret via
    `Authorization: Bearer <token>` or `X-Sandbox-Token`. Comparison uses
    secrets.compare_digest to avoid timing attacks.
    """
    configured_token = get_configured_api_token()
    if not configured_token:
        if unauthenticated_access_explicitly_allowed():
            warn_if_unauthenticated_opt_in()
            return
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail=f"Sandbox executor authentication is not configured; refusing unauthenticated /run. {_UNAUTH_OPT_IN_HINT}",
        )

    provided_token = _extract_request_token(request)
    if not provided_token or not secrets.compare_digest(provided_token, configured_token):
        logger.warning("\U0001f6ab Rejected unauthenticated %s request from %s", request.url.path, request.client.host if request.client else "unknown")
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="Missing or invalid sandbox executor API token")
