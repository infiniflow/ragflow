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

from rag.flow.chunker.title_chunker.common import (
    BaseTitleChunker,
    resolve_target_level,
)


class _ChunkNode:
    def __init__(self, level, title_indexes=None, body_indexes=None):
        self.level = level
        self.title_indexes = title_indexes or []
        self.body_indexes = body_indexes or []
        self.children = []


    def add_child(self, child):
        self.children.append(child)


    def add_body_index(self, index):
        self.body_indexes.append(index)


    def build_tree(self, indexed_lines, depth):
        stack = [self]
        for level, index in indexed_lines:
            if level > depth:
                stack[-1].add_body_index(index)
                continue

            while len(stack) > 1 and level <= stack[-1].level:
                stack.pop()

            node = _ChunkNode(level, title_indexes=[index])
            stack[-1].add_child(node)
            stack.append(node)

        return self


    def get_paths(self, depth, include_heading_content):
        chunk_paths = []
        self._dfs(chunk_paths, [], depth, include_heading_content)
        return chunk_paths


    def _dfs(self, chunk_paths, titles, depth, include_heading_content):
        if self.level == 0 and self.body_indexes:
            chunk_paths.append(titles + self.body_indexes)

        if include_heading_content:
            path_titles = titles + self.title_indexes if 1 <= self.level <= depth else titles

            if self.body_indexes and 1 <= self.level <= depth:
                chunk_paths.append(path_titles + self.body_indexes)
            elif not self.children and 1 <= self.level <= depth:
                chunk_paths.append(path_titles)
        else:
            path_titles = (
                titles + self.title_indexes + self.body_indexes
                if 1 <= self.level <= depth
                else titles
            )

            if not self.children and 1 <= self.level <= depth:
                chunk_paths.append(path_titles)

        for child in self.children:
            child._dfs(chunk_paths, path_titles, depth, include_heading_content)


class HierarchyTitleChunker(BaseTitleChunker):
    start_message = "Start to merge hierarchically."

    def resolve_levels(self, line_records):
        return self.resolve_title_levels(line_records)


    def build_chunks(self, line_records, resolved):
        record_groups = []
        text_records = []
        text_levels = []

        def flush_text_records():
            if not text_records:
                return

            target_level = resolve_target_level(text_levels, self.param.hierarchy)
            if target_level is None:
                record_groups.append(text_records.copy())
            else:
                root = _ChunkNode(0)
                root.build_tree(list(zip(text_levels, range(len(text_records)))), target_level)
                record_groups.extend(
                    [text_records[index] for index in path]
                    for path in root.get_paths(
                        target_level,
                        self.param.include_heading_content,
                    )
                    if path
                )
            text_records.clear()
            text_levels.clear()

        for record, level in zip(line_records, resolved["levels"]):
            if record["doc_type_kwd"] == "text":
                text_records.append(record)
                text_levels.append(level)
                continue

            flush_text_records()
            record_groups.append([record])

        flush_text_records()
        return self.build_chunks_from_record_groups(record_groups)
