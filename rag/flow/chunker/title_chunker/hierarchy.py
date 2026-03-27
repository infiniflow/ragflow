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

import asyncio
import logging
import random
import re
from functools import partial

from common.misc_utils import get_uuid
from rag.utils.base64_image import id2image, image2id
from deepdoc.parser.pdf_parser import RAGFlowPdfParser
from rag.flow.base import ProcessParamBase
from rag.flow.chunker.title_chunker.schema import TitleChunkerFromUpstream
from rag.nlp import concat_img
from common import settings


class TitleChunkerParam(ProcessParamBase):
    def __init__(self):
        super().__init__()
        self.levels = []
        self.hierarchy = None

    def check(self):
        self.check_empty(self.levels, "Hierarchical setups.")
        self.check_empty(self.hierarchy, "Hierarchy number.")

    def get_input_form(self) -> dict[str, dict]:
        return {}


# Regex-group selection.
# Frontend sends candidate regex groups in group-first format:
# [
#   [group1_h1, group1_h2, group1_h3],
#   [group2_h1, group2_h2, group2_h3],
# ]
# Internally each level keeps a list of regexes so matching logic can stay uniform.
def _build_level_groups(raw_levels):
    return [[[expression] for expression in group] for group in raw_levels]


def _match_level(text, level_group):
    """Return the 0-based title level matched by one line, or None for body text."""
    stripped_text = text.strip()
    for level, regexes in enumerate(level_group):
        for regex in regexes:
            if re.match(regex, stripped_text):
                return level
    return None


def _score_level_group(lines, level_group):
    """Prefer groups that match more lines, cover more levels, and stay shallow."""
    matched_levels = []
    for txt in lines:
        lvl = _match_level(txt, level_group)
        if lvl is not None:
            matched_levels.append(lvl)

    if not matched_levels:
        return 0, 0, 0, 0

    return (
        len(matched_levels),
        len(set(matched_levels)),
        sum(1 for lvl in matched_levels if lvl == 0),
        -sum(matched_levels),
    )


def _select_level_group(lines, raw_levels):
    """Pick the candidate regex group that best explains the document."""
    groups = _build_level_groups(raw_levels)
    return max(groups, key=lambda group: _score_level_group(lines, group))


def _resolve_target_level(indexed_lines, body_level, hierarchy):
    """Map hierarchy=N to the Nth title level that actually appears in this document."""
    title_levels = sorted({level for level, _ in indexed_lines if level < body_level})
    if not title_levels:
        return None

    hierarchy_num = max(int(hierarchy), 1)
    return title_levels[min(hierarchy_num, len(title_levels)) - 1]


# Index-based equivalent of rag.nlp.Node used by laws.tree_merge.
# Titles up to the resolved target level form the tree. Deeper titles and body
# lines are merged into the nearest target-level leaf.
class _ChunkNode:
    def __init__(self, level, indexes):
        self.level = level
        self.indexes = indexes
        self.children = []

    def add_child(self, child):
        self.children.append(child)

    def add_index(self, index):
        self.indexes.append(index)

    def build_tree(self, indexed_lines, depth):
        stack = [self]
        for level, index in indexed_lines:
            if level > depth:
                stack[-1].add_index(index)
                continue

            while len(stack) > 1 and level <= stack[-1].level:
                stack.pop()

            node = _ChunkNode(level, [index])
            stack[-1].add_child(node)
            stack.append(node)

        return self

    def get_paths(self, depth):
        chunk_paths = []
        self._dfs(chunk_paths, [], depth)
        return chunk_paths

    def _dfs(self, chunk_paths, titles, depth):
        if self.level == 0 and self.indexes:
            chunk_paths.append(titles + self.indexes)

        if 1 <= self.level <= depth:
            path_titles = titles + self.indexes
        else:
            path_titles = titles

        if self.level > depth and self.indexes:
            chunk_paths.append(path_titles + self.indexes)
        elif not self.children and 1 <= self.level <= depth:
            chunk_paths.append(path_titles)

        for child in self.children:
            child._dfs(chunk_paths, path_titles, depth)


def _extract_lines(from_upstream):
    """Normalize upstream content into plain text lines and optional image ids."""
    if from_upstream.output_format == "markdown":
        payload = from_upstream.markdown_result or ""
        return [line for line in payload.split("\n") if line], []

    if from_upstream.output_format == "text":
        payload = from_upstream.text_result or ""
        return [line for line in payload.split("\n") if line], []

    if from_upstream.output_format == "html":
        payload = from_upstream.html_result or ""
        return [line for line in payload.split("\n") if line], []

    items = from_upstream.chunks if from_upstream.output_format == "chunks" else from_upstream.json_result
    lines = []
    image_ids = []
    for item in items or []:
        raw_text = item.get("text") if isinstance(item, dict) else item
        lines.append(raw_text if isinstance(raw_text, str) else str(raw_text or ""))
        image_ids.append(item.get("img_id") if isinstance(item, dict) else None)
    return lines, image_ids


def _build_indexed_lines(lines, level_group):
    """Convert lines into tree-merge levels using the selected regex group."""
    body_level = len(level_group) + 1
    indexed_lines = []
    for index, text in enumerate(lines):
        matched_level = _match_level(text, level_group)
        indexed_lines.append((body_level if matched_level is None else matched_level + 1, index))
    return indexed_lines, body_level


def _build_chunk_paths(lines, level_group, hierarchy):
    """
    Resolve the effective hierarchy and build chunk paths with laws.tree_merge
    semantics.
    """
    indexed_lines, body_level = _build_indexed_lines(lines, level_group)
    target_level = _resolve_target_level(indexed_lines, body_level, hierarchy)

    if target_level is None:
        return [list(range(len(lines)))] if lines else []

    root = _ChunkNode(0, [])
    root.build_tree(indexed_lines, target_level)
    return root.get_paths(target_level)


def _build_text_chunks(chunk_paths, lines):
    """Serialize chunk paths for markdown/text/html output."""
    return [{"text": "".join(lines[index] + "\n" for index in path)} for path in chunk_paths if path]


def _build_visual_chunks(chunk_paths, lines, image_ids, title_chunker):
    """Serialize chunk paths for chunk/json output with images and positions."""
    cks = []
    images = []
    for path in chunk_paths:
        txt = ""
        img = None
        for index in path:
            txt += lines[index] + "\n"
            concat_img(
                img,
                id2image(
                    image_ids[index],
                    partial(
                        settings.STORAGE_IMPL.get,
                        tenant_id=title_chunker._canvas._tenant_id,
                    ),
                ),
            )
        cks.append(txt)
        images.append(img)

    cks = [
        {
            "text": RAGFlowPdfParser.remove_tag(content),
            "image": image,
            "positions": RAGFlowPdfParser.extract_positions(content),
        }
        for content, image in zip(cks, images)
    ]
    tasks = []
    for chunk in cks:
        tasks.append(
            asyncio.create_task(
                image2id(
                    chunk,
                    partial(
                        settings.STORAGE_IMPL.put,
                        tenant_id=title_chunker._canvas._tenant_id,
                    ),
                    get_uuid(),
                )
            )
        )
    return cks, tasks


async def invoke_hierarchy_title_chunker(title_chunker, **kwargs):
    try:
        from_upstream = TitleChunkerFromUpstream.model_validate(kwargs)
    except Exception as e:
        title_chunker.set_output("_ERROR", f"Input error: {str(e)}")
        return

    title_chunker.set_output("output_format", "chunks")
    title_chunker.callback(random.randint(1, 5) / 100.0, "Start to merge hierarchically.")
    lines, image_ids = _extract_lines(from_upstream)

    level_group = _select_level_group(lines, title_chunker._param.levels)
    chunk_paths = _build_chunk_paths(lines, level_group, title_chunker._param.hierarchy)

    if from_upstream.output_format in ["markdown", "text", "html"]:
        title_chunker.set_output("chunks", _build_text_chunks(chunk_paths, lines))
    else:
        cks, tasks = _build_visual_chunks(
            chunk_paths,
            lines,
            image_ids,
            title_chunker,
        )
        try:
            await asyncio.gather(*tasks, return_exceptions=False)
        except Exception as e:
            logging.error(f"Error in image2id: {e}")
            for t in tasks:
                t.cancel()
            await asyncio.gather(*tasks, return_exceptions=True)
            raise

        title_chunker.set_output("chunks", cks)

    title_chunker.callback(1, "Done.")
