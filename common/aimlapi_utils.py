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
"""Shared request attribution for the aimlapi.com provider.

Every call RAGFlow makes to AIMLAPI — chat, embeddings, vision, the model
catalog and the agent-authorization endpoints — carries the same two headers:
they tell AIMLAPI which client the traffic comes from and which integration it
belongs to. The same client identifier also rides on the consent URL, where a
header cannot reach (the account is created in a browser AIMLAPI controls).
Defaults are RAGFlow's own; both are overridable via environment:

  AIMLAPI_SOURCE       client identifier (default ``agent/ragflow``)
  AIMLAPI_PARTNER_ID   integration id (defaults to RAGFlow's)
"""

import os

DEFAULT_SOURCE = "agent/ragflow"
DEFAULT_PARTNER_ID = "part_yNkcOvbGLtgxWLjy4sRysaer"


def source() -> str:
    return os.environ.get("AIMLAPI_SOURCE", DEFAULT_SOURCE).strip()


def partner_id() -> str:
    return os.environ.get("AIMLAPI_PARTNER_ID", DEFAULT_PARTNER_ID).strip()


def attribution_headers() -> dict:
    """Headers to send on every request to aimlapi.com."""
    headers = {"X-AIMLAPI-Source": source()}
    pid = partner_id()
    if pid:
        headers["X-AIMLAPI-Partner-ID"] = pid
    return headers
