from collections.abc import Mapping
from typing import Any


def get_vlm_config(config: Mapping[str, Any]) -> dict[str, Any]:
    nested = config.get("vlm")
    legacy_llm_id = config.get("llm_id")

    if isinstance(nested, Mapping):
        resolved = dict(nested)
        if resolved.get("llm_id") or not legacy_llm_id:
            return resolved
        resolved["llm_id"] = legacy_llm_id
        return resolved

    if legacy_llm_id:
        return {"llm_id": legacy_llm_id}
    return {}
