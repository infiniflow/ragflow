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

import importlib
import sys


def test_token_chunker_imports_without_pdf_parser_symbols(pdf_parser_stub, monkeypatch):
    monkeypatch.delattr(pdf_parser_stub, "RAGFlowPdfParser")
    for module_name in [
        "rag.flow.chunker",
        "rag.flow.chunker.token_chunker",
        "rag.flow.parser.pdf_chunk_metadata",
    ]:
        sys.modules.pop(module_name, None)

    token_chunker = importlib.import_module("rag.flow.chunker.token_chunker")

    assert callable(token_chunker._merge_text_chunks_by_token_size)

    position_tag = "@@1\t10\t20\t30\t40##"
    extracted_tags = []

    class RuntimePdfParser:
        @staticmethod
        def extract_positions(value):
            extracted_tags.append(value)
            return [([0], 10.0, 20.0, 30.0, 40.0)]

    monkeypatch.setattr(pdf_parser_stub, "RAGFlowPdfParser", RuntimePdfParser, raising=False)

    assert token_chunker.extract_pdf_positions({"position_tag": position_tag}) == [[1, 10.0, 20.0, 30.0, 40.0]]
    assert extracted_tags == [position_tag]
