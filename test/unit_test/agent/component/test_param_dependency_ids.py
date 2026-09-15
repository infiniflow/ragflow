import pytest

from agent.component.iteration import Iteration, IterationParam
from agent.component.list_operations import ListOperations, ListOperationsParam
from agent.component.switch import Switch, SwitchParam
from agent.component.variable_assigner import VariableAssigner, VariableAssignerParam


@pytest.mark.parametrize(
    ("component", "param", "values", "expected"),
    [
        (
            Switch,
            SwitchParam,
            {"conditions": [{"items": [{"cpn_id": "producer@result"}]}]},
            ["producer"],
        ),
        (
            VariableAssigner,
            VariableAssignerParam,
            {"variables": [{"variable": "target@value", "parameter": "source@value"}]},
            ["target", "source"],
        ),
        (Iteration, IterationParam, {"items_ref": "producer@items"}, ["producer"]),
        (ListOperations, ListOperationsParam, {"query": "producer@items"}, ["producer"]),
    ],
)
def test_parameter_references_are_scheduler_dependencies(component, param, values, expected):
    params = param()
    for key, value in values.items():
        setattr(params, key, value)

    cpn = component.__new__(component)
    cpn._param = params

    assert cpn.get_dependency_ids() == expected
