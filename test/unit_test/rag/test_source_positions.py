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

from rag.nlp import add_positions, add_source_positions


def test_add_source_positions_keeps_public_one_based_page_numbers():
    chunk = {}

    add_source_positions(chunk, [[1, 10, 20, 30, 40]])

    assert chunk["page_num_int"] == [1]
    assert chunk["position_int"] == [(1, 10, 20, 30, 40)]
    assert chunk["top_int"] == [30]


def test_add_source_positions_normalizes_legacy_zero_page_number():
    chunk = {}

    add_source_positions(chunk, [[0, 10, 20, 30, 40]])

    assert chunk["page_num_int"] == [1]
    assert chunk["position_int"] == [(1, 10, 20, 30, 40)]
    assert chunk["top_int"] == [30]


def test_add_positions_still_offsets_parser_zero_based_page_numbers():
    chunk = {}

    add_positions(chunk, [[1, 10, 20, 30, 40]])

    assert chunk["page_num_int"] == [2]
    assert chunk["position_int"] == [(2, 10, 20, 30, 40)]
    assert chunk["top_int"] == [30]
