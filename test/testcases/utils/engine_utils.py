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
import requests

_DOC_ENGINE_CACHE = None


def get_doc_engine(rag=None) -> str:
    """Return lower-cased doc_engine from env, or from /system/status if env is unset."""
    global _DOC_ENGINE_CACHE
    env = (os.getenv("DOC_ENGINE") or "").strip().lower()
    if env:
        _DOC_ENGINE_CACHE = env
        return env
    if _DOC_ENGINE_CACHE:
        return _DOC_ENGINE_CACHE
    if rag is None:
        return ""
    try:
        api_url = getattr(rag, "api_url", "")
        if "/api/" in api_url:
            base_url, version = api_url.rsplit("/api/", 1)
            status_url = f"{base_url}/{version}/system/status"
        else:
            status_url = f"{api_url}/system/status"
        headers = getattr(rag, "authorization_header", {})
        res = requests.get(status_url, headers=headers).json()
        engine = str(res.get("data", {}).get("doc_engine", {}).get("type", "")).lower()
        if engine:
            _DOC_ENGINE_CACHE = engine
        return engine
    except Exception:
        return ""
