from collections.abc import Mapping
from typing import Any


def metadata_supports_model_type(metadata: Mapping[str, Any] | None, requested_type: str) -> bool:
    if not metadata:
        return False

    model_types = metadata.get("model_type", [])
    if isinstance(model_types, str):
        model_types = [model_types]
    aliases = {"image2text": "vision", "speech2text": "asr"}
    normalized_types = {aliases.get(str(value).lower(), str(value).lower()) for value in model_types}
    requested = requested_type.lower()
    if requested in normalized_types:
        return True

    tags = {tag.strip().lower() for tag in str(metadata.get("tags", "")).split(",")}
    return requested == "chat" and "chat" in tags
