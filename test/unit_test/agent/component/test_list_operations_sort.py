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
"""Regression tests for ListOperations mixed-type sort (issue #19427).

The Python ListOperations ``sort`` op used ``sorted(items, key=...)`` with
plain Python comparison, which raises ``TypeError`` whenever the key
sequence contains ``None`` alongside a number, or mixed ``int``/``str``
values. The Go runtime port (``internal/agent/component/list_operations.go``
``lessScalar``) handles both cases — numbers compare numerically, everything
else via ``fmt.Sprintf("%v", v)``. The Python side now mirrors that
contract via a ``cmp_to_key`` adapter.

These tests pin both the helpers (``_scalar_compare``, ``_scalar_sort_key``)
and the ``_sort`` invocation path.
"""

import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


def _stub(monkeypatch, name, **attrs):
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    if "." in name:
        parent_name, _, child_name = name.rpartition(".")
        parent_mod = sys.modules.get(parent_name)
        if parent_mod is not None and not hasattr(parent_mod, child_name):
            monkeypatch.setattr(parent_mod, child_name, mod, raising=False)
    return mod


@pytest.fixture
def lo_module(monkeypatch):
    """Load ``agent.component.list_operations`` with heavy deps stubbed so
    the test runs in slim envs without grpcio-status.

    Stubbed:
    - ``common.settings`` → noop (stops the ``rag.utils.gcs_conn`` →
      ``google.cloud.storage`` chain that emits an ``ImportWarning`` at
      collection time, escalated to an error by pytest's filterwarnings).
    - ``agent.component.base.ComponentBase`` / ``ComponentParamBase`` →
      empty base classes (the module imports them for class definition).
    - ``api.utils.api_utils.timeout`` → passthrough decorator (the module
      uses it on ``_invoke``).
    """
    _stub(monkeypatch, "common.settings", docStoreConn=SimpleNamespace(), retriever=SimpleNamespace(), kg_retriever=SimpleNamespace())
    _stub(monkeypatch, "agent.component.base", ComponentBase=type("ComponentBase", (object,), {}), ComponentParamBase=type("ComponentParamBase", (object,), {}))
    _stub(monkeypatch, "api.utils.api_utils", timeout=lambda *_a, **_k: lambda f: f)

    repo_root = Path(__file__).resolve().parents[4]
    module_path = repo_root / "agent" / "component" / "list_operations.py"
    spec = importlib.util.spec_from_file_location("_list_operations_test", module_path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _helpers(lo_module):
    """Pull the helper functions off the loaded module."""
    return lo_module._scalar_compare, lo_module._scalar_sort_key


def _classes(lo_module):
    """Return ``(ListOperations, ListOperationsParam)`` from the loaded module."""
    return lo_module.ListOperations, lo_module.ListOperationsParam


# --------------------------------------------------------------------------- #
# Helper-level tests
# --------------------------------------------------------------------------- #


class TestScalarCompare:
    """Pin the ``_scalar_compare`` semantics (mirror of Go's ``lessScalar``)."""

    @pytest.mark.parametrize(
        "a,b,sign",
        [
            (1, 2, -1),
            (2, 1, 1),
            (1, 1, 0),
            (-1, 1, -1),
            (0, 0.0, 0),  # int 0 == float 0.0 numerically
            (1.5, 2.5, -1),
            (3.14, 3.14, 0),
        ],
    )
    def test_numeric_pair_orders_numerically(self, lo_module, a, b, sign):
        """Two numeric values (int/float, no bool) compare numerically."""
        cmp = lo_module._scalar_compare
        assert (cmp(a, b) > 0) == (sign > 0)
        assert (cmp(a, b) < 0) == (sign < 0)
        assert (cmp(a, b) == 0) == (sign == 0)

    @pytest.mark.parametrize(
        "a,b",
        [
            # Text-rank pairs compare via the Go formatter lex order.
            ("abc", "abd"),
            ("abc", "ab"),
            (None, "A"),  # Go formatter: None → "<nil>", "A" → "A"; "<nil>" < "A"
            (None, "5"),  # "<nil>" < "5" ('< ' < '5')
            (False, True),  # Go formatter: "false" < "true"
            (True, False),  # reverse
            ("10", "5"),  # pure string lex
            ("5", "10"),
        ],
    )
    def test_non_numeric_pair_orders_as_strings(self, lo_module, a, b):
        """Both values land in the text rank (rank 1). The comparator
        orders them via the Go formatter (``fmt.Sprintf(\"%v\", v)``-equivalent):
        ``None`` → ``\"<nil>\"``, bools → ``\"true\"/\"false\"`` (lowercase),
        everything else via ``str()``."""
        cmp = lo_module._scalar_compare
        fmt = lo_module._go_format_scalar
        sa, sb = fmt(a), fmt(b)
        expected = (sa > sb) - (sa < sb)
        assert cmp(a, b) == expected, f"cmp({a!r}, {b!r}): expected {expected} (Go fmt {sa!r} vs {sb!r})"

    def test_bool_treated_as_string_not_number(self, lo_module):
        """``bool`` is a subclass of ``int`` in Python; the comparator must
        exclude it from the numeric branch (matches Go's ``toFloat64OK``)."""
        cmp = lo_module._scalar_compare
        # If bool were treated as int, True == 1 == 1 → comparator returns 0.
        # Treated as string, "True" > "1" → comparator returns 1.
        assert cmp(True, 1) == 1
        assert cmp(False, 0) == 1

    def test_none_orders_as_string(self, lo_module):
        """``None`` sorts via Go formatter (``"<nil>"``) lex order."""
        cmp = lo_module._scalar_compare
        assert cmp(None, None) == 0
        # Go's ``fmt.Sprintf(\"%v\", nil)`` → ``"<nil>"``; ``"<"`` (0x3C)
        # < ``"A"`` (0x41), so ``None`` sorts BEFORE ``"A"``.
        assert cmp(None, "A") == -1
        assert cmp("A", None) == 1

    def test_none_and_int_via_string_branch(self, lo_module):
        """``None`` is text-rank (Go format ``"<nil>"``) and ints are numeric-rank.
        Per the rank boundary, ``None`` (rank 1) is strictly greater than any
        int (rank 0), regardless of string contents."""
        cmp = lo_module._scalar_compare
        # 5 (rank 0) < None (rank 1) → a < b → -1
        assert cmp(5, None) == -1
        assert cmp(None, 5) == 1


class TestScalarSortKey:
    """Pin the ``sorted(..., key=_scalar_sort_key)`` adapter contract."""

    def test_handles_list_of_mixed_scalars(self, lo_module):
        """Sorting a list of mixed scalars does not raise and produces a
        deterministic order. With the transitive-rank comparator:

        - All numbers sort first (rank 0), in numeric order.
        - All non-numbers sort after (rank 1, Go-formatted lex order).

        ``None`` → ``\"<nil>\"`` (Go's ``fmt.Sprintf(\"%v\", nil)``).
        ``True`` → ``\"true\"``, ``False`` → ``\"false\"``.
        Lex order: ``'<'`` (0x3C) > ``'1'`` (0x31) > ``'5'`` (0x35) > ``'a'``
        (0x61) > ``'f'`` (0x66) > ``'t'`` (0x74), so:
        ``\"10\"`` (``'1'``) < ``\"5\"`` (``'5'``) < ``\"<nil>\"`` (``'<'``)
        < ``\"abc\"`` (``'a'``) < ``\"false\"`` (``'f'``) < ``\"true\"`` (``'t'``).
        """
        key = lo_module._scalar_sort_key
        items = [3, None, "abc", True, 5.5, 1, "5", False, "10"]
        result = sorted(items, key=key)
        # Numbers (rank 0): 1, 3, 5.5.
        # Non-numbers (rank 1, Go format lex): "10", "5", "<nil>", "abc", "false", "true".
        assert result == [1, 3, 5.5, "10", "5", None, "abc", False, True]

    def test_numeric_values_order_numerically_not_lex(self, lo_module):
        """Regression (CodeRabbit PR #19782): when numeric rank keys were
        formatted strings, a transitive rank-based sort still ordered
        ``10`` before ``2`` lexicographically. The numeric branch must
        carry the original value so ``[2, 3, 10, "11"]`` results."""
        key = lo_module._scalar_sort_key
        assert sorted([10, 2, 3, "11"], key=key) == [2, 3, 10, "11"]

    def test_sort_key_preserves_huge_int(self, lo_module):
        """Regression: ``float(10**400)`` raises ``OverflowError``. The
        sort key must keep the original integer so huge ints sort
        numerically instead of crashing."""
        key = lo_module._scalar_sort_key
        huge = 10**400
        assert sorted([huge, 2, 10], key=key) == [2, 10, huge]
        assert key(huge) == (0, huge)

    def test_sort_key_distinguishes_ints_beyond_float_precision(self, lo_module):
        """Regression: ``float(2**53) == float(2**53 + 1)`` — a
        float-based key collapses distinct integers around ``2**53`` and
        can misorder them. Original values compare exactly."""
        key = lo_module._scalar_sort_key
        base = 2**53
        assert float(base) == float(base + 1)  # the collapse being guarded
        assert sorted([base + 1, base], key=key) == [base, base + 1]
        assert sorted([base, base + 1, base - 1], key=key) == [base - 1, base, base + 1]

    def test_sort_key_mixed_int_and_float_exact(self, lo_module):
        """int/float pairs in the numeric rank compare exactly (no float
        round-trip), so a huge int still orders above any finite float."""
        key = lo_module._scalar_sort_key
        huge = 10**400
        assert sorted([huge, 1.5, 2], key=key) == [1.5, 2, huge]

    def test_reverse_sort(self, lo_module):
        key = lo_module._scalar_sort_key
        items = [3, None, "abc", True, 5.5, 1, "5", False, "10"]
        result = sorted(items, key=key, reverse=True)
        assert result == sorted(items, key=key)[::-1]

    def test_transitive_no_cycle_10_2_11(self, lo_module):
        """Regression: the previous comparator produced a non-transitive
        cycle (``10 > 2``, ``2 > \"11\"``, ``\"11\" > 10``) — a ``sorted``
        run could yield different orders depending on the starting input
        order. With explicit type-rank ordering, numbers are always less
        than strings, so the cycle is impossible."""
        key = lo_module._scalar_sort_key
        # Two runs of ``sorted`` with different input orders must produce
        # the same output.
        a = sorted([10, 2, "11", 3], key=key)
        b = sorted(["11", 10, 3, 2], key=key)
        assert a == b
        # Rank boundary holds: every number is in the first group, every
        # non-number in the second. Compare ranks via the key tuples
        # (avoids ``TypeError`` from ``max(numeric) < min(text)`` when the
        # list contains mixed types).
        ranks = [key(x)[0] for x in a]
        first_text_idx = ranks.index(1) if 1 in ranks else len(a)
        numeric_count = first_text_idx
        assert all(r == 0 for r in ranks[:numeric_count])
        assert all(r == 1 for r in ranks[numeric_count:])
        # And the comparator itself never returns 1 for ``cmp(int, str)``
        # (would re-introduce the cycle).
        cmp = lo_module._scalar_compare
        assert cmp(10, "11") == -1
        assert cmp(2, "11") == -1

    def test_numeric_rank_boundary_holds(self, lo_module):
        """Pins the rank ordering: every number is strictly less than every
        non-number. Without this, the comparator could cycle (e.g.
        ``2 > \"11\"`` and ``\"11\" > 10`` would imply ``2 > 10`` and
        ``10 > 2``, violating transitivity)."""
        cmp = lo_module._scalar_compare
        # Numbers come before non-numbers (rank boundary):
        # int (rank 0) < str (rank 1).
        assert cmp(2, "11") == -1
        assert cmp("11", 10) == 1  # NOT -1 — str is rank 1, int is rank 0
        # The previous comparator produced cmp("11", 10) == -1 (lex), which
        # combined with cmp(2, "11") == -1 and cmp(2, 10) == -1 yielded a
        # cycle. With explicit rank, no such cycle is possible.

    def test_go_format_none(self, lo_module):
        """Go's ``fmt.Sprintf(\"%v\", nil)`` → ``\"<nil>\"``. Python's
        ``str(None)`` → ``\"None\"``. The Go formatter keeps Python and Go
        orderings aligned on ``None``."""
        fmt = lo_module._go_format_scalar
        assert fmt(None) == "<nil>"

    def test_go_format_bool(self, lo_module):
        """Go's ``%v`` formats bools as ``\"true\"/\"false\"`` (lowercase);
        Python's ``str()`` returns ``\"True\"/\"False\"`` (capitalized).
        The Go formatter keeps the orderings aligned so ``False < True``
        on both runtimes."""
        fmt = lo_module._go_format_scalar
        assert fmt(True) == "true"
        assert fmt(False) == "false"

    def test_go_format_int(self, lo_module):
        fmt = lo_module._go_format_scalar
        assert fmt(42) == "42"
        assert fmt(-3) == "-3"

    def test_go_format_float(self, lo_module):
        fmt = lo_module._go_format_scalar
        # Python's ``str(3.14)`` → ``\"3.14\"``. Go's ``%v`` on a float64
        # also prints minimal precision, so they match for most values.
        assert fmt(3.14) == "3.14"
        assert fmt(1.0) == "1"
        assert fmt(-0.0) == "-0"
        assert fmt(float("inf")) == "+Inf"
        assert fmt(float("-inf")) == "-Inf"
        assert fmt(float("nan")) == "NaN"

    def test_nan_has_deterministic_numeric_position(self, lo_module):
        key = lo_module._scalar_sort_key
        nan = float("nan")
        for values in ([nan, 2.0, 1.0], [2.0, nan, 1.0], [1.0, 2.0, nan]):
            result = sorted(values, key=key)
            assert result[:2] == [1.0, 2.0]
            assert math.isnan(result[2])

    def test_go_format_integral_floats_in_containers(self, lo_module):
        fmt = lo_module._go_format_scalar
        assert fmt([1.0, {"value": 2.0}]) == "[1 map[value:2]]"
        assert fmt({"one": 1.0, "nested": [3.14, 2.0]}) == "map[nested:[3.14 2] one:1]"

    def test_go_format_list_recursive(self, lo_module):
        """Go's ``%v`` renders a list as ``[a b c]``: space-separated,
        string elements unquoted, bools lowercase, nested values
        recursive. Python's ``str()`` would produce ``"[1, 'a', True]"``.
        """
        fmt = lo_module._go_format_scalar
        assert fmt([1, "a", True, None]) == "[1 a true <nil>]"
        assert fmt([]) == "[]"
        assert fmt([[1, 2], "x"]) == "[[1 2] x]"

    def test_go_format_map_recursive(self, lo_module):
        """Go's ``%v`` renders a map as ``map[k:v ...]`` with entries in
        a deterministic (key-sorted) order; keys and values format
        recursively."""
        fmt = lo_module._go_format_scalar
        assert fmt({"b": 1, "a": True}) == "map[a:true b:1]"
        assert fmt({}) == "map[]"
        assert fmt({"k": [1, "x"]}) == "map[k:[1 x]]"

    def test_handles_dict_field_with_mixed_values(self, lo_module):
        """The dict path sorts by ``tuple(field_value for field in sort_by)``,
        wrapping each value with ``_scalar_sort_key`` so mixed-type fields
        sort instead of crashing."""
        key = lo_module._scalar_sort_key
        items = [
            {"name": "a", "priority": 5},
            {"name": "b", "priority": None},
            {"name": "c", "priority": "high"},
            {"name": "d", "priority": True},
            {"name": "e", "priority": 1},
        ]
        sort_by = ["priority"]
        result = sorted(
            items,
            key=lambda x: tuple(key(x.get(k)) for k in sort_by),
        )
        # Numbers (rank 0): e (1), a (5).
        # Non-numbers (rank 1, Go format lex):
        # b → "<nil>", c → "high", d → "true".
        # '<' (0x3C) < 'h' (0x68) < 't' (0x74), so order is "<nil>" < "high" < "true".
        assert [d["name"] for d in result] == ["e", "a", "b", "c", "d"]


# --------------------------------------------------------------------------- #
# _sort invocation tests (full path through ListOperations._sort)
# --------------------------------------------------------------------------- #


class _StubCanvas:
    def get_variable_value(self, _name):
        return None


@pytest.fixture
def list_ops(lo_module):
    ListOperations, ListOperationsParam = lo_module.ListOperations, lo_module.ListOperationsParam
    op = ListOperations.__new__(ListOperations)
    op._canvas = _StubCanvas()
    op._param = ListOperationsParam()
    op._param.operations = "sort"
    op._param.sort_method = "asc"
    return op


class TestListOperationsSortInv:
    """End-to-end through ``ListOperations._sort`` — the bug surfaced here."""

    def test_scalar_list_with_none_and_strings_does_not_raise(self, lo_module, list_ops):
        """The reproduction in #19427: a list with mixed ``None``/int/str
        values used to raise ``TypeError: '<' not supported between
        instances of 'NoneType' and 'int'``. After the fix the sort
        completes deterministically.

        With the transitive rank comparator: numbers (1, 3) sort first
        (rank 0); non-numbers sort after (rank 1, Go format lex).
        ``\"5\"`` (0x35) sorts BEFORE ``\"<nil>\"`` (0x3C), so the
        order is ``[1, 3, \"5\", None, \"abc\"]``."""
        list_ops.inputs = [3, None, "abc", 1, "5"]
        list_ops._sort()
        assert list_ops._param.outputs["result"]["value"] == [1, 3, "5", None, "abc"]

    def test_scalar_list_desc(self, lo_module, list_ops):
        list_ops._param.sort_method = "desc"
        list_ops.inputs = [3, None, "abc", 1, "5"]
        list_ops._sort()
        assert list_ops._param.outputs["result"]["value"] == ["abc", None, "5", 3, 1]

    def test_dict_list_with_none_field_does_not_raise(self, lo_module, list_ops):
        """sort_by='priority' on dicts whose ``priority`` field is ``None``
        used to raise ``TypeError``. After the fix the missing field is
        treated as ``None`` and sorted via the rank-based comparator.

        Numbers first (rank 0): a (5).
        Non-numbers (rank 1, Go format): ``None`` → ``\"<nil>\"``,
        ``True`` → ``\"true\"``, ``\"high\"``. Within text rank:
        ``\"<nil>\"`` < ``\"high\"`` < ``\"true\"`` (``\"h\"`` < ``\"t\"``)."""
        list_ops.inputs = [
            {"name": "a", "priority": 5},
            {"name": "b", "priority": None},
            {"name": "c", "priority": "high"},
            {"name": "d", "priority": True},
        ]
        list_ops._param.sort_by = "priority"
        list_ops._sort()
        # First item must be the numeric 'a' (rank 0).
        # Text rank Go-format lex: "<nil>" < "high" < "true".
        names = [d["name"] for d in list_ops._param.outputs["result"]["value"]]
        assert names == ["a", "b", "c", "d"]

    def test_dict_list_desc_with_none_field(self, lo_module, list_ops):
        list_ops._param.sort_method = "desc"
        list_ops.inputs = [
            {"name": "a", "priority": 5},
            {"name": "b", "priority": None},
            {"name": "c", "priority": "high"},
        ]
        list_ops._param.sort_by = "priority"
        list_ops._sort()
        # Numeric a last (rank 0); text rank reversed: "true", "high", "<nil>".
        result = list_ops._param.outputs["result"]["value"]
        assert [d["name"] for d in result] == ["c", "b", "a"]

    def test_mixed_int_and_str_fields_sort_by_rank(self, lo_module, list_ops):
        """With the rank-based comparator, mixed int/str fields no longer
        fall through to string comparison. All ints come first (rank 0),
        then all strings (rank 1, lex). The previous comparator would
        have sorted ``10`` between ``\"5\"`` and ``\"abc\"`` (lex: '1' <
        '5' < 'a'), but now ``10`` is just another number in the rank-0
        group."""
        list_ops.inputs = [
            {"name": "a", "k": 10},
            {"name": "b", "k": "5"},
            {"name": "c", "k": 1},
            {"name": "d", "k": "abc"},
        ]
        list_ops._param.sort_by = "k"
        list_ops._sort()
        # Rank 0 (numeric): c (1), a (10) — sorted numerically.
        # Rank 1 (text, lex): b ("5"), d ("abc").
        assert [d["name"] for d in list_ops._param.outputs["result"]["value"]] == [
            "c",  # k=1
            "a",  # k=10
            "b",  # k="5"
            "d",  # k="abc"
        ]

    def test_empty_inputs(self, lo_module, list_ops):
        """Edge case — sort with no items is a no-op that still sets the
        outputs (matches existing behavior)."""
        list_ops.inputs = []
        list_ops._sort()
        assert list_ops._param.outputs["result"]["value"] == []

    def test_inputs_is_none_treated_as_empty(self, lo_module, list_ops):
        """Edge case — the operator handles ``self.inputs is None`` by
        skipping the op entirely (set in ``_invoke``). Pin the contract."""
        list_ops.inputs = None
        # No-op when inputs is None (matches the early return in _invoke).
        # Call _sort directly: it should treat None as no items.
        list_ops._sort()
        assert list_ops._param.outputs["result"]["value"] == []
