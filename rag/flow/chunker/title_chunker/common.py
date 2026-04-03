#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

import random
import re
import sys
from abc import ABC, abstractmethod
from collections import Counter
from copy import deepcopy

from deepdoc.parser.pdf_parser import RAGFlowPdfParser
from deepdoc.parser.utils import extract_pdf_outlines
from rag.flow.base import ProcessBase, ProcessParamBase
from rag.flow.parser.pdf_chunk_metadata import (
    PDF_POSITIONS_KEY,
    extract_pdf_positions,
    finalize_pdf_chunk,
    merge_pdf_positions,
    restore_pdf_text_previews,
)
from rag.nlp import not_bullet, not_title

BODY_LEVEL = sys.maxsize - 1


class TitleChunkerParam(ProcessParamBase):
    def __init__(self):
        super().__init__()
        self.levels = []
        self.hierarchy = None
        self.include_heading_content = False

    def check(self):
        if self.method in {"hierarchy", "group"}:
            self.check_empty(self.levels, "Hierarchical setups.")
        if self.method == "hierarchy":
            self.check_empty(self.hierarchy, "Hierarchy number.")

    def get_input_form(self) -> dict[str, dict]:
        return {}


class BaseTitleChunker(ABC):
    start_message = "Start to chunk by title."

    def __init__(self, process: ProcessBase, from_upstream):
        self.process = process
        self.param = process._param
        self.from_upstream = from_upstream


    async def invoke(self):
        self.process.set_output("output_format", "chunks")
        self.process.callback(random.randint(1, 5) / 100.0, self.start_message)
        line_records = self.extract_line_records()
        resolved = self.resolve_levels(line_records)
        chunks = self.build_chunks(line_records, resolved)
        await self.set_chunks(chunks)
        self.process.callback(1, "Done.")


    def extract_line_records(self):
        # Normalize all upstream payloads into an ordered record stream.
        # Level resolution and chunk construction operate on this stream only,
        # so strategy code does not depend on source-specific output layouts.
        if self.from_upstream.output_format == "markdown":
            payload = self.from_upstream.markdown_result or ""
            return [{"text": line, "doc_type_kwd": "text", "img_id": None, "layout": "", PDF_POSITIONS_KEY: []} for line in payload.split("\n") if line]

        if self.from_upstream.output_format == "text":
            payload = self.from_upstream.text_result or ""
            return [{"text": line, "doc_type_kwd": "text", "img_id": None, "layout": "", PDF_POSITIONS_KEY: []} for line in payload.split("\n") if line]

        if self.from_upstream.output_format == "html":
            payload = self.from_upstream.html_result or ""
            return [{"text": line, "doc_type_kwd": "text", "img_id": None, "layout": "", PDF_POSITIONS_KEY: []} for line in payload.split("\n") if line]

        items = self.from_upstream.chunks if self.from_upstream.output_format == "chunks" else self.from_upstream.json_result
        return [
            {
                "text": str(item.get("text") or ""),
                "doc_type_kwd": str(item.get("doc_type_kwd") or "text"),
                "img_id": item.get("img_id"),
                "layout": "{} {}".format(item.get("layout_type", ""), item.get("layoutno", "")).strip(),
                PDF_POSITIONS_KEY: extract_pdf_positions(item),
            }
            for item in items or []
        ]


    def extract_outlines(self):
        file = self.from_upstream.file or {}
        source = (
            file.get("blob")
            or file.get("binary")
            or file.get("path")
            or file.get("name")
        )
        if not source:
            return []
        return extract_pdf_outlines(source)


    @staticmethod
    def match_regex_level(text, level_group):
        stripped = text.strip()
        for level, pattern in enumerate(level_group, start=1):
            if re.match(pattern, stripped) and not not_bullet(stripped):
                return level
        return None


    @staticmethod
    def select_level_group(lines, raw_levels):
        if not raw_levels:
            return []

        # Select one regex family before assigning numeric levels. Mixing
        # patterns across families would make the level numbers ambiguous and
        # break downstream comparisons.
        hits = [0] * len(raw_levels)
        for i, group in enumerate(raw_levels):
            for sec in lines:
                sec = sec.strip()
                if not sec:
                    continue
                for pattern in group:
                    if re.match(pattern, sec) and not not_bullet(sec):
                        hits[i] += 1
                        break

        maximum = 0
        selected = -1
        for i, hit in enumerate(hits):
            if hit <= maximum:
                continue
            selected = i
            maximum = hit

        if selected < 0:
            return []
        return [pattern for pattern in raw_levels[selected] if pattern]


    @staticmethod
    def match_layout_level(text, layout, fallback_level):
        if re.search(r"(section|title|head)", layout, re.I) and not not_title(text.split("@")[0].strip()):
            return fallback_level
        return BODY_LEVEL


    @staticmethod
    def _outline_similarity(left, right):
        left_pairs = {left[i] + left[i + 1] for i in range(len(left) - 1)}
        right_pairs = {right[i] + right[i + 1] for i in range(min(len(left), len(right) - 1))}
        return len(left_pairs & right_pairs) / max(len(left_pairs), len(right_pairs), 1)


    def resolve_outline_levels(self, line_records):
        outlines = self.extract_outlines()
        if not line_records or len(outlines) / len(line_records) <= 0.03:
            return None

        max_level = max(level for _, level, _ in outlines) + 1
        levels = []
        for record in line_records:
            if record["doc_type_kwd"] != "text":
                levels.append(BODY_LEVEL)
                continue
            text = record["text"]
            for outline_text, level, _ in outlines:
                if self._outline_similarity(outline_text, text) > 0.8:
                    levels.append(level + 1)
                    break
            else:
                levels.append(BODY_LEVEL)

        return {
            "levels": levels,
            "most_level": max(1, max_level - 1),
            "source": "outline",
        }


    def resolve_frequency_levels(self, line_records):
        level_group = self.select_level_group(
            [record["text"] for record in line_records],
            self.param.levels,
        )
        fallback_level = len(level_group) + 1
        levels = []
        for record in line_records:
            if record["doc_type_kwd"] != "text":
                levels.append(BODY_LEVEL)
                continue
            level = self.match_regex_level(record["text"], level_group)
            if level is not None:
                levels.append(level)
                continue
            levels.append(
                self.match_layout_level(
                    record["text"],
                    record["layout"],
                    fallback_level,
                )
            )

        most_level = None
        for level, _ in Counter(levels).most_common():
            if level < BODY_LEVEL:
                most_level = level
                break

        return {
            "levels": levels,
            "most_level": most_level,
            "source": "frequency",
        }


    def resolve_title_levels(self, line_records):
        return self.resolve_outline_levels(line_records) or self.resolve_frequency_levels(line_records)


    def resolve_manual_levels(self, line_records):
        return self.resolve_title_levels(line_records)["levels"]


    def build_chunks_from_record_groups(self, record_groups):
        # Strategy code decides record grouping. This method materializes each
        # group into the output chunk representation. For PDF-like inputs, the
        # chunk box is defined by merged source positions and the text payload
        # is normalized by removing parser tags.
        if self.from_upstream.output_format in ["markdown", "text", "html"]:
            return [
                {"text": "".join(record["text"] + "\n" for record in records)}
                for records in record_groups
                if records
            ]

        return [
            (
                {
                    "text": RAGFlowPdfParser.remove_tag("".join(record["text"] + "\n" for record in records)),
                    "doc_type_kwd": "text",
                    PDF_POSITIONS_KEY: merge_pdf_positions(records),
                }
                if records[0]["doc_type_kwd"] == "text"
                else {
                    "text": records[0]["text"],
                    "doc_type_kwd": records[0]["doc_type_kwd"],
                    "img_id": records[0]["img_id"],
                    PDF_POSITIONS_KEY: records[0][PDF_POSITIONS_KEY],
                }
            )
            for records in record_groups
            if records
        ]


    async def set_chunks(self, chunks):
        if self.from_upstream.output_format in ["markdown", "text", "html"]:
            self.process.set_output("chunks", chunks)
            return

        # Text grouping runs before visual enrichment. Preview text and final
        # box metadata are derived here from the merged PDF positions.
        await restore_pdf_text_previews(chunks, self.from_upstream, self.process._canvas)
        self.process.set_output("chunks", [finalize_pdf_chunk(deepcopy(chunk)) for chunk in chunks])


    @abstractmethod
    def resolve_levels(self, line_records):
        raise NotImplementedError()


    @abstractmethod
    def build_chunks(self, line_records, resolved):
        raise NotImplementedError()


def resolve_target_level(levels, hierarchy):
    title_levels = sorted({level for level in levels if 0 < level < BODY_LEVEL})
    if not title_levels:
        return None

    hierarchy_num = max(int(hierarchy), 1)
    return title_levels[min(hierarchy_num, len(title_levels)) - 1]
