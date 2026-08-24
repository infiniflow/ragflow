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

_TOKEN_MISSING_WARNING = (
    "********************************************************************************\n"
    "⚠️  SECURITY WARNING: SANDBOX_EXECUTOR_MANAGER_API_TOKEN is not set.\n"
    "    The executor manager accepts unauthenticated requests on /run.\n"
    "    Anyone who can reach this endpoint can execute code in the sandbox pool.\n"
    "    Set SANDBOX_EXECUTOR_MANAGER_API_TOKEN to a strong shared secret\n"
    "    (and make sure RAGFlow is configured with the same value) and restart.\n"
    "********************************************************************************"
)

_missing_token_warned = False


def get_configured_api_token() -> str:
    """Return the configured shared-secret API token, or an empty string."""
    return (os.getenv(API_TOKEN_ENV_VAR) or "").strip()


def warn_if_token_missing() -> None:
    """Log a prominent warning (once) when no API token is configured.

    Keeping the endpoint open when the token is unset preserves backwards
    compatibility with existing deployments; the warning nudges operators to
    enable authentication.
    """
    global _missing_token_warned
    if get_configured_api_token():
        return
    if not _missing_token_warned:
        _missing_token_warned = True
        logger.warning(_TOKEN_MISSING_WARNING)


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

    When SANDBOX_EXECUTOR_MANAGER_API_TOKEN is set, every request must present
    the same secret via `Authorization: Bearer <token>` or `X-Sandbox-Token`.
    Comparison uses secrets.compare_digest to avoid timing attacks.

    When the variable is unset, requests are allowed through (backwards
    compatibility) and a warning is logged.
    """
    warn_if_token_missing()

    configured_token = get_configured_api_token()
    if not configured_token:
        return

    provided_token = _extract_request_token(request)
    if not provided_token or not secrets.compare_digest(provided_token, configured_token):
        logger.warning("🚫 Rejected unauthenticated %s request from %s", request.url.path, request.client.host if request.client else "unknown")
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="Missing or invalid sandbox executor API token")
