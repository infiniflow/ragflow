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
"""

# Offset between page-range tasks of one document (stride = per-page chunk
# budget). The spreadsheet parser passes a row offset, and Excel caps a sheet
# at 1,048,576 rows, so the value stays inside the int32 column range.
PAGE_ORDER_STRIDE = 1024

# Parent/abstract rows are listed after the document's source chunks.
DERIVED_CHUNK_ORDER_BASE = 1 << 30


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
