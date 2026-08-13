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

from docx import Document
from docx.oxml import OxmlElement
from docx.oxml.ns import qn

from deepdoc.parser.docx_numbering import apply_numbered_headings_to_markdown, extract_numbered_headings


def _element(tag, value=None, **attributes):
    element = OxmlElement(tag)
    if value is not None:
        element.set(qn("w:val"), str(value))
    for name, attribute_value in attributes.items():
        element.set(qn(f"w:{name}"), str(attribute_value))
    return element


def _add_numbering_level(abstract, level, style_id, level_text):
    definition = _element("w:lvl", ilvl=level)
    definition.append(_element("w:start", 1))
    definition.append(_element("w:numFmt", "decimal"))
    definition.append(_element("w:pStyle", style_id))
    definition.append(_element("w:lvlText", level_text))
    abstract.append(definition)


def _set_style_numbering(style, number_id, level):
    properties = style.element.get_or_add_pPr()
    for existing in properties.findall(qn("w:numPr")):
        properties.remove(existing)
    numbering = _element("w:numPr")
    numbering.append(_element("w:ilvl", level))
    numbering.append(_element("w:numId", number_id))
    properties.append(numbering)


def _numbered_document():
    document = Document()
    numbering_root = document.part.numbering_part.element
    abstract_id = 4242
    number_id = 4242

    abstract = _element("w:abstractNum", abstractNumId=abstract_id)
    _add_numbering_level(abstract, 0, "Heading1", "%1")
    _add_numbering_level(abstract, 1, "Heading2", "%1.%2")
    numbering_root.append(abstract)

    number = _element("w:num", numId=number_id)
    number.append(_element("w:abstractNumId", abstract_id))
    numbering_root.append(number)

    _set_style_numbering(document.styles["Heading 1"], number_id, 0)
    _set_style_numbering(document.styles["Heading 2"], number_id, 1)
    document.add_paragraph("Введение", style="Heading 1")
    document.add_paragraph("Порядок работы", style="Heading 1")
    document.add_paragraph("Начало работы", style="Heading 2")

    stream = BytesIO()
    document.save(stream)
    stream.seek(0)
    return Document(stream)


def test_extract_numbered_headings_from_style_numbering():
    headings = extract_numbered_headings(_numbered_document())

    assert [(heading.numbered_text, heading.level) for heading in headings] == [
        ("1 Введение", 1),
        ("2 Порядок работы", 1),
        ("2.1 Начало работы", 2),
    ]


def test_apply_numbered_headings_to_markdown():
    headings = extract_numbered_headings(_numbered_document())

    markdown = "# Введение\n\n# Порядок работы\n\n## Начало работы\n"

    assert apply_numbered_headings_to_markdown(markdown, headings) == "# 1 Введение\n\n# 2 Порядок работы\n\n## 2.1 Начало работы"


def test_apply_numbered_headings_to_setext_markdown():
    headings = extract_numbered_headings(_numbered_document())

    markdown = "Введение\n========\n\nПорядок работы\n===============\n\nНачало работы\n-------------\n"

    assert apply_numbered_headings_to_markdown(markdown, headings) == "1 Введение\n========\n\n2 Порядок работы\n===============\n\n2.1 Начало работы\n-------------"
