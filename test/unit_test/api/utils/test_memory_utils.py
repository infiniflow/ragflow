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
import pytest

from api.utils.memory_utils import memory_type_names, order_facet_options, order_owner_options


def _option_ids(options):
    return [option["id"] for option in options]


@pytest.mark.p1
def test_memory_type_names_follow_canonical_bit_flag_order():
    assert memory_type_names() == ["raw", "semantic", "episodic", "procedural"]


@pytest.mark.p1
def test_facet_options_keep_canonical_order_regardless_of_aggregation_order():
    counts = {
        "procedural": {"id": "procedural", "count": 5},
        "raw": {"id": "raw", "count": 13},
        "semantic": {"id": "semantic", "count": 7},
        "episodic": {"id": "episodic", "count": 7},
    }

    assert _option_ids(order_facet_options(counts, memory_type_names())) == ["raw", "semantic", "episodic", "procedural"]


@pytest.mark.p1
def test_facet_options_append_unknown_values_after_canonical_ones():
    counts = {
        "vector": {"id": "vector", "count": 2},
        "graph": {"id": "graph", "count": 1},
        "table": {"id": "table", "count": 3},
    }

    assert _option_ids(order_facet_options(counts, ("table", "graph"))) == ["table", "graph", "vector"]


@pytest.mark.p1
def test_owner_options_sort_by_label_case_insensitively_then_id():
    counts = {
        "tenant-2": {"id": "tenant-2", "label": "Zeta", "count": 1},
        "tenant-1": {"id": "tenant-1", "label": "owner", "count": 3},
        "tenant-3": {"id": "tenant-3", "label": "Owner", "count": 2},
    }

    assert _option_ids(order_owner_options(counts)) == ["tenant-1", "tenant-3", "tenant-2"]
