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

QUERIT_SEARCH_URL = "https://api.querit.ai/v1/search"


class Querit:
    def __init__(self, api_key: str):
        self.api_key = api_key

    def search(self, query: str) -> list[dict[str, Any]]:
        try:
            response = requests.post(
                QUERIT_SEARCH_URL,
                headers={
                    "Accept": "application/json",
                    "Authorization": f"Bearer {self.api_key}",
                    "Content-Type": "application/json",
                },
                json={
                    "query": query,
                    "count": 6,
                    "chunksPerDoc": 1,
                },
                timeout=DEFAULT_TIMEOUT,
            )
            response.raise_for_status()
            response_data = response.json()
            if not isinstance(response_data, dict):
                raise TypeError("Querit API response must be a JSON object.")

            results_container = response_data.get("results", {})
            if not isinstance(results_container, dict):
                raise TypeError("Querit API response field results must be an object.")
            results = results_container.get("result", [])
            if not isinstance(results, list):
                raise TypeError("Querit API response field results.result must be an array.")

            normalized_results = []
            for result in results:
                if not isinstance(result, dict):
                    continue
                content = _querit_text(result.get("snippet"))
                if not content:
                    continue
                normalized_results.append(
                    {
                        "url": _querit_text(result.get("url")),
                        "title": _querit_text(result.get("title")),
                        "content": content,
                        "score": 1.0,
                    }
                )
            return normalized_results
        except requests.HTTPError as error:
            status = error.response.status_code if error.response is not None else "unknown"
            logger.error("Querit search failed: HTTP %s", status)
            return []
        except (requests.RequestException, TypeError, ValueError) as error:
            logger.error("Querit search failed: %s", type(error).__name__)
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
        # Counts only: the query and the retrieved page text are user data.
        logger.info("Querit search returned %d chunks", len(chunks))
        return {"chunks": chunks, "doc_aggs": doc_aggs}


def _querit_text(value: Any) -> str:
    return "" if value is None else str(value)
