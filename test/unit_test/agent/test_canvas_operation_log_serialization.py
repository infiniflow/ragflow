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

from __future__ import annotations

import json

import pytest

from test.unit_test.agent.test_canvas_at_split import _load_canvas_module


class _ParamStub:
    def as_dict(self):
        return {
            "outputs": {
                "output_format": {"type": "str", "value": "chunks"},
                "_elapsed_time": {"type": "float", "value": 1.25},
                "chunks": {
                    "type": "Array",
                    "value": [
                        {
                            "text": "chunk text",
                            "q_1024_vec": [0.1, 0.2],
                            "metadata": {
                                "source": "unit-test",
                                "q_3_vec": [0.3, 0.4, 0.5],
                            },
                        }
                    ],
                },
            }
        }


class _ComponentStub:
    component_name = "Tokenizer"

    def __init__(self):
        self._param = _ParamStub()

    def __str__(self):
        return json.dumps(
            {
                "component_name": self.component_name,
                "params": self._param.as_dict(),
            }
        )


def _make_graph(canvas_mod):
    graph = canvas_mod.Graph.__new__(canvas_mod.Graph)
    graph.dsl = {
        "components": {},
        "graph": {"nodes": [{"id": "Tokenizer", "data": {"name": "Tokenizer"}}]},
        "globals": {},
        "path": ["Tokenizer"],
    }
    graph.components = {
        "Tokenizer": {
            "obj": _ComponentStub(),
            "downstream": [],
            "upstream": [],
        }
    }
    graph.path = ["Tokenizer"]
    graph.task_id = "task-1"
    return graph


@pytest.mark.p2
def test_operation_log_serialization_strips_dense_vectors(monkeypatch):
    canvas_mod = _load_canvas_module(monkeypatch)
    graph = _make_graph(canvas_mod)

    payload = json.loads(graph.to_operation_log_json())
    chunk = payload["components"]["Tokenizer"]["obj"]["params"]["outputs"]["chunks"]["value"][0]

    assert "q_1024_vec" not in chunk
    assert "q_3_vec" not in chunk["metadata"]
    assert chunk["text"] == "chunk text"
    assert chunk["metadata"]["source"] == "unit-test"
    assert payload["components"]["Tokenizer"]["obj"]["params"]["outputs"]["output_format"]["value"] == "chunks"
    assert payload["components"]["Tokenizer"]["obj"]["params"]["outputs"]["_elapsed_time"]["value"] == 1.25


@pytest.mark.p2
def test_string_serialization_keeps_runtime_outputs(monkeypatch):
    canvas_mod = _load_canvas_module(monkeypatch)
    graph = _make_graph(canvas_mod)

    payload = json.loads(str(graph))
    chunk = payload["components"]["Tokenizer"]["obj"]["params"]["outputs"]["chunks"]["value"][0]

    assert chunk["q_1024_vec"] == [0.1, 0.2]
    assert chunk["metadata"]["q_3_vec"] == [0.3, 0.4, 0.5]
