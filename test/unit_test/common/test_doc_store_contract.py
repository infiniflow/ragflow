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
"""
Contract tests for DocStoreConnection.

The retriever calls ``get_scores`` (rag/nlp/search.py) and the document
metadata service calls ``create_doc_meta_idx``
(api/db/services/doc_metadata_service.py) on every backend, but neither was
declared ``@abstractmethod``. A backend could omit one, pass a smoke test,
and only fail at runtime with ``AttributeError`` -- exactly the shape of
issue #14570 (``'OSConnection' object has no attribute 'create_doc_meta_idx'``).
The Go DocEngine interface (internal/engine/engine.go) already requires both
(GetScores, CreateMetadataStore) and compile-enforces them; these tests pin
the same contract on the Python side so a missing method is caught at
construction, not in production.
"""

from __future__ import annotations

import pytest

pytest.importorskip("numpy")  # doc_store_base imports numpy at module load

from common.doc_store.doc_store_base import DocStoreConnection  # noqa: E402

CONTRACT_METHODS = ("get_scores", "create_doc_meta_idx")


def _stub_body():
    """A concrete implementation of every abstract method on the ABC."""
    body = {}
    for name in DocStoreConnection.__abstractmethods__:
        body[name] = lambda self, *a, **k: None
    return body


@pytest.mark.parametrize("method", CONTRACT_METHODS)
def test_method_is_abstract(method):
    assert method in DocStoreConnection.__abstractmethods__


@pytest.mark.parametrize("missing", CONTRACT_METHODS)
def test_backend_missing_a_contract_method_cannot_be_instantiated(missing):
    body = _stub_body()
    del body[missing]
    partial = type("PartialBackend", (DocStoreConnection,), body)
    with pytest.raises(TypeError, match=missing):
        partial()


def test_backend_implementing_the_full_contract_instantiates():
    complete = type("CompleteBackend", (DocStoreConnection,), _stub_body())
    assert complete().get_scores(None) is None
