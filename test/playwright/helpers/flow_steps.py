from __future__ import annotations

from typing import Callable, Sequence

import pytest

StepFn = Callable[..., None]
Steps = Sequence[tuple[str, StepFn]]


def flow_params(steps: Steps):
    return [pytest.param(step_fn, id=step_id) for step_id, step_fn in steps]


def require(flow_state: dict, *keys: str) -> None:
    missing = [key for key in keys if not flow_state.get(key)]
    if missing:
        pytest.skip(f"Missing prerequisite: {', '.join(missing)}")
