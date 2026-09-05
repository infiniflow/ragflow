from __future__ import annotations

def require(flow_state: dict, *keys: str) -> None:
    missing = [key for key in keys if not flow_state.get(key)]
    if missing:
        raise AssertionError(f"Missing prerequisite: {', '.join(missing)}")
