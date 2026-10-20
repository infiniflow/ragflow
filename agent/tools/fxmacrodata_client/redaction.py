"""One credential redaction rule set for transport diagnostics and responses."""

from __future__ import annotations

import json
import re
from urllib.parse import quote, quote_plus

REDACTED = "[redacted]"
_BEARER_ASSIGNMENT = re.compile(r"(?i)((?:proxy[_-]?)?authorization[\"']?\s*[:=]\s*[\"']?)(?:Bearer|Basic)\s+[^\s\"'&<>]+")
_CREDENTIAL_ASSIGNMENT = re.compile(r"(?i)((?:api[_-]?key|access[_-]?token|authorization|password|client[_-]?secret|token)[\"']?\s*[:=]\s*[\"']?)[^\s\"'&<>]+")


def credential_variants(api_key: str, *, json_escapes: bool = False) -> tuple[str, ...]:
    """Every spelling of ``api_key`` that text may carry, longest first.

    URL encodings cover query strings and logged request lines. JSON escapes
    cover a key hidden inside a JSON document stored in a text field.
    """
    if not api_key:
        return ()
    variants = {api_key, quote(api_key, safe=""), quote_plus(api_key, safe=""), quote_plus(api_key)}
    if json_escapes:
        variants.update(json.dumps(api_key, ensure_ascii=ascii_only)[1:-1] for ascii_only in (True, False))
        units = api_key.encode("utf-16-be", errors="surrogatepass")
        for hex_case in ("04x", "04X"):
            variants.add("".join("\\u" + format(int.from_bytes(units[i : i + 2], "big"), hex_case) for i in range(0, len(units), 2)))
    variants.update(re.sub(r"%[0-9A-F]{2}", lambda match: match[0].lower(), item) for item in tuple(variants))
    return tuple(sorted(variants, key=len, reverse=True))


def redact_credentials(value: str, variants: tuple[str, ...] = ()) -> str:
    """Replace the configured key spellings and any credential-shaped assignment."""
    for variant in variants:
        value = value.replace(variant, REDACTED)
    value = _BEARER_ASSIGNMENT.sub(r"\1" + REDACTED, value)
    return _CREDENTIAL_ASSIGNMENT.sub(r"\1" + REDACTED, value)
