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
def test_switch_numeric_variable_with_unparseable_value_falls_through_to_else():
    """A numeric variable compared against an unparseable value is a non-match,
    not a ValueError that kills the canvas run (#19416)."""
    param = SwitchParam()
    param.conditions = [
        {
            "logical_operator": "and",
            "items": [{"cpn_id": "score", "operator": "=", "value": ""}],
            "to": ["case_target"],
        }
    ]
    param.end_cpn_ids = ["else_target"]

    cpn = _switch(param, {"score": 5})
    cpn._invoke()

    assert cpn.output("_next") == ["else_target"]
    assert cpn.output("next") == ["else_target"]


@pytest.mark.p1
@pytest.mark.parametrize("operator", [">", "<", "≥", "≤"])
def test_switch_ordering_operator_with_incomparable_value_is_non_match(operator):
    """An ordering operator on an incomparable pair is a non-match instead of a
    TypeError from the numeric fallback (#19416)."""
    cpn = _switch(SwitchParam())

    assert cpn.process_operator(5, operator, "abc") is False
    assert cpn.process_operator(None, operator, None) is False


@pytest.mark.p1
def test_switch_ordering_operator_still_compares_numerically_and_textually():
    """Parseable pairs keep comparing numerically; text pairs keep their own ordering."""
    cpn = _switch(SwitchParam())

    assert cpn.process_operator(5, ">", "3") is True
    assert cpn.process_operator("5", "<", 9) is True
    assert cpn.process_operator("abc", ">", "abd") is False
