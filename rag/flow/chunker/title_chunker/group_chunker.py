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

from common.token_utils import num_tokens_from_string
from rag.flow.chunker.title_chunker.common import (
    BaseTitleChunker,
    resolve_target_level,
)

MIN_GROUP_TOKENS = 32


def _build_section_ids(levels, target_level):
    """Assign a stable section id that increments whenever a title at or above
    ``target_level`` starts a new section."""
    sec_ids = []
    sid = 0
    for i, level in enumerate(levels):
        if target_level is not None and level <= target_level and i > 0:
            sid += 1
        sec_ids.append(sid)
    return sec_ids


def _resolve_group_target_level(levels, hierarchy, most_level):
    """Pick the level used as the grouping target for the group method."""
    if hierarchy and int(hierarchy) > 0:
        return resolve_target_level(levels, hierarchy)
    return most_level


class GroupTitleChunker(BaseTitleChunker):
    """Group consecutive records under the same title into one chunk."""

    start_message = "Start to group by title levels."

    def resolve_levels(self, line_records):
        """Resolve title levels via the shared outline/frequency strategy."""
        return self.resolve_title_levels(line_records)

    def build_chunks(self, line_records, resolved):
        """Build chunks by merging records inside the same logical section.

        The merge ceiling uses the configurable ``chunk_token_cap`` (0/None
        means "no ceiling"). The post-build ``_enforce_token_cap`` in
        BaseTitleChunker is the single hard guarantee, so any residual over-cap
        chunk (e.g. a single record bigger than the cap) is still re-split
        there.
        """
        target_level = _resolve_group_target_level(
            resolved["levels"],
            self.param.hierarchy,
            resolved["most_level"],
        )
        sec_ids = _build_section_ids(resolved["levels"], target_level)
        record_groups = []
        tk_cnt = 0
        last_sid = -2

        cap = self.param.chunk_token_cap or 0

        # The merge state is driven by (current section id, current token size).
        # A chunk stays open while records remain in the same logical section,
        # except that very small chunks are allowed to absorb the next record
        # regardless of section change.
        for record, sec_id in zip(line_records, sec_ids):
            if record["doc_type_kwd"] != "text":
                record_groups.append([record])
                tk_cnt = 0
                last_sid = -2
                continue

            text = record["text"]
            if not text.strip():
                continue

            token_count = num_tokens_from_string(text)
            merge_ceiling_ok = cap <= 0 or tk_cnt < cap
            should_merge = record_groups and record_groups[-1][0]["doc_type_kwd"] == "text" and (tk_cnt < MIN_GROUP_TOKENS or (merge_ceiling_ok and sec_id == last_sid))

            if should_merge:
                record_groups[-1].append(record)
                tk_cnt += token_count
            else:
                record_groups.append([record])
                tk_cnt = token_count

            last_sid = sec_id

        return self.build_chunks_from_record_groups(record_groups)
