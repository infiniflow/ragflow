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
import pytest

from ragflow_sdk import DataSet


@pytest.mark.p2
def test_sdk_delete_documents_sends_empty_ids_to_api():
    class _Response:
        def json(self):
            return {"code": 101, "message": "should either provide doc ids or set delete_all(true), dataset: ds-1"}

    class _Rag:
        def __init__(self):
            self.calls = []

        def delete(self, path, json):
            self.calls.append((path, json))
            return _Response()

    rag = _Rag()
    dataset = DataSet(rag, {"id": "ds-1"})

    with pytest.raises(Exception) as exception_info:
        dataset.delete_documents(ids=[])

    assert "should either provide doc ids or set delete_all(true), dataset:" in str(exception_info.value)
    assert rag.calls == [("/datasets/ds-1/documents", {"ids": [], "delete_all": False})]
