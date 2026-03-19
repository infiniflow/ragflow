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

import hashlib
import math
from typing import Any

from common import settings
from common.doc_store.doc_store_base import SparseVector
from rag.nlp import rag_tokenizer

SPARSE_VECTOR_NAME = "sparse"
SPARSE_VECTOR_FIELD = "q_sparse_vec"
DENSE_VECTOR_NAME = "dense"
MULTIVECTOR_VECTOR_NAME = "colpali"
DEFAULT_SPARSE_MODEL = ""
DEFAULT_SPARSE_VOCAB_SIZE = 131072
ENABLED_SPARSE_MODELS = {"token", "builtin"}

_CACHED_MODEL = None
_CACHED_CONFIG = None


class TokenSparseEmbeddingModel:
    def __init__(self, vocab_size: int = DEFAULT_SPARSE_VOCAB_SIZE):
        self.vocab_size = max(int(vocab_size), 1024)

    @staticmethod
    def _normalize_text(value: Any) -> str:
        if value is None:
            return ""
        if isinstance(value, str):
            return value
        if isinstance(value, list):
            return "\n".join(str(v) for v in value if v is not None)
        return str(value)

    def _token_to_index(self, token: str) -> int:
        digest = hashlib.blake2b(token.encode("utf-8", "ignore"), digest_size=8).digest()
        return int.from_bytes(digest, byteorder="big", signed=False) % self.vocab_size

    def _encode_text(self, text: Any) -> SparseVector:
        normalized = self._normalize_text(text)
        tokens = [token for token in rag_tokenizer.tokenize(normalized).split() if token]
        if not tokens:
            return SparseVector(indices=[], values=[])
        weights = {}
        counts = {}
        for token in tokens:
            counts[token] = counts.get(token, 0) + 1
        for token, count in counts.items():
            index = self._token_to_index(token)
            weights[index] = weights.get(index, 0.0) + (1.0 + math.log1p(count))
        indices = sorted(weights.keys())
        values = [weights[index] for index in indices]
        return SparseVector(indices=indices, values=values)

    def encode(self, texts: list[Any]):
        return [self._encode_text(text) for text in texts], 0

    def encode_queries(self, text: Any):
        return self._encode_text(text), 0


def _sparse_config() -> tuple[bool, str, int]:
    conf = getattr(settings, "QDRANT", {}) or {}
    model_name = str(conf.get("sparse_model", DEFAULT_SPARSE_MODEL) or "").strip().lower()
    vocab_size = int(conf.get("sparse_vocab_size", DEFAULT_SPARSE_VOCAB_SIZE) or DEFAULT_SPARSE_VOCAB_SIZE)
    enabled = bool(getattr(settings, "DOC_ENGINE_QDRANT", False)) and model_name in ENABLED_SPARSE_MODELS
    return enabled, model_name, vocab_size



def get_sparse_embedding_model():
    global _CACHED_MODEL, _CACHED_CONFIG

    enabled, model_name, vocab_size = _sparse_config()
    config = (enabled, model_name, vocab_size)
    if config == _CACHED_CONFIG:
        return _CACHED_MODEL
    _CACHED_CONFIG = config
    _CACHED_MODEL = TokenSparseEmbeddingModel(vocab_size=vocab_size) if enabled else None
    return _CACHED_MODEL



def sparse_vector_to_payload(sparse_vector: SparseVector | dict | None) -> dict | None:
    if sparse_vector is None:
        return None
    if isinstance(sparse_vector, dict):
        if "indices" in sparse_vector:
            return sparse_vector
        return None
    if not sparse_vector.indices:
        return None
    return sparse_vector.to_dict_old()



def attach_sparse_vector(row: dict, sparse_vector: SparseVector | dict | None):
    payload = sparse_vector_to_payload(sparse_vector)
    if payload is None:
        row.pop(SPARSE_VECTOR_FIELD, None)
        return False
    row[SPARSE_VECTOR_FIELD] = payload
    return True



def build_sparse_text(*parts: Any) -> str:
    # Future multivector extension should keep visual/page inputs separate from this text-only sparse helper.
    values = []
    for part in parts:
        if part is None:
            continue
        if isinstance(part, list):
            values.extend(str(item) for item in part if item is not None)
        else:
            values.append(str(part))
    return "\n".join(value for value in values if value)


def dense_vector_field_name(vector_size: int) -> str:
    return f"q_{int(vector_size)}_vec"
