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


def test_switch_none_upstream_value_routes_to_else():
    # Regression: a None upstream value used to raise AttributeError on
    # None.lower() for the string operators instead of routing to else.
    param = SwitchParam()
    param.conditions = [
        {
            "logical_operator": "and",
            "items": [{"cpn_id": "answer", "operator": "contains", "value": "yes"}],
            "to": ["case_target"],
        }
    ]
    param.end_cpn_ids = ["else_target"]

    cpn = _switch(param, {"answer": None})
    cpn._invoke()

    assert cpn.output("_next") == ["else_target"]
    assert cpn.output("next") == ["else_target"]


def test_process_operator_handles_none_for_string_operators():
    cpn = Switch.__new__(Switch)
    for operator in ["contains", "not contains", "start with", "end with"]:
        # Must not raise on a None input/value.
        assert cpn.process_operator(None, operator, "yes") in (True, False)
        assert cpn.process_operator("yes", operator, None) in (True, False)
    assert cpn.process_operator(None, "empty", "") is True
    assert cpn.process_operator(None, "not empty", "") is False
