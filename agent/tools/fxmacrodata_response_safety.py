"""Sanitize public responses, including JSON stored inside MCP text blocks."""

from __future__ import annotations

import json
import re
from typing import Any
from urllib.parse import quote, quote_plus

_SECRET_FIELD = re.compile(
    r"^(?:api[_-]?key|access[_-]?token|authorization|proxy[_-]?authorization|"
    r"password|client[_-]?secret|secret|token)$", re.IGNORECASE
)


def sanitize_response(value: Any, api_key: str = "") -> Any:
    """Keep JSON scalar types and redact credentials before record projection.

    MCP text may itself contain JSON, whose Unicode escapes hide credentials
    from a literal string replacement. Parse those strings before redaction,
    and retain their original formatting when no value needs changing.
    """
    variants = {api_key, quote(api_key, safe=""), quote_plus(api_key)} if api_key else set()
    if api_key:
        variants.update(json.dumps(api_key, ensure_ascii=ascii_only)[1:-1] for ascii_only in (True, False))
        units = api_key.encode("utf-16-be", errors="surrogatepass")
        for hex_case in ("04x", "04X"):
            variants.add("".join("\\u" + format(int.from_bytes(units[i:i + 2], "big"), hex_case) for i in range(0, len(units), 2)))
    variants.update(re.sub(r"%[0-9A-F]{2}", lambda match: match[0].lower(), item) for item in tuple(variants))

    def text(item: str) -> str:
        for variant in sorted(variants, key=len, reverse=True):
            item = item.replace(variant, "[redacted]")
        item = re.sub(
            r"(?i)((?:proxy[_-]?)?authorization[\"']?\s*[:=]\s*[\"']?)(?:Bearer|Basic)\s+[^\s\"'&<>]+",
            r"\1[redacted]", item,
        )
        return re.sub(
            r"(?i)((?:api[_-]?key|access[_-]?token|authorization|password|client[_-]?secret|token)[\"']?\s*[:=]\s*[\"']?)[^\s\"'&<>]+",
            r"\1[redacted]", item,
        )

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
            return {
                text(key): "[redacted]" if _SECRET_FIELD.fullmatch(key) else visit(child, depth + 1)
                for key, child in item.items()
            }
        return item

    return visit(value, 0)
