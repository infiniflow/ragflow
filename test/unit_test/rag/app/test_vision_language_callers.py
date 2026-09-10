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
import re
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


@pytest.mark.p1
def test_mistral_ocr_forwards_language_to_parser(monkeypatch):
    parser = Mock()
    parser.parse_pdf.return_value = (["section"], [])
    ocr_model = Mock(mdl=parser)
    monkeypatch.setattr(naive, "resolve_model_config", Mock(return_value={"llm_name": "mistral-ocr"}))
    monkeypatch.setattr(naive, "LLMBundle", Mock(return_value=ocr_model))

    sections, tables, returned_parser = naive.by_mistral_ocr(
        "document.pdf",
        binary=b"pdf",
        from_page=2,
        to_page=5,
        lang="Japanese",
        callback=lambda *_args, **_kwargs: None,
        parse_method="raw",
        mistral_ocr_llm_name="mistral-ocr",
        tenant_id="tenant-id",
        vision_model=object(),
    )

    assert sections == ["section"]
    assert tables == []
    assert returned_parser is parser
    assert parser.parse_pdf.call_args.kwargs["lang"] == "Japanese"


@pytest.mark.p1
def test_mineru_forwards_dataset_language_to_parser(monkeypatch):
    parser = Mock()
    parser.parse_pdf.return_value = (["section"], [])
    ocr_model = Mock(mdl=parser)
    monkeypatch.setattr(naive, "resolve_model_config", Mock(return_value={"llm_name": "mineru"}))
    monkeypatch.setattr(naive, "LLMBundle", Mock(return_value=ocr_model))

    sections, tables, returned_parser = naive.by_mineru(
        "document.pdf",
        binary=b"pdf",
        from_page=2,
        to_page=5,
        lang="Japanese",
        callback=lambda *_args, **_kwargs: None,
        parse_method="raw",
        mineru_llm_name="mineru",
        tenant_id="tenant-id",
        vision_model=object(),
    )

    assert sections == ["section"]
    assert tables == []
    assert returned_parser is parser
    assert parser.parse_pdf.call_args.kwargs["lang"] == "Japanese"


@pytest.mark.p1
@pytest.mark.parametrize(
    "parser_error",
    [
        RuntimeError("[MinerU] server request failed"),
        FileNotFoundError("[MinerU] PDF not found"),
    ],
)
def test_mineru_propagates_parser_error(monkeypatch, parser_error):
    parser = Mock()
    parser.parse_pdf.side_effect = parser_error
    ocr_model = Mock(mdl=parser)
    callback = Mock()
    monkeypatch.setattr(naive, "resolve_model_config", Mock(return_value={"llm_name": "mineru"}))
    monkeypatch.setattr(naive, "LLMBundle", Mock(return_value=ocr_model))

    with pytest.raises(type(parser_error)) as exc_info:
        naive.by_mineru(
            "document.pdf",
            binary=b"pdf",
            callback=callback,
            mineru_llm_name="mineru",
            tenant_id="tenant-id",
            vision_model=object(),
        )

    assert exc_info.value is parser_error
    callback.assert_not_called()


@pytest.mark.p1
def test_mineru_raises_when_model_is_not_configured(monkeypatch):
    callback = Mock()
    monkeypatch.setattr(naive, "get_first_provider_model_name", Mock(return_value=None))
    monkeypatch.setattr(naive, "ensure_mineru_from_env", Mock(return_value=None))

    with pytest.raises(RuntimeError, match="MinerU model not found or not configured"):
        naive.by_mineru(
            "document.pdf",
            binary=b"pdf",
            callback=callback,
            tenant_id="tenant-id",
        )

    callback.assert_not_called()


@pytest.mark.p1
def test_manual_mineru_preserves_global_page_position(monkeypatch):
    from rag.app import manual

    class _MinerUParser:
        outlines = []
        page_from = 12

        @staticmethod
        def extract_positions(text):
            positions = []
            for tag in re.findall(r"@@[0-9-]+\t[0-9.\t]+##", text):
                page, left, right, top, bottom = tag.strip("#").strip("@").split("\t")
                positions.append(([int(page) - 1], float(left), float(right), float(top), float(bottom)))
            return positions

        @staticmethod
        def remove_tag(text):
            return re.sub(r"@@[\t0-9.-]+?##", "", text)

        def crop(self, text, need_position=False):
            positions = [(pages[0] + self.page_from, left, right, top, bottom) for pages, left, right, top, bottom in self.extract_positions(text) if pages and pages[0] >= 0]
            return (None, positions) if need_position else None

    parser = _MinerUParser()
    parse = Mock(return_value=([("Document body", "text", "@@1\t10.0\t190.0\t50.0\t100.0##")], [], parser))
    monkeypatch.setattr(manual, "PARSERS", {"mineru": parse})
    monkeypatch.setattr(manual, "normalize_layout_recognizer", lambda _value: ("MinerU", None))
    monkeypatch.setattr(manual, "extract_pdf_outlines", lambda *_args, **_kwargs: [])
    monkeypatch.setattr(manual, "bullets_category", lambda _texts: [0])
    monkeypatch.setattr(manual, "title_frequency", lambda _bullets, _sections: (0, [0]))
    monkeypatch.setattr(manual.rag_tokenizer, "tokenize", lambda text: text)
    monkeypatch.setattr(manual.rag_tokenizer, "fine_grained_tokenize", lambda text: text)
    monkeypatch.setattr(manual, "num_tokens_from_string", lambda text: len(text.split()))

    chunks = manual.chunk(
        "document.pdf",
        binary=b"%PDF-1.4 fake",
        from_page=12,
        to_page=15,
        lang="English",
        callback=lambda *_args, **_kwargs: None,
        parser_config={"layout_recognize": "MinerU", "chunk_token_num": 128, "delimiter": "\n"},
    )

    assert len(chunks) == 1
    assert chunks[0]["page_num_int"] == [13]
    assert chunks[0]["position_int"][0][0] == 13
