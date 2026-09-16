# Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Tag-aware overlap-prefix behaviour for the TokenChunker JSON path.

Mirrors Go's computeOverlapPrefix: the overlap cut is measured on the
tag-free *visible* text, never on the raw text that still carries the
``@@...##`` coordinate tags. This prevents two bugs:
  * the overlap prefix diverging from Go on tag-bearing inputs (parity), and
  * a partial ``@@...##`` fragment leaking into the next chunk when the cut
    lands inside a tag.
"""

from rag.flow.chunker.token_chunker import _merge_text_chunks_by_token_size


def test_overlap_prefix_cut_on_visible_text_not_raw():
    # prev chunk carries a coordinate tag; visible text is "ABCD".
    prev_text = "ABCD@@1\t2\t3\t4##"
    chunks = [
        {"ck_type": "text", "text": prev_text, "tk_nums": 10},
        {"ck_type": "text", "text": "EFGH", "tk_nums": 4},
    ]
    # chunk_token_size=5 -> threshold=4; prev tk_nums=10 > 4 so the second
    # chunk starts a new group and prepends a 20% overlap prefix.
    merged = _merge_text_chunks_by_token_size(chunks, chunk_token_size=5, overlapped_percent=20)

    assert len(merged) == 2
    new_text = merged[1]["text"]
    # Go behaviour: visible="ABCD", cut = int(4 * 0.8) = 3 -> prefix "D".
    # The new chunk must be exactly "D" + "EFGH".
    assert new_text == "DEFGH", new_text


def test_overlap_prefix_never_leaks_partial_tag():
    # A longer tag so the raw-text cut falls *inside* the tag (the buggy path).
    prev_text = "ABCD@@100\t200\t300\t400##"
    chunks = [
        {"ck_type": "text", "text": prev_text, "tk_nums": 10},
        {"ck_type": "text", "text": "WXYZ", "tk_nums": 4},
    ]
    merged = _merge_text_chunks_by_token_size(chunks, chunk_token_size=5, overlapped_percent=20)

    assert len(merged) == 2
    new_text = merged[1]["text"]
    # No '@@' may appear anywhere in the produced chunk text: the prefix is
    # cut from tag-free visible text, so a dangling tag fragment cannot leak.
    assert "@@" not in new_text, new_text
    # And the prefix must be the visible-cut prefix ("D"), not raw-cut debris.
    assert new_text == "DWXYZ", new_text
