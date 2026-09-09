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

SERPLY_SEARCH_URL = "https://api.serply.io/v1/search/"


class Serply:
    def __init__(self, api_key: str):
        self.api_key = api_key

    def search(self, query: str) -> list[dict[str, Any]]:
        try:
            response = requests.get(
                SERPLY_SEARCH_URL,
                headers={
                    "Accept": "application/json",
                    "X-Api-Key": self.api_key,
                    # Serply sits behind Cloudflare, which rejects requests
                    # without an explicit User-Agent, so always send one.
                    "User-Agent": "ragflow-web-search",
                },
                params={
                    "q": query,
                    "num": 6,
                },
                timeout=DEFAULT_TIMEOUT,
            )
            response.raise_for_status()
            response_data = response.json()
            if not isinstance(response_data, dict):
                raise TypeError("Serply API response must be a JSON object.")

            results = response_data.get("results", [])
            if not isinstance(results, list):
                raise TypeError("Serply API response field results must be an array.")

            normalized_results = []
            for result in results:
                if not isinstance(result, dict):
                    continue
                content = _serply_text(result.get("description")).strip()
                if not content:
                    continue
                normalized_results.append(
                    {
                        "url": _serply_text(result.get("link")),
                        "title": _serply_text(result.get("title")),
                        "content": content,
                        "score": 1.0,
                    }
                )
            return normalized_results
        except requests.HTTPError as error:
            # requests builds the HTTPError message from the response URL, and
            # the query is a URL parameter here, so the message must not be logged.
            status = error.response.status_code if error.response is not None else "unknown"
            logger.error("Serply search failed: HTTP %s", status)
            return []
        except (requests.RequestException, TypeError, ValueError) as error:
            logger.error("Serply search failed: %s", type(error).__name__)
            return []

    def retrieve_chunks(self, question: str) -> dict[str, list]:
        chunks = []
        doc_aggs = []
        results = self.search(question)
        logger.info("Serply search returned %d results", len(results))
        for result in results:
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
        return {"chunks": chunks, "doc_aggs": doc_aggs}


def _serply_text(value: Any) -> str:
    return "" if value is None else str(value)
