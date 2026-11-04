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

"""Reading-order numbering for chunks in the document engine.

The chunk list pages by ``chunk_order_int`` first and only then by the position
columns, which are numeric arrays on Elasticsearch but hex strings on Infinity.
Rows left at the column default 0 paginate by a string comparison and come back
scrambled, so ingestion numbers them.

Only rows written by this code are numbered. Documents ingested before it keep
the column default (0 on Infinity, absent on Elasticsearch) and still page in
the old order until they are re-ingested: the reading order of rows that carry
no position data cannot be recovered afterwards, because that order is exactly
what was missing.
"""

# Offset between page-range tasks of one document (stride = per-page chunk
# budget). Page parsers number pages and cap a range at MAXIMUM_PAGE_NUMBER
# (100,000), so 100,000 * 1024 stays inside the int32 column and far below
# DERIVED_CHUNK_ORDER_BASE. The spreadsheet parser passes a row offset instead,
# but table documents never carry derived rows, so they are outside this bound.
PAGE_ORDER_STRIDE = 1024

# Parent/abstract rows are listed after the document's source chunks. Only
# page-indexed documents produce them, so source orders above cannot reach it.
DERIVED_CHUNK_ORDER_BASE = 1 << 30

# TOC rows list after every parent row, so they own a band of their own; the
# 1 << 24 slots below it stay reserved for parent rows. One parsing task builds
# one TOC, so callers add the task's first page to this base to keep a document's
# TOCs in range order.
TOC_CHUNK_ORDER_BASE = DERIVED_CHUNK_ORDER_BASE + (1 << 24)

# Fields ``insert_chunks`` reads to derive a parent row from a source chunk.
PARENT_LINK_FIELDS = ("mom", "mom_with_weight", "mom_id")


def assign_chunk_order(chunks, from_page=0):
    """Number ``chunks`` in reading order, offset by the task's first page.

    ``from_page`` is a page index, or a row index for the spreadsheet parser.
    """
    base = max(int(from_page or 0), 0) * PAGE_ORDER_STRIDE
    for offset, chunk in enumerate(chunks or []):
        if isinstance(chunk, dict):
            chunk["chunk_order_int"] = base + offset
    return chunks


def assign_derived_chunk_order(chunks):
    """Number rows derived from source chunks after the document's chunks."""
    for offset, chunk in enumerate(chunks or []):
        if isinstance(chunk, dict):
            chunk["chunk_order_int"] = DERIVED_CHUNK_ORDER_BASE + offset
    return chunks


def drop_parent_link(chunk):
    """Remove the parent link a derived row inherited from its source chunk.

    A derived row is a copy of a source chunk, and ``insert_chunks`` turns every
    row carrying ``mom`` / ``mom_with_weight`` into a parent row whose id hashes
    that text. Keeping the link therefore writes a second parent that shares the
    original parent's id and re-stamps its order and page columns.
    """
    for field in PARENT_LINK_FIELDS:
        chunk.pop(field, None)
    return chunk
