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

from common.chunk_order import (
    DERIVED_CHUNK_ORDER_BASE,
    PAGE_ORDER_STRIDE,
    assign_chunk_order,
    assign_derived_chunk_order,
)


class TestAssignChunkOrder:
    def test_numbers_chunks_in_reading_order(self):
        chunks = [{"id": "a"}, {"id": "b"}, {"id": "c"}]

        assert assign_chunk_order(chunks) is chunks
        assert [c["chunk_order_int"] for c in chunks] == [0, 1, 2]

    def test_page_range_tasks_of_one_document_do_not_collide(self):
        first = assign_chunk_order([{"id": "a"}, {"id": "b"}], from_page=0)
        second = assign_chunk_order([{"id": "c"}], from_page=12)

        assert [c["chunk_order_int"] for c in first] == [0, 1]
        assert second[0]["chunk_order_int"] == 12 * PAGE_ORDER_STRIDE
        assert second[0]["chunk_order_int"] > max(c["chunk_order_int"] for c in first)

    def test_negative_page_is_clamped_and_non_dict_rows_are_skipped(self):
        chunks = [{"id": "a"}, None]

        assign_chunk_order(chunks, from_page=-5)

        assert chunks[0]["chunk_order_int"] == 0

    def test_largest_spreadsheet_row_offset_stays_inside_int32(self):
        # The spreadsheet parser hands its first row in as ``from_page`` and
        # Excel caps a sheet at 1,048,576 rows; the column is an int32 on both
        # Elasticsearch and Infinity.
        chunks = assign_chunk_order([{"id": "a"}], from_page=1_048_576)

        assert chunks[0]["chunk_order_int"] < 2**31 - 1


class TestAssignDerivedChunkOrder:
    def test_derived_rows_sort_after_the_document_chunks(self):
        sources = assign_chunk_order([{"id": "s1"}, {"id": "s2"}], from_page=2048)
        mothers = assign_derived_chunk_order([{"id": "p1"}, {"id": "p2"}])

        assert [m["chunk_order_int"] for m in mothers] == [DERIVED_CHUNK_ORDER_BASE, DERIVED_CHUNK_ORDER_BASE + 1]
        assert min(m["chunk_order_int"] for m in mothers) > max(s["chunk_order_int"] for s in sources)

    def test_derived_values_fit_the_int32_chunk_column(self):
        assert 0 < DERIVED_CHUNK_ORDER_BASE < 2**31 - 1
