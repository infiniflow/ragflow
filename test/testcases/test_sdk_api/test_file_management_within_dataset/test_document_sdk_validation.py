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
import pytest

from ragflow_sdk.modules.document import Document


class _StubResponse:
    def __init__(self, payload, content=b""):
        self._payload = payload
        self.content = content

    def json(self):
        return self._payload


class _StubRag:
    def __init__(self):
        self.get_response = None
        self.put_response = None

    def get(self, *_args, **_kwargs):
        return self.get_response

    def put(self, *_args, **_kwargs):
        return self.put_response

    def delete(self, *_args, **_kwargs):
        return None


@pytest.mark.p2
def test_document_validation_and_error_paths():
    rag = _StubRag()
    doc = Document(rag, {"id": "doc_id", "dataset_id": "dataset_id"})

    with pytest.raises(Exception) as excinfo:
        doc.update({"meta_fields": "bad"})
    assert "meta_fields" in str(excinfo.value)

    rag.get_response = _StubResponse({"code": 1, "message": "download error"})
    with pytest.raises(Exception) as excinfo:
        doc.download()
    assert "download error" in str(excinfo.value)

    rag.get_response = _StubResponse({"code": 1, "message": "list error"})
    with pytest.raises(Exception) as excinfo:
        doc.list_chunks()
    assert "list error" in str(excinfo.value)
