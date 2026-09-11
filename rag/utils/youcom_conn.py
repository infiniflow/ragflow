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
from typing import Any

import requests

from common.http_client import DEFAULT_TIMEOUT
from common.misc_utils import get_uuid
from rag.nlp import rag_tokenizer

logger = logging.getLogger(__name__)

# You.com serves the same response shape from two endpoints. The keyless one is
# rate-limited but needs no credentials; the keyed one lifts those limits and
# exposes the full Search API. The keyless endpoint rejects an X-API-Key header,
# so the endpoint and the headers are always chosen together.
YOUCOM_SEARCH_URL = "https://api.you.com/v1/search"
YOUCOM_KEYLESS_SEARCH_URL = "https://api.you.com/v1/agents/search"
YOUCOM_RESULT_COUNT = 6
# Identifies RAGFlow to You.com. On the keyless endpoint there is no key to
# attribute traffic to, so this is the only signal available.
YOUCOM_USER_AGENT = "RAGFlow youdotcom-integration/infiniflow-ragflow"


class YouCom:
    def __init__(self, api_key: str = ""):
        self.api_key = api_key.strip() if isinstance(api_key, str) else ""

    def search(self, query: str) -> list[dict[str, Any]]:
        headers = {
            "Accept": "application/json",
            "User-Agent": YOUCOM_USER_AGENT,
        }
        url = YOUCOM_KEYLESS_SEARCH_URL
        if self.api_key:
            url = YOUCOM_SEARCH_URL
            headers["X-API-Key"] = self.api_key

        try:
            response = requests.get(
                url,
                headers=headers,
                params={"query": query, "count": YOUCOM_RESULT_COUNT},
                timeout=DEFAULT_TIMEOUT,
            )
            response.raise_for_status()
            response_data = response.json()
            if not isinstance(response_data, dict):
                raise TypeError("You.com API response must be a JSON object.")

            results_container = response_data.get("results", {})
            if not isinstance(results_container, dict):
                raise TypeError("You.com API response field results must be an object.")

            normalized_results = []
            # `count` applies per section, so web and news together can exceed
            # it. Web results lead; the merged list is trimmed back afterwards.
            for section in ("web", "news"):
                section_results = results_container.get(section, [])
                if section_results is None:
                    continue
                if not isinstance(section_results, list):
                    raise TypeError(f"You.com API response field results.{section} must be an array.")
                for result in section_results:
                    if not isinstance(result, dict):
                        continue
                    content = _youcom_content(result)
                    if not content:
                        continue
                    normalized_results.append(
                        {
                            "url": _youcom_text(result.get("url")),
                            "title": _youcom_text(result.get("title")),
                            "content": content,
                            "score": 1.0,
                        }
                    )
            return normalized_results[:YOUCOM_RESULT_COUNT]
        except requests.HTTPError as error:
            # Never log the exception message: requests builds it from the
            # response URL, and the query is a URL parameter here.
            status = error.response.status_code if error.response is not None else "unknown"
            logger.error("You.com search failed: HTTP %s", status)
            return []
        except (requests.RequestException, TypeError, ValueError) as error:
            logger.error("You.com search failed: %s", type(error).__name__)
            return []

    def retrieve_chunks(self, question: str) -> dict[str, list]:
        chunks = []
        doc_aggs = []
        for result in self.search(question):
            chunk_id = get_uuid()
            chunks.append(
                {
                    "chunk_id": chunk_id,
                    "content_ltks": rag_tokenizer.tokenize(result["content"]),
                    "content_with_weight": result["content"],
                    "doc_id": chunk_id,
                    "docnm_kwd": result["title"],
                    "kb_id": [],
                    "important_kwd": [],
                    "image_id": "",
                    "similarity": result["score"],
                    "vector_similarity": 1.0,
                    "term_similarity": 0,
                    "vector": [],
                    "positions": [],
                    "url": result["url"],
                }
            )
            doc_aggs.append(
                {
                    "doc_name": result["title"],
                    "doc_id": chunk_id,
                    "count": 1,
                    "url": result["url"],
                }
            )
        # Deliberately logs counts only: the query and the retrieved page text
        # are user data and must not reach the logs.
        logger.info("[YouCom] retrieved %s chunks (keyed=%s)", len(chunks), bool(self.api_key))
        return {"chunks": chunks, "doc_aggs": doc_aggs}


def _youcom_content(result: dict[str, Any]) -> str:
    """Prefer the extracted page passages; news hits only carry a description."""
    snippets = result.get("snippets")
    if isinstance(snippets, list):
        joined = "\n".join(_youcom_text(snippet) for snippet in snippets if _youcom_text(snippet).strip())
        if joined:
            return joined
    return _youcom_text(result.get("description"))


def _youcom_text(value: Any) -> str:
    return "" if value is None else str(value)
