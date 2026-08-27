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

from io import BytesIO

from pypdf import PdfWriter
from pypdf.annotations import Link
from pypdf.generic import NameObject

from rag.utils.file_utils import extract_links_from_pdf


def _pdf_with_indirect_annots_array(url: str) -> bytes:
    writer = PdfWriter()
    page = writer.add_blank_page(width=200, height=200)
    writer.add_annotation(0, Link(rect=(0, 0, 100, 100), url=url))

    # Keep the annotation array indirect to exercise the PDF form seen in the
    # wild, rather than the direct array produced by PdfWriter by default.
    page[NameObject("/Annots")] = writer._add_object(page["/Annots"])

    output = BytesIO()
    writer.write(output)
    return output.getvalue()


def test_extract_links_from_pdf_dereferences_indirect_annots_array():
    url = "https://example.com/indirect"

    links = extract_links_from_pdf(_pdf_with_indirect_annots_array(url))

    assert links == {url}


def test_extract_links_from_pdf_keeps_no_annots_tolerant():
    writer = PdfWriter()
    writer.add_blank_page(width=200, height=200)
    output = BytesIO()
    writer.write(output)

    assert extract_links_from_pdf(output.getvalue()) == set()
