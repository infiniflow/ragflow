import pytest

from agent.component.variable_assigner import VariableAssigner, VariableAssignerParam


class _FakeCanvas:
    def __init__(self, values=None):
        self._values = values or {}

    def get_variable_value(self, key):
        return self._values.get(key)

    def set_variable_value(self, key, value):
        self._values[key] = value


def _make_component(variables, initial=None):
    comp = VariableAssigner.__new__(VariableAssigner)
    comp._canvas = _FakeCanvas(initial or {"counter": 5, "items": [1, 2, 3]})
    comp._param = VariableAssignerParam()
    comp._param.variables = variables
    return comp


@pytest.mark.p1
def test_set_number_zero_succeeds():
    comp = _make_component([{"variable": "counter", "operator": "set", "parameter": 0}])
    comp._invoke()
    assert comp._canvas._values["counter"] == 0


@pytest.mark.p1
def test_clear_without_parameter_succeeds():
    comp = _make_component([{"variable": "items", "operator": "clear"}])
    comp._invoke()
    assert comp._canvas._values["items"] == []
