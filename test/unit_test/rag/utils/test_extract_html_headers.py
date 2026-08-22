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

from rag.utils import file_utils


class _Response:
    content = b"<html />"
    url = "https://example.test/"
    status_code = 200

    def __init__(self):
        self.headers = {"Content-Type": "text/html"}

    def raise_for_status(self):
        pass


def test_extract_html_does_not_reuse_request_headers(monkeypatch):
    monkeypatch.setattr(file_utils, "_GLOBAL_SESSION", None)
    request_headers = []

    def fake_get(self, url, timeout, headers=None):
        request_headers.append({**self.headers, **(headers or {})})
        return _Response()

    monkeypatch.setattr(file_utils.requests.Session, "get", fake_get)

    file_utils.extract_html("https://example.test/one", headers={"Authorization": "Bearer request-one"})
    file_utils.extract_html("https://example.test/two")

    assert request_headers[0]["Authorization"] == "Bearer request-one"
    assert "Authorization" not in request_headers[1]
