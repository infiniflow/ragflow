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
        ("YES", True),
        ("No", False),
        ("true", True),
        ("FALSE", False),
    ],
)
def test_set_boolean_normalizes_boolean_spellings(parameter, expected):
    # Boolean normalization does not access instance state, so initialization is unnecessary.
    component = VariableAssigner.__new__(VariableAssigner)

    result = component._set(False, parameter)

    assert result is expected
    assert type(result) is bool
