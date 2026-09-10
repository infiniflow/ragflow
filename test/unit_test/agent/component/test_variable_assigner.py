import pytest

from agent.component.variable_assigner import VariableAssigner


@pytest.mark.p1
@pytest.mark.parametrize(
    ("parameter", "expected"),
    [
        (True, True),
        (False, False),
        ("yes", True),
        ("no", False),
    ],
)
def test_set_boolean_preserves_booleans_and_migrates_legacy_values(parameter, expected):
    component = VariableAssigner.__new__(VariableAssigner)

    result = component._set(False, parameter)

    assert result is expected
    assert type(result) is bool
