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
from unittest.mock import Mock

import pytest

from rag.app import one


@pytest.mark.p1
def test_docx_chunk_forwards_language_to_vision_wrapper(monkeypatch, caplog):
    docx_parser = Mock(return_value=[("caption", object(), None)])
    monkeypatch.setattr(one.naive, "Docx", Mock(return_value=docx_parser))

    vision_wrapper = Mock()
    monkeypatch.setattr(one, "vision_figure_parser_docx_wrapper_naive", vision_wrapper)
    monkeypatch.setattr(one.rag_tokenizer, "tokenize", lambda text: text)
    monkeypatch.setattr(one.rag_tokenizer, "fine_grained_tokenize", lambda text: text)
    monkeypatch.setattr(one, "tokenize", Mock())

    with caplog.at_level(logging.INFO, logger=one.__name__):
        one.chunk(
            "document.docx",
            binary=b"docx",
            lang="Japanese",
            callback=lambda *_args, **_kwargs: None,
            tenant_id="tenant-id",
        )

    vision_wrapper.assert_called_once()
    args = vision_wrapper.call_args.args
    kwargs = vision_wrapper.call_args.kwargs
    assert args[1] == [0]
    assert kwargs["lang"] == "Japanese"
    assert kwargs["tenant_id"] == "tenant-id"
    assert "DOCX figure vision enhancement: language=Japanese image_count=1" in caplog.messages
