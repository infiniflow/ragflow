from __future__ import annotations

from typing import Callable, Sequence

import pytest

StepFn = Callable[..., None]
Steps = Sequence[tuple[str, StepFn]]


def flow_params(steps: Steps):
    return [pytest.param(step_fn, id=step_id) for step_id, step_fn in steps]


def make_flow(steps: Steps):
    params = flow_params(steps)

    def run(step_fn: StepFn, *args, **kwargs):
        return step_fn(*args, **kwargs)

    return params, run


def require(flow_state: dict, *keys: str) -> None:
    missing = [key for key in keys if not flow_state.get(key)]
    if missing:
        pytest.skip(f"Missing prerequisite: {', '.join(missing)}")
