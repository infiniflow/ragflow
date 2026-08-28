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
import re
from dataclasses import dataclass

from docx.oxml.ns import qn

logger = logging.getLogger(__name__)


@dataclass(frozen=True)
class NumberedHeading:
    text: str
    numbered_text: str
    level: int


@dataclass(frozen=True)
class _NumberingLevel:
    start: int = 1
    number_format: str = "decimal"
    level_text: str = ""
    paragraph_style: str = ""
    restart_level: int | None = None


@dataclass(frozen=True)
class _NumberingInstance:
    abstract_id: int
    overrides: dict[int, _NumberingLevel]


class DOCXNumberingResolver:
    """Best-effort materialization of display-only Word heading numbering."""

    def __init__(self, document, enabled=False):
        self._enabled = enabled
        self._abstracts: dict[int, dict[int, _NumberingLevel]] = {}
        self._instances: dict[int, _NumberingInstance] = {}
        self._instance_order: list[int] = []
        self._counters: dict[int, list[int]] = {}
        if enabled:
            self._parse_numbering(document)

    def numbered_heading(self, paragraph) -> NumberedHeading | None:
        """Advance numbering state and return a materialized heading, if any."""
        if not self._enabled:
            return None
        heading_level = self._heading_level(paragraph)
        text = paragraph.text.strip()
        if heading_level is None or not text:
            return None

        reference = self._paragraph_numbering_reference(paragraph)
        if reference is None:
            return None

        number_id, list_level = reference
        definition = self._resolve_level(number_id, list_level)
        if definition is None or definition.number_format in {"bullet", "none"} or not definition.level_text:
            return None

        values = self._counters.setdefault(number_id, [0] * 9)
        values[list_level] = definition.start if values[list_level] == 0 else values[list_level] + 1
        self._restart_lower_levels(number_id, list_level, values)

        marker = self._render_marker(definition.level_text, number_id, values).strip()
        if not marker:
            return None
        numbered_text = text if text == marker or text.startswith(f"{marker} ") else f"{marker} {text}"
        return NumberedHeading(text=text, numbered_text=numbered_text, level=heading_level)

    def heading_level(self, paragraph) -> int | None:
        """Return the one-based outline level for a heading paragraph."""
        return self._heading_level(paragraph)

    def _parse_numbering(self, document):
        try:
            root = document.part.numbering_part.element
        except (AttributeError, KeyError):
            logger.debug("DOCX numbering part is unavailable")
            return

        for abstract in root.findall(qn("w:abstractNum")):
            abstract_id = _int_attribute(abstract, "w:abstractNumId")
            if abstract_id is None:
                continue
            levels = {}
            for level_element in abstract.findall(qn("w:lvl")):
                list_level = _int_attribute(level_element, "w:ilvl")
                if list_level is not None:
                    levels[list_level] = _parse_level(level_element)
            self._abstracts[abstract_id] = levels

        for number in root.findall(qn("w:num")):
            number_id = _int_attribute(number, "w:numId")
            abstract_id = _child_int(number, "w:abstractNumId")
            if number_id is None or abstract_id is None:
                continue
            overrides = {}
            for override in number.findall(qn("w:lvlOverride")):
                list_level = _int_attribute(override, "w:ilvl")
                if list_level is None:
                    continue
                definition = self._abstracts.get(abstract_id, {}).get(list_level)
                level_element = override.find(qn("w:lvl"))
                if level_element is not None:
                    overridden_definition = _parse_level(level_element)
                    definition = _NumberingLevel(
                        start=overridden_definition.start,
                        number_format=overridden_definition.number_format,
                        level_text=overridden_definition.level_text,
                        paragraph_style=overridden_definition.paragraph_style,
                        # Word ignores lvlRestart inside a level override.
                        restart_level=definition.restart_level if definition is not None else None,
                    )
                if definition is None:
                    continue
                start_override = _child_int(override, "w:startOverride")
                if start_override is not None:
                    definition = _NumberingLevel(
                        start=start_override,
                        number_format=definition.number_format,
                        level_text=definition.level_text,
                        paragraph_style=definition.paragraph_style,
                        restart_level=definition.restart_level,
                    )
                overrides[list_level] = definition
            self._instances[number_id] = _NumberingInstance(abstract_id=abstract_id, overrides=overrides)
            self._instance_order.append(number_id)
        logger.debug("Resolved %d DOCX numbering definitions", len(self._instances))

    def _paragraph_numbering_reference(self, paragraph):
        properties = paragraph._p.pPr
        if _is_numbering_disabled(properties):
            return None

        reference = _properties_numbering_reference(properties)
        if reference is not None:
            return reference

        style = paragraph.style
        seen = set()
        while style is not None and style.style_id not in seen:
            seen.add(style.style_id)
            if _is_numbering_disabled(style.element.pPr):
                return None
            reference = _properties_numbering_reference(style.element.pPr)
            if reference is not None:
                number_id, list_level = reference
                if style.element.pPr.numPr is not None and style.element.pPr.numPr.ilvl is None:
                    list_level = self._level_for_style(number_id, style.style_id, list_level)
                return number_id, list_level
            style = style.base_style

        style_id = paragraph.style.style_id if paragraph.style is not None else ""
        for number_id in self._instance_order:
            list_level = self._level_for_style(number_id, style_id, None)
            if list_level is not None:
                return number_id, list_level
        return None

    def _level_for_style(self, number_id, style_id, default):
        instance = self._instances.get(number_id)
        if instance is None:
            return default
        levels = dict(self._abstracts.get(instance.abstract_id, {}))
        levels.update(instance.overrides)
        for list_level, definition in levels.items():
            if definition.paragraph_style == style_id:
                return list_level
        return default

    def _resolve_level(self, number_id, list_level):
        if not 0 <= list_level < 9:
            return None
        instance = self._instances.get(number_id)
        if instance is None:
            return None
        return instance.overrides.get(list_level) or self._abstracts.get(instance.abstract_id, {}).get(list_level)

    def _heading_level(self, paragraph):
        style = paragraph.style
        if style is None:
            return None
        heading_level = _heading_style_level(style)
        if heading_level is None:
            return None

        properties = paragraph._p.pPr
        outline_level = _child_int(properties, "w:outlineLvl")
        if outline_level is not None and 0 <= outline_level < 9:
            return outline_level + 1

        seen = set()
        while style is not None and style.style_id not in seen:
            seen.add(style.style_id)
            outline_level = _child_int(style.element.pPr, "w:outlineLvl")
            if outline_level is not None and 0 <= outline_level < 9:
                return outline_level + 1
            style = style.base_style
        return heading_level

    def _restart_lower_levels(self, number_id, list_level, values):
        for index in range(list_level + 1, len(values)):
            definition = self._resolve_level(number_id, index)
            restart_level = definition.restart_level if definition is not None else None
            if restart_level == 0:
                continue
            if list_level < (restart_level if restart_level is not None else index):
                values[index] = 0

    def _render_marker(self, template, number_id, values):
        marker = template
        for index in range(9):
            placeholder = f"%{index + 1}"
            if placeholder not in marker:
                continue
            definition = self._resolve_level(number_id, index)
            value = values[index] or (definition.start if definition is not None else 1)
            number_format = definition.number_format if definition is not None else "decimal"
            marker = marker.replace(placeholder, _format_number(value, number_format))
        return marker


def extract_numbered_headings(document, enabled=False):
    resolver = DOCXNumberingResolver(document, enabled=enabled)
    headings = []
    for paragraph in document.paragraphs:
        heading = resolver.numbered_heading(paragraph)
        if heading is not None:
            headings.append(heading)
    return headings


def apply_numbered_headings_to_markdown(markdown, headings):
    if not markdown or not headings:
        return markdown
    lines = markdown.splitlines()
    heading_index = 0
    for index, line in enumerate(lines):
        match = re.match(r"^(#{1,6})\s+(.*?)\s*$", line)
        candidate = line.strip()
        is_setext_heading = (
            bool(candidate)
            and re.match(r"(?:[-+*]|\d+[.)])\s+", candidate) is None
            and "|" not in candidate
            and index + 1 < len(lines)
            and re.fullmatch(r"\s*(?:=+|-+)\s*", lines[index + 1]) is not None
        )
        if match is None and not is_setext_heading:
            continue
        text = re.sub(r"[*_~`]", "", match.group(2) if match is not None else line).strip()
        for candidate_index in range(heading_index, len(headings)):
            heading = headings[candidate_index]
            if text != heading.text:
                continue
            if match is not None:
                lines[index] = f"{'#' * min(max(heading.level, 1), 6)} {heading.numbered_text}"
            else:
                lines[index] = heading.numbered_text
            heading_index = candidate_index + 1
            break
    return "\n".join(lines)


def _properties_numbering_reference(properties):
    if properties is None or properties.numPr is None or properties.numPr.numId is None:
        return None
    number_id = _value_int(properties.numPr.numId)
    if number_id is None or number_id == 0:
        return None
    list_level = _value_int(properties.numPr.ilvl)
    return number_id, list_level or 0


def _is_numbering_disabled(properties):
    if properties is None or properties.numPr is None or properties.numPr.numId is None:
        return False
    return _value_int(properties.numPr.numId) == 0


def _parse_level(element):
    return _NumberingLevel(
        start=_child_int(element, "w:start") or 1,
        number_format=_child_value(element, "w:numFmt") or "decimal",
        level_text=_child_value(element, "w:lvlText") or "",
        paragraph_style=_child_value(element, "w:pStyle") or "",
        restart_level=_child_int(element, "w:lvlRestart"),
    )


def _heading_style_level(style):
    for name in (style.style_id, style.name):
        match = re.fullmatch(r"(?:heading|заголовок)\s*(\d+)", name or "", re.IGNORECASE)
        if match:
            level = int(match.group(1))
            return level if 1 <= level <= 9 else None
    return None


def _child_value(parent, tag):
    if parent is None:
        return None
    child = parent.find(qn(tag))
    return child.get(qn("w:val")) if child is not None else None


def _child_int(parent, tag):
    value = _child_value(parent, tag)
    try:
        return int(value) if value is not None else None
    except ValueError:
        return None


def _int_attribute(element, attribute):
    try:
        value = element.get(qn(attribute))
        return int(value) if value is not None else None
    except ValueError:
        return None


def _value_int(element):
    if element is None:
        return None
    try:
        return int(element.val)
    except (TypeError, ValueError):
        return None


def _format_number(value, number_format):
    if number_format == "lowerLetter":
        return _format_letters(value, False)
    if number_format == "upperLetter":
        return _format_letters(value, True)
    if number_format == "lowerRoman":
        return _format_roman(value).lower()
    if number_format == "upperRoman":
        return _format_roman(value)
    if number_format == "decimalZero" and 0 <= value < 10:
        return f"0{value}"
    return str(value)


def _format_letters(value, upper):
    if value <= 0:
        return str(value)
    result = ""
    base = ord("A" if upper else "a")
    while value > 0:
        value -= 1
        result = chr(base + value % 26) + result
        value //= 26
    return result


def _format_roman(value):
    if value <= 0 or value > 3999:
        return str(value)
    symbols = (
        (1000, "M"),
        (900, "CM"),
        (500, "D"),
        (400, "CD"),
        (100, "C"),
        (90, "XC"),
        (50, "L"),
        (40, "XL"),
        (10, "X"),
        (9, "IX"),
        (5, "V"),
        (4, "IV"),
        (1, "I"),
    )
    result = ""
    for number, symbol in symbols:
        while value >= number:
            result += symbol
            value -= number
    return result
