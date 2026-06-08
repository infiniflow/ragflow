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
"""Webhook security validators extracted from agent_api.py.

Every validator raises ``Exception`` with a human-readable message on
failure and returns ``None`` (or a decoded JWT dict for ``_validate_jwt_auth``)
on success.  The outer route handler is responsible for catching these and
returning the appropriate HTTP error response.
"""

import ipaddress
import time

import jwt
from quart import request


async def _validate_max_body_size(security_cfg):
    """Reject requests whose Content-Length exceeds ``max_body_size``."""
    max_size = security_cfg.get("max_body_size")
    if not max_size:
        return

    units = {"kb": 1024, "mb": 1024**2}
    size_str = max_size.lower()

    for suffix, factor in units.items():
        if size_str.endswith(suffix):
            limit = int(size_str.replace(suffix, "")) * factor
            break
    else:
        raise Exception("Invalid max_body_size format")

    MAX_LIMIT = 10 * 1024 * 1024  # 10 MB
    if limit > MAX_LIMIT:
        raise Exception("max_body_size exceeds maximum allowed size (10MB)")

    content_length = request.content_length
    if content_length is not None:
        if content_length > limit:
            raise Exception(f"Request body too large: {content_length} > {limit}")
    else:
        # Content-Length absent (e.g. chunked transfer encoding) — read body to enforce limit.
        body = await request.get_data()
        if len(body) > limit:
            raise Exception(f"Request body too large: {len(body)} > {limit}")


def _validate_ip_whitelist(security_cfg):
    """Reject requests from IPs not present in ``ip_whitelist``."""
    whitelist = security_cfg.get("ip_whitelist", [])
    if not whitelist:
        return

    client_ip = request.remote_addr
    if not client_ip:
        raise Exception("Unable to determine client IP address")

    for rule in whitelist:
        if "/" in rule:
            if ipaddress.ip_address(client_ip) in ipaddress.ip_network(rule, strict=False):
                return
        else:
            if client_ip == rule:
                return

    raise Exception(f"IP {client_ip} is not allowed by whitelist")


def _validate_rate_limit(security_cfg, agent_id):
    """Token-bucket rate limiting backed by Redis.

    ``agent_id`` is required explicitly because this function is no longer a
    closure and cannot close over the outer route scope.
    """
    rl = security_cfg.get("rate_limit")
    if not rl:
        return

    limit = int(rl.get("limit", 60))
    if limit <= 0:
        raise Exception("rate_limit.limit must be > 0")
    per = rl.get("per", "minute")

    window = {
        "second": 1,
        "minute": 60,
        "hour": 3600,
        "day": 86400,
    }.get(per)

    if not window:
        raise Exception(f"Invalid rate_limit.per: {per}")

    capacity = limit
    rate = limit / window
    cost = 1

    key = f"rl:tb:{agent_id}"
    now = time.time()

    try:
        from rag.utils.redis_conn import REDIS_CONN

        res = REDIS_CONN.lua_token_bucket(
            keys=[key],
            args=[capacity, rate, now, cost],
            client=REDIS_CONN.REDIS,
        )

        allowed = int(res[0])
        if allowed != 1:
            raise Exception("Too many requests (rate limit exceeded)")

    except Exception as e:
        raise Exception(f"Rate limit error: {e}")


def _validate_token_auth(security_cfg):
    """Validate a header-based static token."""
    token_cfg = security_cfg.get("token", {})
    header = token_cfg.get("token_header")
    token_value = token_cfg.get("token_value")

    if not header or not token_value:
        raise Exception("Token auth is misconfigured: token_header and token_value are required")

    provided = request.headers.get(header)
    if not provided or provided != token_value:
        raise Exception("Invalid token authentication")


def _validate_basic_auth(security_cfg):
    """Validate HTTP Basic Auth credentials."""
    auth_cfg = security_cfg.get("basic_auth", {})
    username = auth_cfg.get("username")
    password = auth_cfg.get("password")

    auth = request.authorization
    if not auth or auth.username != username or auth.password != password:
        raise Exception("Invalid Basic Auth credentials")


def _validate_jwt_auth(security_cfg):
    """Validate a JWT Bearer token and check required custom claims."""
    jwt_cfg = security_cfg.get("jwt", {})
    secret = jwt_cfg.get("secret")
    if not secret:
        raise Exception("JWT secret not configured")

    auth_header = request.headers.get("Authorization", "")
    if not auth_header.startswith("Bearer "):
        raise Exception("Missing Bearer token")

    token = auth_header[len("Bearer "):].strip()
    if not token:
        raise Exception("Empty Bearer token")

    alg = (jwt_cfg.get("algorithm") or "HS256").upper()

    decode_kwargs = {
        "key": secret,
        "algorithms": [alg],
    }
    options = {}
    if jwt_cfg.get("audience"):
        decode_kwargs["audience"] = jwt_cfg["audience"]
        options["verify_aud"] = True
    else:
        options["verify_aud"] = False

    if jwt_cfg.get("issuer"):
        decode_kwargs["issuer"] = jwt_cfg["issuer"]
        options["verify_iss"] = True
    else:
        options["verify_iss"] = False

    try:
        decoded = jwt.decode(token, options=options, **decode_kwargs)
    except Exception as e:
        raise Exception(f"Invalid JWT: {str(e)}")

    raw_required_claims = jwt_cfg.get("required_claims", [])
    if isinstance(raw_required_claims, str):
        required_claims = [raw_required_claims]
    elif isinstance(raw_required_claims, (list, tuple, set)):
        required_claims = list(raw_required_claims)
    else:
        required_claims = []

    required_claims = [c for c in required_claims if isinstance(c, str) and c.strip()]

    RESERVED_CLAIMS = {"exp", "sub", "aud", "iss", "nbf", "iat"}
    for claim in required_claims:
        if claim in RESERVED_CLAIMS:
            raise Exception(f"Reserved JWT claim cannot be required: {claim}")

    for claim in required_claims:
        if claim not in decoded:
            raise Exception(f"Missing JWT claim: {claim}")

    return decoded


async def validate_webhook_security(security_cfg: dict, agent_id: str):
    """Run all security checks in order; raise on first failure.

    ``agent_id`` is forwarded to ``_validate_rate_limit`` (rate-limit Redis key).
    """
    if not security_cfg:
        return

    await _validate_max_body_size(security_cfg)
    _validate_ip_whitelist(security_cfg)
    _validate_rate_limit(security_cfg, agent_id)

    auth_type = security_cfg.get("auth_type", "none")

    if auth_type == "none":
        return
    elif auth_type == "token":
        _validate_token_auth(security_cfg)
    elif auth_type == "basic":
        _validate_basic_auth(security_cfg)
    elif auth_type == "jwt":
        _validate_jwt_auth(security_cfg)
    else:
        raise Exception(f"Unsupported auth_type: {auth_type}")
