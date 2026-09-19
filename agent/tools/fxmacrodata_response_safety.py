"""Sanitize public responses, including JSON stored inside MCP text blocks."""

from __future__ import annotations

import json
import re
from typing import Any

from agent.tools.fxmacrodata_client.redaction import REDACTED, credential_variants, redact_credentials

_SECRET_FIELD = re.compile(
    r"^(?:api[_-]?key|access[_-]?token|authorization|proxy[_-]?authorization|"
    r"password|client[_-]?secret|secret|token)$",
    re.IGNORECASE,
)


def sanitize_response(value: Any, api_key: str = "") -> Any:
    """Keep JSON scalar types and redact credentials before record projection.

    MCP text may itself contain JSON, whose Unicode escapes hide credentials
    from a literal string replacement. Parse those strings before redaction,
    and retain their original formatting when no value needs changing.
    """
    variants = credential_variants(api_key, json_escapes=True)

    def text(item: str) -> str:
        return redact_credentials(item, variants)

    def visit(item: Any, depth: int) -> Any:
        if depth > 64:
            raise ValueError("FXMacroData response exceeds the supported nesting depth.")
        if isinstance(item, str):
            try:
                parsed = json.loads(item)
            except RecursionError:
                raise ValueError("FXMacroData response exceeds the supported nesting depth.") from None
            except ValueError:
                return text(item)
            if not isinstance(parsed, (str, list, dict)):
                return text(item)
            safe = visit(parsed, depth + 1)
            return item if safe == parsed else json.dumps(safe, ensure_ascii=False)
        if isinstance(item, list):
            return [visit(child, depth + 1) for child in item]
        if isinstance(item, dict):
            return {text(key): REDACTED if _SECRET_FIELD.fullmatch(key) else visit(child, depth + 1) for key, child in item.items()}
        return item

    return visit(value, 0)
