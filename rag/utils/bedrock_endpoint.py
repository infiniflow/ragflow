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

import os
import re
from urllib.parse import urlsplit


SUPPORTED_BEDROCK_AUTH_MODES = frozenset({"access_key_secret", "iam_role", "assume_role", "bedrock_api_key"})
SUPPORTED_BEDROCK_ENDPOINT_TYPES = frozenset({"runtime", "mantle_openai", "mantle_anthropic"})
_AWS_BEDROCK_HOST_PATTERNS = (
    re.compile(r"bedrock(?:-runtime)?(?:-fips)?\.[a-z0-9-]+\.(?:amazonaws\.com(?:\.cn)?|api\.aws|api\.amazonwebservices\.com\.cn)"),
    re.compile(r"bedrock-mantle\.[a-z0-9-]+\.api\.aws"),
    re.compile(r"vpce-[a-z0-9-]+\.bedrock(?:-mantle|-fips|-runtime(?:-fips)?)?\.[a-z0-9-]+\.vpce\.amazonaws\.com(?:\.cn)?"),
)
_AWS_REGION_PATTERN = re.compile(r"[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?")


def validate_bedrock_api_key(api_key: str) -> str:
    api_key = api_key.strip()
    if not api_key or not api_key.isprintable():
        raise ValueError("Bedrock API key must not be empty or contain control characters")
    return api_key


def validate_bedrock_region(region_name: str) -> None:
    if not _AWS_REGION_PATTERN.fullmatch(region_name):
        raise ValueError("Bedrock region must be a valid AWS region identifier")


def _mantle_root_url(endpoint_url: str) -> str:
    root_url = endpoint_url.rstrip("/")
    for suffix in ("/anthropic/v1/messages", "/v1/models", "/anthropic", "/v1"):
        if root_url.endswith(suffix):
            return root_url[: -len(suffix)]
    return root_url


def normalize_bedrock_endpoint(endpoint_type: str, endpoint_url: str) -> str:
    endpoint_url = endpoint_url.rstrip("/")
    if not endpoint_url or endpoint_type == "runtime":
        return endpoint_url
    root_url = _mantle_root_url(endpoint_url)
    if endpoint_type == "mantle_openai":
        return f"{root_url}/v1"
    if endpoint_type == "mantle_anthropic":
        return f"{root_url}/anthropic"
    raise ValueError(f"Unsupported Bedrock endpoint type: {endpoint_type}")


def mantle_model_catalog_url(endpoint_url: str) -> str:
    return f"{_mantle_root_url(endpoint_url)}/v1"


def resolve_bedrock_endpoint(auth_mode: str | None, endpoint_type: str | None, endpoint_url: str | None) -> tuple[str, str]:
    if not auth_mode:
        raise ValueError("Bedrock auth_mode must be provided in the key")
    if auth_mode not in SUPPORTED_BEDROCK_AUTH_MODES:
        raise ValueError(f"Unsupported Bedrock auth_mode: {auth_mode}")

    resolved_type = endpoint_type or "runtime"
    if resolved_type not in SUPPORTED_BEDROCK_ENDPOINT_TYPES:
        raise ValueError(f"Unsupported Bedrock endpoint type: {resolved_type}")
    resolved_url = normalize_bedrock_endpoint(resolved_type, endpoint_url or "")
    if resolved_type != "runtime" and not resolved_url:
        raise ValueError("Bedrock endpoint URL must be provided for a Mantle endpoint")
    if auth_mode != "bedrock_api_key" and resolved_type != "runtime":
        raise ValueError("Bedrock Mantle endpoints require Bedrock API key authentication")
    validate_bedrock_endpoint_target(resolved_url)
    return resolved_type, resolved_url


def _configured_bedrock_endpoint_targets() -> tuple[str, ...]:
    return tuple(item.strip().lower() for item in os.getenv("BEDROCK_ENDPOINT_HOST_ALLOWLIST", "").split(",") if item.strip())


def _is_configured_bedrock_endpoint(hostname: str, port: int | None) -> bool:
    for allowed_target in _configured_bedrock_endpoint_targets():
        try:
            parsed_target = urlsplit(f"//{allowed_target}")
            allowed_host = (parsed_target.hostname or "").rstrip(".")
            allowed_port = parsed_target.port
        except ValueError:
            continue
        if allowed_host.startswith("*."):
            host_matches = hostname.endswith(allowed_host[1:]) and hostname != allowed_host[2:]
        else:
            host_matches = hostname == allowed_host
        if host_matches and ((allowed_port is None and port in (None, 443)) or allowed_port == port):
            return True
    return False


def validate_bedrock_endpoint_target(endpoint_url: str) -> None:
    if not endpoint_url:
        return
    parsed = urlsplit(endpoint_url)
    if parsed.scheme != "https":
        raise ValueError("Bedrock endpoint URL must use HTTPS")
    if not parsed.hostname or parsed.username or parsed.password:
        raise ValueError("Bedrock endpoint URL must not contain credentials")
    if parsed.query or parsed.fragment:
        raise ValueError("Bedrock endpoint URL must not contain a query or fragment")
    try:
        port = parsed.port
    except ValueError as error:
        raise ValueError("Bedrock endpoint URL must specify a valid port") from error
    hostname = parsed.hostname.rstrip(".").lower()
    if any(pattern.fullmatch(hostname) for pattern in _AWS_BEDROCK_HOST_PATTERNS):
        if port not in (None, 443):
            raise ValueError("Bedrock endpoint URL must not specify a non-default port")
        return
    if _is_configured_bedrock_endpoint(hostname, port):
        return
    if port not in (None, 443):
        raise ValueError("Bedrock endpoint URL non-default port requires an exact host:port entry in BEDROCK_ENDPOINT_HOST_ALLOWLIST")
    raise ValueError("Bedrock endpoint hostname is not allowed; configure BEDROCK_ENDPOINT_HOST_ALLOWLIST for a trusted proxy")
