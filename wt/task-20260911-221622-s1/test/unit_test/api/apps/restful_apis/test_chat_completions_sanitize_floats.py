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
"""Regression tests for `_sanitize_json_floats` in
`api/apps/restful_apis/chat_api.py` (fixes #15245).

The function strips NaN / ±Infinity from chat completion payloads
before serialization so the response is RFC 8259 JSON. Without it,
Go-style downstream consumers reject the stream with
`failed to encode response: json: unsupported value: NaN`.

The function has no module-level dependencies beyond stdlib `math`,
so the test extracts the function definition from `chat_api.py` via
the `ast` module and executes it in an isolated namespace. That
avoids stubbing the dozens of heavy imports `chat_api.py` pulls in
at module load (quart, peewee models, services, etc.) and keeps the
test fast and focused.
"""

from __future__ import annotations

import ast
import math
from pathlib import Path

import pytest


def _load_sanitize_function():
    """Extract `_sanitize_json_floats` from chat_api.py without importing it.

    Walks the source AST, finds the function definition, compiles only
    that node into a tiny module, and exec's it in a namespace that
    only exposes `math`. Returns the function object ready to call.
    """
    repo_root = Path(__file__).resolve().parents[5]
    source = (repo_root / "api" / "apps" / "restful_apis" / "chat_api.py").read_text()
    tree = ast.parse(source)
    fn_node = None
    for node in tree.body:
        if isinstance(node, ast.FunctionDef) and node.name == "_sanitize_json_floats":
            fn_node = node
            break
    assert fn_node is not None, "_sanitize_json_floats not found in chat_api.py"
    extracted = ast.Module(body=[fn_node], type_ignores=[])
    ns = {"math": math}
    exec(compile(extracted, str(repo_root / "api/apps/restful_apis/chat_api.py"), "exec"), ns)
    return ns["_sanitize_json_floats"]


_sanitize = _load_sanitize_function()


@pytest.mark.p1
class TestSanitizeJsonFloats:
    """Regression for #15245: NaN/Inf in retrieval scores must not leak
    into the JSON response."""

    @pytest.mark.p1
    def test_passthrough_for_healthy_values(self):
        """Plain numbers, strings, bools, None, lists, and dicts survive
        untouched."""
        assert _sanitize(0) == 0
        assert _sanitize(1.5) == 1.5
        assert _sanitize(-3.25) == -3.25
        assert _sanitize("hello") == "hello"
        assert _sanitize(None) is None
        assert _sanitize(True) is True
        assert _sanitize(False) is False
        assert _sanitize([1, 2, 3]) == [1, 2, 3]
        assert _sanitize({"a": 1, "b": "x"}) == {"a": 1, "b": "x"}

    @pytest.mark.p1
    def test_nan_becomes_none(self):
        """NaN at the top level is replaced with None — the only RFC
        8259-compatible representation."""
        assert _sanitize(float("nan")) is None

    @pytest.mark.p1
    @pytest.mark.parametrize("value", [float("inf"), float("-inf")])
    def test_positive_and_negative_infinity_become_none(self, value):
        assert _sanitize(value) is None

    @pytest.mark.p1
    def test_nan_inside_dict_replaced(self):
        """NaN appearing as a dict value is replaced; sibling keys are
        preserved verbatim."""
        out = _sanitize({"similarity": float("nan"), "doc_id": "abc", "score": 0.42})
        assert out == {"similarity": None, "doc_id": "abc", "score": 0.42}

    @pytest.mark.p1
    def test_inf_inside_list_replaced(self):
        out = _sanitize([1, float("inf"), 2, float("-inf"), 3])
        assert out == [1, None, 2, None, 3]

    @pytest.mark.p1
    def test_deeply_nested_chunks_format(self):
        """Mimics the actual `reference.chunks` shape — list of dicts of
        scores, with NaN possible per-chunk."""
        payload = {
            "answer": "...",
            "reference": {
                "chunks": [
                    {
                        "id": "c1",
                        "similarity": float("nan"),
                        "vector_similarity": 0.81,
                        "term_similarity": 0.62,
                    },
                    {
                        "id": "c2",
                        "similarity": 0.74,
                        "vector_similarity": float("inf"),
                        "term_similarity": 0.55,
                    },
                ]
            },
        }
        out = _sanitize(payload)
        assert out["answer"] == "..."
        chunks = out["reference"]["chunks"]
        assert chunks[0] == {
            "id": "c1",
            "similarity": None,
            "vector_similarity": 0.81,
            "term_similarity": 0.62,
        }
        assert chunks[1] == {
            "id": "c2",
            "similarity": 0.74,
            "vector_similarity": None,
            "term_similarity": 0.55,
        }

    @pytest.mark.p1
    def test_tuples_are_walked_and_returned_as_tuples(self):
        """Tuples are not idiomatic in JSON payloads but the function
        walks them defensively. The container type is preserved."""
        out = _sanitize((1, float("nan"), 3))
        assert out == (1, None, 3)
        assert isinstance(out, tuple)

    @pytest.mark.p1
    def test_numpy_float32_nan_caught_via_math_isnan(self):
        """numpy float scalar types (float32 / float16) do not subclass
        Python's `float` so a naive isinstance check would miss them.
        The probe-via-math.isnan approach used by the function should
        catch them anyway. Skip cleanly if numpy is unavailable so this
        test doesn't gate CI on the optional dep."""
        np = pytest.importorskip("numpy")

        # float64 is a subclass of float so the easy isinstance path
        # would have caught this; included for documentation.
        assert _sanitize(np.float64("nan")) is None

        # float32 / float16 are NOT subclasses of Python float; the
        # math.isnan probe is what saves us.
        assert _sanitize(np.float32("nan")) is None
        assert _sanitize(np.float16("nan")) is None

        # Healthy numpy scalars still pass through.
        assert _sanitize(np.float32(1.5)) == pytest.approx(1.5)

    @pytest.mark.p1
    def test_strings_are_not_treated_as_numeric(self):
        """`math.isnan` would TypeError on a string; the function must
        swallow that and treat the string as a regular non-numeric leaf."""
        assert _sanitize("NaN") == "NaN"
        assert _sanitize("Infinity") == "Infinity"
        assert _sanitize({"label": "NaN"}) == {"label": "NaN"}
