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
"""Validation and URL helpers for the MWS GPT Model Hub provider."""

from urllib.parse import urlparse, urlunparse


def normalize_mws_project_url(base_url: str | None) -> str:
    """Validate and normalize an MWS GPT Model Hub project-root URL."""
    value = (base_url or "").strip().rstrip("/")
    parsed = urlparse(value)
    path_parts = parsed.path.strip("/").split("/")
    if parsed.scheme not in {"http", "https"} or not parsed.netloc or not parsed.hostname or len(path_parts) != 2 or path_parts[0] != "projects" or not path_parts[1]:
        raise ValueError("MWS API URL must be a project root in the form https://gpt.mwsapis.ru/projects/<project>")
    if parsed.username or parsed.password or parsed.params or parsed.query or parsed.fragment:
        raise ValueError("MWS API URL must not contain credentials, parameters, a query string, or a fragment")
    return urlunparse((parsed.scheme, parsed.netloc, f"/projects/{path_parts[1]}", "", "", ""))


def mws_api_url(base_url: str | None, endpoint: str) -> str:
    """Build an MWS API endpoint relative to a validated project root."""
    return f"{normalize_mws_project_url(base_url)}/{endpoint.strip('/')}"


def require_mws_token(token: str | None) -> str:
    """Return a normalized MWS bearer token or reject an empty value."""
    value = (token or "").strip()
    if not value:
        raise ValueError("MWS Token is required")
    return value
