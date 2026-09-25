import pytest

from agent.component.switch import Switch, SwitchParam


class _Canvas:
    def __init__(self, variables=None):
        self.variables = variables or {}

    def is_canceled(self):
        return False

    def get_variable_value(self, cpn_id):
        return self.variables[cpn_id]

    def get_component_name(self, cpn_id):
        return cpn_id


def _switch(param, variables=None):
    cpn = Switch.__new__(Switch)
    cpn._canvas = _Canvas(variables)
    cpn._id = "switch"
    cpn._param = param
    return cpn


def test_switch_empty_condition_falls_through_to_else():
    param = SwitchParam()
    param.conditions = [
        {
            "logical_operator": "and",
            "items": [{"cpn_id": "", "operator": "=", "value": "yes"}],
            "to": ["case_target"],
        }
    ]
    param.end_cpn_ids = ["else_target"]

    cpn = _switch(param)
    cpn._invoke()

    assert cpn.output("_next") == ["else_target"]
    assert cpn.output("next") == ["else_target"]


def test_switch_non_empty_and_condition_still_matches():
    param = SwitchParam()
    param.conditions = [
        {
            "logical_operator": "and",
            "items": [{"cpn_id": "answer", "operator": "=", "value": "yes"}],
            "to": ["case_target"],
        }
    ]
    param.end_cpn_ids = ["else_target"]

    cpn = _switch(param, {"answer": "yes"})
    cpn._invoke()

    assert cpn.output("_next") == ["case_target"]
    assert cpn.output("next") == ["case_target"]


@pytest.mark.p1
def test_switch_none_input_contains_falls_through_to_else():
    param = SwitchParam()
    param.conditions = [
        {
            "logical_operator": "and",
            "items": [{"cpn_id": "answer", "operator": "contains", "value": "foo"}],
            "to": ["case_target"],
        }
    ]
    param.end_cpn_ids = ["else_target"]

    cpn = _switch(param, {"answer": None})
    cpn._invoke()

    assert cpn.output("_next") == ["else_target"]
    assert cpn.output("next") == ["else_target"]


@pytest.mark.p1
def test_switch_none_value_contains_does_not_raise():
    param = SwitchParam()
    param.conditions = [
        {
            "logical_operator": "and",
            "items": [{"cpn_id": "answer", "operator": "contains", "value": None}],
            "to": ["case_target"],
        }
    ]
    param.end_cpn_ids = ["else_target"]

    cpn = _switch(param, {"answer": "foobar"})
    cpn._invoke()

    assert cpn.output("_next") == ["case_target"]
    assert cpn.output("next") == ["case_target"]


@pytest.mark.p1
@pytest.mark.parametrize("operator", ["=", "≠", "==", "!=", "<>"])
def test_switch_equality_operator_with_unparseable_value_falls_through_to_else(operator):
    """A numeric variable compared against an unparseable value is a non-match,
    not a ValueError that kills the canvas run (#19416)."""
    param = SwitchParam()
    param.conditions = [
        {
            "logical_operator": "and",
            "items": [{"cpn_id": "score", "operator": operator, "value": ""}],
            "to": ["case_target"],
        }
    ]
    param.end_cpn_ids = ["else_target"]

    cpn = _switch(param, {"score": 5})
    cpn._invoke()

    assert cpn.output("_next") == ["else_target"]
    assert cpn.output("next") == ["else_target"]


@pytest.mark.p1
@pytest.mark.parametrize("operator", [">", "<", "≥", "≤", ">=", "<="])
def test_switch_ordering_operator_requires_numeric_operands(operator):
    """Ordering operators deliberately raise on non-numeric operands (#19987);
    the equality guard must not leak into them."""
    cpn = _switch(SwitchParam())

    with pytest.raises(ValueError, match="requires numeric operands"):
        cpn.process_operator(5, operator, "abc")
    with pytest.raises(ValueError, match="requires numeric operands"):
        cpn.process_operator("abc", operator, "abd")
    with pytest.raises(ValueError, match="requires numeric operands"):
        cpn.process_operator(True, operator, 1)


@pytest.mark.p1
@pytest.mark.parametrize(
    "operator, left, right, expected",
    [
        (">", 5, "3", True),
        ("<", "5", 9, True),
        (">=", 5, 5, True),
        ("<=", 5, 6, True),
        (">", 1, 2, False),
    ],
)
def test_switch_ordering_operator_still_compares_numerically(operator, left, right, expected):
    """Parseable pairs keep comparing numerically, operator aliases included."""
    cpn = _switch(SwitchParam())

    assert cpn.process_operator(left, operator, right) is expected
