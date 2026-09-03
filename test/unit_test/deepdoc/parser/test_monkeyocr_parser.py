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
from unittest.mock import Mock, patch

import pytest

from common.constants import MAXIMUM_PAGE_NUMBER
from deepdoc.parser.monkeyocr_parser import MonkeyOCRParser


@pytest.mark.p1
def test_parse_pdf_defaults_page_to_maximum_when_omitted(tmp_path):
    """Canvas/flow parser calls parse_pdf without page_to; default must match MinerU."""
    pdf_path = tmp_path / "document.pdf"
    pdf_path.write_bytes(b"%PDF-1.4 fake")
    parser = MonkeyOCRParser(monkeyocr_api="http://127.0.0.1:9000")
    parent_parse = Mock(return_value=([], []))

    with patch.object(MonkeyOCRParser.__bases__[0], "parse_pdf", parent_parse):
        parser.parse_pdf(
            filepath=str(pdf_path),
            binary=None,
            callback=None,
            parse_method="pipeline",
        )

    assert parent_parse.call_args.kwargs["page_to"] == MAXIMUM_PAGE_NUMBER
    assert parent_parse.call_args.kwargs["page_from"] == 0
