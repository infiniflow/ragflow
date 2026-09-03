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

import logging
from typing import Protocol

from rag.utils.querit_conn import Querit
from rag.utils.serply_conn import Serply
from rag.utils.tavily_conn import Tavily
from rag.utils.youcom_conn import YouCom

WEB_SEARCH_PROVIDER_TAVILY = "tavily"
WEB_SEARCH_PROVIDER_QUERIT = "querit"
WEB_SEARCH_PROVIDER_SERPLY = "serply"
WEB_SEARCH_PROVIDER_YOUCOM = "youcom"

# You.com serves a keyless endpoint, so it is usable with no credentials at all.
# Every other provider here requires a key before it can be selected.
KEYLESS_WEB_SEARCH_PROVIDERS = frozenset({WEB_SEARCH_PROVIDER_YOUCOM})

logger = logging.getLogger(__name__)


class WebSearchProvider(Protocol):
    def retrieve_chunks(self, question: str) -> dict[str, list]:
        """Return web results in RAGFlow's chunk and document aggregate shape."""


def _get_api_key(prompt_config: dict, field: str) -> str:
    api_key = prompt_config.get(field)
    return api_key.strip() if isinstance(api_key, str) else ""


def has_web_search_provider(prompt_config: dict | None) -> bool:
    if not prompt_config:
        return False
    provider = prompt_config.get("web_search_provider", WEB_SEARCH_PROVIDER_TAVILY)
    if provider in KEYLESS_WEB_SEARCH_PROVIDERS:
        return True
    if provider == WEB_SEARCH_PROVIDER_TAVILY:
        return bool(_get_api_key(prompt_config, "tavily_api_key"))
    if provider == WEB_SEARCH_PROVIDER_QUERIT:
        return bool(_get_api_key(prompt_config, "querit_api_key"))
    if provider == WEB_SEARCH_PROVIDER_SERPLY:
        return bool(_get_api_key(prompt_config, "serply_api_key"))
    return False


def create_web_search_provider(prompt_config: dict | None) -> WebSearchProvider | None:
    if not prompt_config:
        logger.debug("Web search provider resolution: provider=none status=disabled")
        return None

    provider = prompt_config.get("web_search_provider", WEB_SEARCH_PROVIDER_TAVILY)
    if provider not in (
        WEB_SEARCH_PROVIDER_TAVILY,
        WEB_SEARCH_PROVIDER_QUERIT,
        WEB_SEARCH_PROVIDER_SERPLY,
        WEB_SEARCH_PROVIDER_YOUCOM,
    ):
        logger.debug("Web search provider resolution: provider=%s status=invalid", provider)
        return None
    if not has_web_search_provider(prompt_config):
        logger.debug("Web search provider resolution: provider=%s status=disabled", provider)
        return None

    logger.debug("Web search provider resolution: provider=%s status=resolved", provider)
    if provider == WEB_SEARCH_PROVIDER_QUERIT:
        return Querit(_get_api_key(prompt_config, "querit_api_key"))
    if provider == WEB_SEARCH_PROVIDER_SERPLY:
        return Serply(_get_api_key(prompt_config, "serply_api_key"))
    if provider == WEB_SEARCH_PROVIDER_YOUCOM:
        # The key is optional: an empty one selects the keyless endpoint.
        return YouCom(_get_api_key(prompt_config, "youcom_api_key"))
    return Tavily(_get_api_key(prompt_config, "tavily_api_key"))
