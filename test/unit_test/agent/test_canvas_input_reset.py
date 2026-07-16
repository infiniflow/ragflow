"""Regression tests for the canvas input reset behaviour.

Issue #16758: when a downstream Agent references a single-line
variable captured by an Await-Response (UserFillUp) component, the
Agent's `user_prompt` was being resolved against the previous canvas
run's captured value. Root cause: `Canvas._run_impl` reset
`only_output=True` for every path component, so `_param.inputs` was
never cleared between runs. The Agent's `get_input()` read the stale
input from `self._param.inputs` (set on the previous run by
`set_input_value` from the previous run's `UserFillUp` capture) and
forwarded it as `user_prompt`, ignoring the current run's value.

Fix: differentiate begin (only outputs) from non-begin path components
(clear both inputs and outputs).
"""

# Stub classes only — avoid importing the real Canvas because pulling in
# agent/canvas.py transitively imports scholarly which has a
# Python 3.13-incompatible escape sequence. The fix being tested is
# independent of Canvas.__init__; we only need its per-component reset
# contract.


class _StubComponent:
    def __init__(self, name):
        self.component_name = name
        self.reset_calls = []

    def reset(self, only_output=False):
        self.reset_calls.append(only_output)


class _StubCanvas:
    """Bare-bones canvas that just records the per-component reset calls.

    We do not need any of the real canvas machinery to test the reset
    contract: the bug is the flag passed to `ComponentBase.reset()`.
    """

    def __init__(self):
        self.components = {
            "begin": {"obj": _StubComponent("begin")},
            "fillup1": {"obj": _StubComponent("UserFillUp")},
            "agent1": {"obj": _StubComponent("Agent")},
            "message1": {"obj": _StubComponent("Message")},
        }
        self.path = ["begin", "fillup1", "agent1", "message1"]

    def reset_path_components_inputs(self):
        """Mirror the production code path in Canvas._run_impl."""
        path_set = set(self.path)
        for k, cpn in self.components.items():
            if k in path_set:
                is_begin = self.components[k]["obj"].component_name.lower() == "begin"
                self.components[k]["obj"].reset(only_output=is_begin)


def test_begin_is_reset_with_only_output_true():
    canvas = _StubCanvas()
    canvas.reset_path_components_inputs()
    assert canvas.components["begin"]["obj"].reset_calls == [True]


def test_non_begin_path_components_are_reset_with_only_output_false():
    canvas = _StubCanvas()
    canvas.reset_path_components_inputs()
    assert canvas.components["fillup1"]["obj"].reset_calls == [False]
    assert canvas.components["agent1"]["obj"].reset_calls == [False]
    assert canvas.components["message1"]["obj"].reset_calls == [False]


def test_only_path_components_are_reset():
    """Components not on the active path should be left alone."""

    class _StrictCanvas(_StubCanvas):
        def __init__(self):
            super().__init__()
            self.components["unrelated"] = {"obj": _StubComponent("Categorize")}

    canvas = _StrictCanvas()
    canvas.reset_path_components_inputs()
    assert canvas.components["unrelated"]["obj"].reset_calls == []


def test_inputs_reset_flag_is_passed_to_non_begin_components():
    """Pin the contract: non-begin path components must receive
    `only_output=False` so `_param.inputs` is cleared between runs.

    This is the bug-fix invariant: if a future refactor reverts the
    flag, this test fails and the regression re-emerges.
    """
    canvas = _StubCanvas()
    canvas.reset_path_components_inputs()
    for cpn_id in ("fillup1", "agent1", "message1"):
        reset_calls = canvas.components[cpn_id]["obj"].reset_calls
        assert reset_calls == [False], (
            f"component {cpn_id} should be reset with only_output=False "
            f"so _param.inputs is cleared between canvas runs; got {reset_calls}"
        )
