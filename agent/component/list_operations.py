from abc import ABC
import os
from functools import cmp_to_key

from agent.component.base import ComponentBase, ComponentParamBase
from api.utils.api_utils import timeout


# Type rank for the ListOperations ``sort`` op. Pairs of same-rank values
# compare numerically (numbers) or lexicographically (everything else). Across
# ranks, the lower rank always wins — this gives a strict, transitive
# ordering that avoids the cycle the previous ``lessScalar``-style
# comparator produced (``10 > 2``, ``2 > "11"``, ``"11" > 10``). The Go
# runtime port (internal/agent/component/list_operations.go ``lessKey``)
# uses the same rank-then-compare scheme, so flows run on either runtime
# produce the same ordering — see issue #19427 follow-up review.
_NUMERIC_RANK = 0
_TEXT_RANK = 1


def _scalar_rank(v):
    """Return the rank for ``v`` (lower wins). Numbers (``int``/``float``,
    excluding ``bool`` since Python treats ``bool`` as an ``int`` subclass)
    come first; everything else (``str``, ``None``, ``bool``, ``list``,
    etc.) sorts into the text rank. Matches Go's ``lessKey`` rank."""
    if not isinstance(v, bool) and isinstance(v, (int, float)):
        return _NUMERIC_RANK
    return _TEXT_RANK


def _go_format_scalar(v):
    """Render ``v`` the way Go's ``fmt.Sprintf(\"%v\", v)`` does for the
    non-numeric branch. Python's ``str()`` differs from Go's ``%v`` for
    some types (``None`` → ``\"None\"`` vs ``\"<nil>\"``,
    ``True``/``False`` → ``\"True\"/\"False\"`` vs ``\"true\"/\"false\"``).
    Pin a uniform formatter so the Python and Go runtimes agree on the
    lexicographic fallback (CodeRabbit review on PR #19782)."""
    if v is None:
        return "<nil>"
    if isinstance(v, bool):
        return "true" if v else "false"
    return str(v)


def _scalar_compare(a, b):
    """Comparator for the ListOperations ``sort`` op — transitive order
    matching the Go runtime's ``lessKey`` (internal/agent/component/list_operations.go).

    1. Rank by type: numbers (``int``/``float`` excluding ``bool``) before
       everything else. This breaks the previous cycle
       (``10 > 2``, ``2 > "11"``, ``"11" > 10``) where two numbers
       compared numerically but a number-vs-string fell through to
       lexicographic.
    2. Within the numeric rank: compare numerically.
    3. Within the text rank: compare lexicographically via
       :func:`_go_format_scalar` so Python ``str()`` / ``True`` / ``None``
       differences don't pull the ordering out of alignment with Go's
       ``fmt.Sprintf(\"%v\", v)`` fallback.

    ``bool`` is excluded from the numeric branch (matches Go's
    ``toFloat64OK`` returning ``false`` for ``bool``); ``True``/``False``
    sort by their Go-formatted string (``\"true\"`` < ``\"false\"``).
    """
    a_rank, b_rank = _scalar_rank(a), _scalar_rank(b)
    if a_rank != b_rank:
        return -1 if a_rank < b_rank else 1
    if a_rank == _NUMERIC_RANK:
        if a < b:
            return -1
        if a > b:
            return 1
        return 0
    sa, sb = _go_format_scalar(a), _go_format_scalar(b)
    if sa < sb:
        return -1
    if sa > sb:
        return 1
    return 0


def _scalar_sort_key(v):
    """Return a sort key for ``v`` that ``sorted`` can use safely.

    For numbers (rank 0), the key is ``(0, float(v), formatted)`` —
    including the float value forces Python's tuple comparison to order
    numbers numerically (``2 < 10``, not ``"10" < "2"`` lex order).

    For everything else (rank 1), the key is ``(1, formatted)`` — the
    rank itself ensures all numbers sort before all non-numbers, and the
    formatted string orders non-numbers lex (matching Go's
    ``fmt.Sprintf(\"%v\", v)``).
    """
    formatted = _go_format_scalar(v)
    if _scalar_rank(v) == _NUMERIC_RANK:
        return (0, float(v), formatted)
    return (1, formatted)


class ListOperationsParam(ComponentParamBase):
    """
    Define the List Operations component parameters.
    """

    def __init__(self):
        super().__init__()
        self.query = ""
        self.operations = "nth"
        self.n = 0
        self.strict = False
        self.sort_method = "asc"
        # Comma-separated list of map keys to sort by (primary,
        # tiebreak, ...). Empty / unset falls back to the legacy
        # full-hashable-key behaviour (sort by the lexicographically
        # first field). Mirrors internal/agent/component/list_operations.go
        # parseSortByFieldList + opSort's SortBy path.
        self.sort_by = ""
        self.filter = {"operator": "=", "value": ""}
        self.outputs = {"result": {"value": [], "type": "Array of ?"}, "first": {"value": "", "type": "?"}, "last": {"value": "", "type": "?"}}

    @staticmethod
    def _normalize_operation_name(operation):
        op = "" if operation is None else str(operation).strip()
        if op.lower() == "topn":
            return "head"
        return op or "nth"

    def check(self):
        self.check_empty(self.query, "query")
        self.operations = self._normalize_operation_name(self.operations)
        self.check_valid_value(
            self.operations,
            "Support operations",
            ["nth", "head", "tail", "filter", "sort", "drop_duplicates"],
        )

    def get_input_form(self) -> dict[str, dict]:
        return {}


class ListOperations(ComponentBase, ABC):
    component_name = "ListOperations"

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10 * 60)))
    def _invoke(self, **kwargs):
        self.input_objects = []
        inputs = getattr(self._param, "query", None)
        self.inputs = self._canvas.get_variable_value(inputs)
        if self.inputs is None:
            # A missing/unset variable (e.g. the upstream node was routed
            # around by a conditional branch) means "no variable passed in":
            # operate on an empty list instead of failing the whole run.
            # Non-list values are still a misconfiguration and keep raising.
            self.inputs = []
        elif not isinstance(self.inputs, list):
            raise TypeError("The input of List Operations should be an array.")
        self.set_input_value(inputs, self.inputs)
        if self._param.operations == "nth":
            self._nth()
        elif self._param.operations == "head":
            self._head()
        elif self._param.operations == "tail":
            self._tail()
        elif self._param.operations == "filter":
            self._filter()
        elif self._param.operations == "sort":
            self._sort()
        elif self._param.operations == "drop_duplicates":
            self._drop_duplicates()

    def _coerce_n(self):
        try:
            return int(getattr(self._param, "n", 0))
        except Exception:
            return 0

    def _is_strict(self):
        strict = getattr(self._param, "strict", False)
        if isinstance(strict, str):
            return strict.strip().lower() in {"1", "true", "yes", "on"}
        return bool(strict)

    def _set_outputs(self, outputs):
        self._param.outputs["result"]["value"] = outputs
        self._param.outputs["first"]["value"] = outputs[0] if outputs else None
        self._param.outputs["last"]["value"] = outputs[-1] if outputs else None

    def _raise_strict_range_error(self, operation, n):
        raise ValueError(f"{operation} requires n to be within the valid range in strict mode, got {n}.")

    def _nth(self):
        n = self._coerce_n()
        strict = self._is_strict()
        if n == 0:
            if strict:
                self._raise_strict_range_error("nth", n)
            outputs = []
        elif n > 0:
            if n <= len(self.inputs):
                outputs = [self.inputs[n - 1]]
            elif strict:
                self._raise_strict_range_error("nth", n)
            else:
                outputs = []
        else:
            if abs(n) <= len(self.inputs):
                outputs = [self.inputs[n]]
            elif strict:
                self._raise_strict_range_error("nth", n)
            else:
                outputs = []
        self._set_outputs(outputs)

    def _head(self):
        n = self._coerce_n()
        strict = self._is_strict()
        if strict:
            if 1 <= n <= len(self.inputs):
                outputs = self.inputs[:n]
            else:
                self._raise_strict_range_error("head", n)
        else:
            if n < 1:
                outputs = []
            else:
                outputs = self.inputs[:n]
        self._set_outputs(outputs)

    def _tail(self):
        n = self._coerce_n()
        strict = self._is_strict()
        if strict:
            if 1 <= n <= len(self.inputs):
                outputs = self.inputs[-n:]
            else:
                self._raise_strict_range_error("tail", n)
        else:
            if n < 1:
                outputs = []
            else:
                outputs = self.inputs[-n:]
        self._set_outputs(outputs)

    def _filter(self):
        self._set_outputs([i for i in self.inputs if self._eval(self._norm(i), self._param.filter["operator"], self._param.filter["value"])])

    def _norm(self, v):
        s = "" if v is None else str(v)
        return s

    def _eval(self, v, operator, value):
        if operator == "=":
            return v == value
        elif operator == "≠":
            return v != value
        elif operator == "contains":
            return value in v
        elif operator == "start with":
            return v.startswith(value)
        elif operator == "end with":
            return v.endswith(value)
        else:
            return False

    def _sort(self):
        items = self.inputs or []
        method = getattr(self._param, "sort_method", "asc") or "asc"
        reverse = method == "desc"

        if not items:
            self._set_outputs([])
            return

        first = items[0]

        if isinstance(first, dict):
            sort_by_raw = getattr(self._param, "sort_by", "") or ""
            sort_by = [k.strip() for k in sort_by_raw.split(",") if k.strip()]
            if sort_by:
                # Wrap each field value with ``_scalar_sort_key`` so the
                # resulting tuple of ``(rank, formatted_str[, float])``
                # keys compares cleanly across heterogeneous field types.
                outputs = sorted(
                    items,
                    key=lambda x: tuple(_scalar_sort_key(x.get(k)) for k in sort_by),
                    reverse=reverse,
                )
            else:
                outputs = sorted(
                    items,
                    key=lambda x: self._hashable(x),
                    reverse=reverse,
                )
        else:
            outputs = sorted(items, key=_scalar_sort_key, reverse=reverse)

        self._set_outputs(outputs)

    def _drop_duplicates(self):
        seen = set()
        outs = []
        for item in self.inputs:
            k = self._hashable(item)
            if k in seen:
                continue
            seen.add(k)
            outs.append(item)
        self._set_outputs(outs)

    def _hashable(self, x):
        if isinstance(x, dict):
            return tuple(sorted((k, self._hashable(v)) for k, v in x.items()))
        if isinstance(x, (list, tuple)):
            return tuple(self._hashable(v) for v in x)
        if isinstance(x, set):
            return tuple(sorted(self._hashable(v) for v in x))
        return x

    def thoughts(self) -> str:
        return "ListOperation in progress"
