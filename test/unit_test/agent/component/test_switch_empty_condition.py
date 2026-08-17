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
#

"""Unit tests for ``Switch`` condition evaluation.

Regression: an "and" condition whose ``items`` are all skipped (empty
``cpn_id``) leaves the per-condition result list empty, and ``all([]) is True``,
so the Switch matched that condition unconditionally and routed to it before
ever reaching the else/end branch.
"""

from types import SimpleNamespace

import pytest

from agent.component.switch import Switch


pytestmark = [pytest.mark.p1]


class _FakeCanvas:
    def __init__(self, refs=None):
        self._refs = refs or {}

    def get_variable_value(self, token):
        return self._refs.get(token)

    def get_component_name(self, cpn_id):
        return cpn_id


def _make_switch(conditions, end_cpn_ids, refs=None):
    sw = Switch.__new__(Switch)
    sw._param = SimpleNamespace(conditions=conditions, end_cpn_ids=end_cpn_ids)
    sw._canvas = _FakeCanvas(refs)
    sw.check_if_canceled = lambda _msg: False
    sw.set_input_value = lambda *_args, **_kwargs: None
    outputs = {}
    sw.set_output = lambda key, value: outputs.__setitem__(key, value)
    sw._invoke()
    return outputs


def test_empty_condition_falls_through_to_end():
    # The only item is skipped (blank cpn_id) -> result list is empty.
    conditions = [
        {
            "logical_operator": "and",
            "items": [{"cpn_id": "", "operator": "contains", "value": "x"}],
            "to": ["TARGET"],
        }
    ]
    outputs = _make_switch(conditions, end_cpn_ids=["END"])
    assert outputs["_next"] == ["END"]


def test_empty_and_items_fall_through_to_end():
    conditions = [
        {
            "logical_operator": "and",
            "items": [],
            "to": ["TARGET"],
        }
    ]
    outputs = _make_switch(conditions, end_cpn_ids=["END"])
    assert outputs["_next"] == ["END"]


def test_satisfied_and_condition_still_routes():
    # A genuinely satisfied "and" condition must still route to its target
    # (the fix must not break the real all(res) path).
    conditions = [
        {
            "logical_operator": "and",
            "items": [{"cpn_id": "c@out", "operator": "contains", "value": "hello"}],
            "to": ["TARGET"],
        }
    ]
    outputs = _make_switch(conditions, end_cpn_ids=["END"], refs={"c@out": "hello world"})
    assert outputs["_next"] == ["TARGET"]
