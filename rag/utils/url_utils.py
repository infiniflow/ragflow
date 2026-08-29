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
import re
from urllib.parse import urlparse, urlunparse


def ensure_v1(url: str) -> str:
    """Ensure the URL ends with a versioned path segment like ``/v1``.

    If the path already contains a segment starting with ``v{digit}`` (e.g.
    ``/v1``, ``/v2``, ``/v3``, ``/v1beta``, ``/v1alpha1``), the URL is
    returned unchanged.  Otherwise the base host is kept and ``/v1`` is
    appended.

    If the path ends with a known OpenAI-compatible endpoint segment
    (``/chat/completions``, ``/embeddings``, ``/completions``), the
    endpoint segment is stripped first so the OpenAI client does not
    double the path when it re-appends the same segment on the request.
    This is what happens when a user pastes a full endpoint URL
    (``https://api.pzero.studio/v1/chat/completions``) into the base-URL
    field instead of just the host.

    Examples::

        >>> ensure_v1("https://api.example.com")
        'https://api.example.com/v1'
        >>> ensure_v1("https://api.example.com/v1")
        'https://api.example.com/v1'
        >>> ensure_v1("https://api.example.com/v2/chat")
        'https://api.example.com/v2/chat'
        >>> ensure_v1("https://api.example.com/api/v3")
        'https://api.example.com/api/v3'
        >>> ensure_v1("https://generativelanguage.googleapis.com/v1beta/openai/")
        'https://generativelanguage.googleapis.com/v1beta/openai/'
        >>> ensure_v1("https://api.pzero.studio/v1/chat/completions")
        'https://api.pzero.studio/v1'
        >>> ensure_v1("https://api.openai.com/v1/embeddings")
        'https://api.openai.com/v1'
    """
    if not url:
        return url

    parsed = urlparse(url)
    path = parsed.path.rstrip("/")

    # Strip a trailing OpenAI-compatible endpoint if the user pasted a full
    # endpoint URL. The OpenAI client will re-append the same segment on the
    # request, so leaving it in place would double the path. See issue #18965.
    endpoint_stripped = False
    for endpoint in ("/chat/completions", "/embeddings", "/completions"):
        if path.endswith(endpoint):
            path = path[: -len(endpoint)].rstrip("/")
            endpoint_stripped = True
            break

    # Check if any path segment starts with v{digit}, e.g. v1, v2beta, v1alpha1
    segments = path.split("/")
    has_version_segment = any(re.match(r"^v\d+", segment) for segment in segments)

    if has_version_segment and not endpoint_stripped:
        # Preserve the original URL (including any trailing slash) so callers
        # round-trip exactly.
        return url

    # Either the original URL had no version segment (append /v1) or we
    # stripped an endpoint (drop the doubled segment and reuse the version).
    if has_version_segment:
        return urlunparse((parsed.scheme, parsed.netloc, path, parsed.params, parsed.query, parsed.fragment))

    # No versioned segment found – append /v1
    new_path = (path + "/v1") if path else "/v1"
    return urlunparse((parsed.scheme, parsed.netloc, new_path, parsed.params, parsed.query, parsed.fragment))


def append_api_path(url: str, endpoint: str) -> str:
    """Append an API endpoint path exactly once while preserving the base path."""
    if not url:
        return url

    parsed = urlparse(url)
    path = parsed.path.rstrip("/")
    endpoint_path = f"/{endpoint.strip('/')}"
    if not path.endswith(endpoint_path):
        path = f"{path}{endpoint_path}"

    return urlunparse((parsed.scheme, parsed.netloc, path, parsed.params, parsed.query, parsed.fragment))
