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

import json
import logging

from rag.utils import ob_conn


def test_truncates_at_utf8_boundary_without_logging_content(caplog):
    keyword = "sensitive-" + "中" * 100

    with caplog.at_level(logging.WARNING, logger="ragflow.ob_conn"):
        normalized, truncated = ob_conn._sanitize_important_keywords([keyword])

    assert truncated is True
    assert len(normalized[0].encode("utf-8")) <= ob_conn._IMPORTANT_KEYWORD_MAX_BYTES
    assert normalized[0].encode("utf-8").decode("utf-8") == normalized[0]
    assert caplog.records
    assert keyword not in caplog.records[0].getMessage()
    assert str(len(keyword.encode("utf-8"))) in caplog.records[0].getMessage()


def test_recomputes_tokens_from_stored_keywords_without_mutating_input(monkeypatch):
    keyword = "x" * (ob_conn._IMPORTANT_KEYWORD_MAX_BYTES + 1)
    fields = {"important_kwd": [keyword], "important_tks": "stale tokens"}
    monkeypatch.setattr(ob_conn.rag_tokenizer, "tokenize", lambda text: f"tokens:{text}")

    normalized = ob_conn._normalize_important_keyword_fields(fields)

    assert fields["important_kwd"] == [keyword]
    assert fields["important_tks"] == "stale tokens"
    assert normalized["important_kwd"] == ["x" * ob_conn._IMPORTANT_KEYWORD_MAX_BYTES]
    assert normalized["important_tks"] == "tokens:" + "x" * ob_conn._IMPORTANT_KEYWORD_MAX_BYTES


def test_json_escaping_does_not_change_stored_keyword_length():
    keyword = "\\" * 200

    normalized, truncated = ob_conn._sanitize_important_keywords([keyword])
    stored = json.loads(json.dumps(normalized, ensure_ascii=False))[0]

    assert truncated is False
    assert stored == keyword
    assert len(stored.encode("utf-8")) == 200
