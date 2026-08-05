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
import ast
from pathlib import Path
from unittest.mock import Mock

import pytest

from rag.app import naive

REPO_ROOT = Path(__file__).resolve().parents[4]


def _call_name(call):
    if isinstance(call.func, ast.Name):
        return call.func.id
    if isinstance(call.func, ast.Attribute):
        return call.func.attr
    return None


@pytest.mark.p1
@pytest.mark.parametrize(
    ("relative_path", "expected_call_count"),
    [
        ("rag/app/book.py", 1),
        ("rag/app/manual.py", 2),
        ("rag/app/naive.py", 2),
        ("rag/app/one.py", 1),
        ("rag/app/paper.py", 1),
        ("rag/app/table.py", 1),
    ],
)
def test_all_figure_wrapper_callers_forward_language(relative_path, expected_call_count):
    tree = ast.parse((REPO_ROOT / relative_path).read_text())
    calls = [node for node in ast.walk(tree) if isinstance(node, ast.Call) and (_call_name(node) or "").startswith("vision_figure_parser_")]

    assert len(calls) == expected_call_count
    for call in calls:
        language = next((keyword.value for keyword in call.keywords if keyword.arg == "lang"), None)
        assert isinstance(language, ast.Name), f"{relative_path}:{call.lineno} does not forward lang"
        assert language.id == "lang"


@pytest.mark.p1
def test_markdown_chunk_forwards_language_to_model_and_figure_parser(monkeypatch):
    markdown_parser = Mock(return_value=([("section", "")], [], [object()]))
    monkeypatch.setattr(naive, "Markdown", Mock(return_value=markdown_parser))
    monkeypatch.setattr(naive, "get_tenant_default_model_by_type", Mock(return_value={"llm_name": "vision-model"}))

    vision_model = object()
    llm_bundle = Mock(return_value=vision_model)
    monkeypatch.setattr(naive, "LLMBundle", llm_bundle)

    parser_instance = Mock(return_value=[((None, "description"), None)])
    parser_factory = Mock(return_value=parser_instance)
    monkeypatch.setattr(naive, "VisionFigureParser", parser_factory)

    monkeypatch.setattr(naive.rag_tokenizer, "tokenize", lambda text: text)
    monkeypatch.setattr(naive.rag_tokenizer, "fine_grained_tokenize", lambda text: text)
    monkeypatch.setattr(naive, "num_tokens_from_string", lambda _text: 1)
    monkeypatch.setattr(naive, "tokenize_table", Mock(return_value=[]))
    monkeypatch.setattr(naive, "tokenize_chunks", Mock(return_value=[]))
    monkeypatch.setattr(naive, "tokenize_chunks_with_images", Mock(return_value=[]))

    naive.chunk(
        "document.md",
        binary=b"markdown",
        lang="Japanese",
        callback=lambda *_args, **_kwargs: None,
        tenant_id="tenant-id",
        is_root=False,
        parser_config={
            "chunk_token_num": 128,
            "delimiter": "\n",
            "analyze_hyperlink": False,
        },
    )

    llm_bundle.assert_called_once_with(
        "tenant-id",
        {"llm_name": "vision-model"},
        lang="Japanese",
    )
    assert parser_factory.call_args.kwargs["vision_model"] is vision_model
    assert parser_factory.call_args.kwargs["lang"] == "Japanese"
    parser_instance.assert_called_once()
