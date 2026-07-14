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
    """
    if not url:
        return url

    parsed = urlparse(url)
    path = parsed.path.rstrip("/")

    # Check if any path segment starts with v{digit}, e.g. v1, v2beta, v1alpha1
    segments = path.split("/")
    if any(re.match(r"^v\d+", segment) for segment in segments):
        return url

    # No versioned segment found – append /v1
    new_path = (path + "/v1") if path else "/v1"
    return urlunparse((parsed.scheme, parsed.netloc, new_path, parsed.params, parsed.query, parsed.fragment))
