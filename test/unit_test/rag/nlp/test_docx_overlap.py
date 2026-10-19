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
import subprocess
import sys
import textwrap

import pytest
from PIL import Image

from rag import nlp
from rag.nlp import naive_merge_docx


@pytest.fixture(autouse=True)
def count_characters(monkeypatch):
    monkeypatch.setattr(nlp, "num_tokens_from_string", lambda text: len(text.strip()))


def test_docx_builtin_applies_configured_overlap():
    # Isolate the real parser from the PDF/LLM module stubs in the unit suite.
    result = subprocess.run(
        [
            sys.executable,
            "-c",
            textwrap.dedent("""
            import json
            from io import BytesIO
            from docx import Document
            from rag import nlp
            from rag.app import naive

            nlp.num_tokens_from_string = lambda text: len(text.strip())
            document = Document()
            document.add_paragraph("ABCDEFGHI。KLMNOPQRS。UVWXYZ123。")
            binary = BytesIO()
            document.save(binary)
            outputs = []
            for overlap in (0, 0.2, 20, 0.3, 30):
                chunks = naive.chunk(
                    "overlap.docx",
                    binary=binary.getvalue(),
                    callback=lambda *args, **kwargs: None,
                    is_root=False,
                    parser_config={"chunk_token_num": 10, "delimiter": "。", "overlapped_percent": overlap},
                )
                outputs.append([chunk["content_with_weight"] for chunk in chunks])
            print(json.dumps(outputs))
        """),
        ],
        capture_output=True,
        text=True,
        timeout=60,
    )
    assert result.returncode == 0, result.stderr
    outputs = json.loads(result.stdout.strip().splitlines()[-1])
    assert outputs == [
        ["ABCDEFGHI。", "KLMNOPQRS。", "UVWXYZ123。"],
        ["ABCDEFGHI。", "I。KLMNOPQRS。", "RS。UVWXYZ123。"],
        ["ABCDEFGHI。", "I。KLMNOPQRS。", "RS。UVWXYZ123。"],
        ["ABCDEFGHI。", "HI。KLMNOPQRS。", "QRS。UVWXYZ123。"],
        ["ABCDEFGHI。", "HI。KLMNOPQRS。", "QRS。UVWXYZ123。"],
    ]


def test_docx_overlap_reserves_chunk_budget():
    chunks, _ = naive_merge_docx(
        [("AAA。BBB。CCC。DDD。", None, None)],
        chunk_token_num=10,
        delimiter="。",
        overlapped_percent=20,
    )

    assert [chunk["text"] for chunk in chunks] == ["AAA。BBB。", "B。CCC。DDD。"]
    assert [chunk["tk_nums"] for chunk in chunks] == [8, 10]


def test_docx_zero_overlap_preserves_chunking():
    chunks, _ = naive_merge_docx(
        [("AAA。BBB。CCC。DDD。", None, None)],
        chunk_token_num=10,
        delimiter="。",
    )

    assert [chunk["text"] for chunk in chunks] == ["AAA。BBB。CCC。", "DDD。"]


@pytest.mark.parametrize("kind", ["table", "image"])
@pytest.mark.parametrize("overlap", [0, 20])
def test_docx_media_preserves_order_and_stops_overlap(kind, overlap):
    image = Image.new("RGB", (1, 1)) if kind == "image" else None
    table = "<table><tr><td>value</td></tr></table>" if kind == "table" else None
    chunks, image_indices = naive_merge_docx(
        [("AAA。", None, None), ("", image, table), ("BBB。CCC。", None, None)],
        chunk_token_num=10,
        delimiter="。",
        overlapped_percent=overlap,
    )

    assert [chunk["ck_type"] for chunk in chunks] == ["text", kind, "text"]
    assert chunks[0]["text"] == "AAA。"
    assert chunks[1]["text"] == (table or "")
    assert chunks[1]["image"] is image
    assert chunks[2]["text"] == "BBB。CCC。"
    assert image_indices == ([1] if kind == "image" else [])


def test_docx_custom_delimiter_remains_hard_boundary():
    chunks, _ = naive_merge_docx(
        [("ABCDEFGHI|KLMNOPQRS|UVWXYZ123", None, None)],
        chunk_token_num=100,
        delimiter="`|`",
        overlapped_percent=20,
    )

    assert [chunk["text"] for chunk in chunks] == ["ABCDEFGHI", "KLMNOPQRS", "UVWXYZ123"]
