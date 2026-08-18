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

from common.token_utils import num_tokens_from_string, truncate
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

# Sentence-boundary punctuation used to re-split an oversized title chunk.
# Covers Chinese/English period, exclamation mark, question mark, the Chinese
# variants, and the newline. The capturing group keeps the delimiter attached
# to the preceding sentence so re-merged text preserves original boundaries.
_SENTENCE_BOUNDARY_RE = re.compile(r"([。!?？；！\n]|\. )", re.DOTALL)


class TitleChunkerParam(ProcessParamBase):
    def __init__(self):
        super().__init__()
        self.levels = []
        self.hierarchy = None
        self.include_heading_content = False
        self.root_chunk_as_heading = False
        # Hard ceiling on each text chunk's token count. A built chunk that
        # exceeds it is re-split on sentence boundaries into <= cap sub-chunks
        # (see BaseTitleChunker._enforce_token_cap). 0/None disables the
        # ceiling for full backward compatibility.
        self.chunk_token_cap = 512

    def check(self):
        if self.method in {"hierarchy", "group"}:
            self.check_empty(self.levels, "Hierarchical setups.")
        if self.method == "hierarchy":
            self.check_empty(self.hierarchy, "Hierarchy number.")
        if self.chunk_token_cap:
            self.check_positive_integer(self.chunk_token_cap, "Chunk token cap.")
            if not (128 <= int(self.chunk_token_cap) <= 8000):
                raise ValueError("Chunk token cap must be between 128 and 8000.")

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
        chunks = self._enforce_token_cap(chunks)
        await self.set_chunks(chunks)
        self.process.callback(1, "Done.")

    @staticmethod
    def _split_text_by_sentences(text):
        # Split on sentence boundaries, keeping the delimiter attached to the
        # preceding sentence so re-merged text preserves original punctuation.
        if not text:
            return []
        raw = re.split(_SENTENCE_BOUNDARY_RE, text)
        sentences = []
        for i in range(0, len(raw), 2):
            piece = raw[i]
            if i + 1 < len(raw):
                piece += raw[i + 1]
            if piece:
                sentences.append(piece)
        return sentences

    @staticmethod
    def _hard_split_by_tokens(text, cap):
        # Fallback for a single boundary-less run that still exceeds the cap:
        # truncate at token boundaries until every piece is within the cap.
        # Mirrors common.token_utils.truncate semantics (prefix <= cap tokens).
        out = []
        rest = text or ""
        while rest:
            head = truncate(rest, cap)
            if not head:
                out.append(rest)
                break
            out.append(head)
            if head == rest:
                break
            rest = rest[len(head):]
        return out

    def _split_text_chunk_by_cap(self, chunk, cap):
        # Re-split one oversized text chunk into <= cap sub-chunks. Sentence
        # boundaries first; any remaining over-cap segment is hard-split.
        text = chunk.get("text") or ""
        sentences = self._split_text_by_sentences(text)
        if not sentences:
            return [chunk]

        groups = []
        current = ""
        for sentence in sentences:
            candidate = current + sentence if current else sentence
            if current and num_tokens_from_string(candidate) > cap:
                groups.append(current)
                current = sentence
            else:
                current = candidate
        if current:
            groups.append(current)

        final_groups = []
        for group in groups:
            if num_tokens_from_string(group) <= cap:
                final_groups.append(group)
            else:
                final_groups.extend(self._hard_split_by_tokens(group, cap))
        if not final_groups:
            return [chunk]

        has_positions = PDF_POSITIONS_KEY in chunk
        orig_positions = chunk.get(PDF_POSITIONS_KEY)
        out = []
        for i, group in enumerate(final_groups):
            sub = dict(chunk)
            sub["text"] = group
            # Plan A: the original (coarse, page-level) coordinates attach to the
            # first sub-chunk only; subsequent sub-chunks carry none.
            if has_positions:
                sub[PDF_POSITIONS_KEY] = orig_positions if i == 0 else []
            out.append(sub)
        return out

    def _enforce_token_cap(self, chunks):
        # Hard token ceiling applied to every text chunk after build_chunks,
        # so both hierarchy and group methods honour chunk_token_cap. Table and
        # image chunks are atomic and skipped. cap == 0/None disables it.
        cap = self.param.chunk_token_cap
        if not cap or cap <= 0:
            return chunks

        out = []
        for chunk in chunks:
            if chunk.get("doc_type_kwd", "text") != "text":
                out.append(chunk)
                continue
            if num_tokens_from_string(chunk.get("text") or "") <= cap:
                out.append(chunk)
                continue
            out.extend(self._split_text_chunk_by_cap(chunk, cap))
        return out

    def extract_line_records(self):
        """
        Normalize all upstream input payloads into a unified ordered record stream.
        All level resolution and chunk construction logic operates on this standard stream,
        decoupling downstream chunking strategies from different upstream output formats.
        """
        import logging

        logger = logging.getLogger(__name__)

        payload = None
        # Extract raw content payload based on upstream output format type
        if self.from_upstream.output_format == "markdown":
            payload = self.from_upstream.markdown_result or ""
        elif self.from_upstream.output_format == "text":
            payload = self.from_upstream.text_result or ""
        elif self.from_upstream.output_format == "html":
            payload = self.from_upstream.html_result or ""

        # Boundary robustness fix: explicit None check to distinguish `None` and empty string ""
        # Prevents empty payload from unexpectedly falling through to structured chunk branch
        if payload is not None:
            lines = payload.split("\n")
            input_line_count = len(lines)

            # Format-branched text processing to preserve original document semantics
            # Plain text: perform full whitespace stripping and invalid empty line filtering
            if self.from_upstream.output_format == "text":
                clean_lines = [line.strip() for line in lines if line.strip()]
            # Markdown & HTML: retain original indentation/spacing, only filter pure blank lines
            else:
                clean_lines = [line for line in lines if line.strip()]

            output_line_count = len(clean_lines)
            # Production observability log: added format dimension per project coding guidelines
            logger.info(f"payload filter: format={self.from_upstream.output_format} before={input_line_count} after={output_line_count}")

            return [{"text": line, "doc_type_kwd": "text", "img_id": None, "layout": "", PDF_POSITIONS_KEY: []} for line in clean_lines]
        items = self.from_upstream.chunks if self.from_upstream.output_format == "chunks" else self.from_upstream.json_result
        return [
            {
                "text": item.get("text") or "",
                "doc_type_kwd": str(item.get("doc_type_kwd") or "text"),
                "img_id": item.get("img_id"),
                "layout": "{} {}".format(item.get("layout_type", ""), item.get("layoutno", "")).strip(),
                PDF_POSITIONS_KEY: extract_pdf_positions(item),
            }
            for item in items or []
        ]

    def extract_outlines(self):
        file = self.from_upstream.file or {}
        source = file.get("blob") or file.get("binary") or file.get("path") or file.get("name")
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
        if re.search(r"(section|title|head)", layout, re.IGNORECASE) and not not_title(text.split("@")[0].strip()):
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

    def build_chunks_from_record_groups(self, record_groups):
        # Strategy code decides record grouping. This method materializes each
        # group into the output chunk representation. For PDF-like inputs, the
        # chunk box is defined by merged source positions and the text payload
        # is normalized by removing parser tags.
        if self.from_upstream.output_format in ["markdown", "text", "html"]:
            chunks = [{"text": "".join(record["text"] + "\n" for record in records)} for records in record_groups if records]
        else:
            chunks = [
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

        if self.param.root_chunk_as_heading and len(chunks) > 1:
            root_chunk = chunks[0]
            root_text = root_chunk.get("text", "")

            for ck in chunks[1:]:
                ck["text"] = root_text + "\n" + ck.get("text", "")

            return chunks[1:]

        return chunks

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
