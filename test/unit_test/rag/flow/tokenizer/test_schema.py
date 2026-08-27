import pytest
from pydantic import ValidationError

from rag.flow.tokenizer.schema import TokenizerFromUpstream


@pytest.mark.parametrize(
    ("output_format", "payload_field", "payload"),
    [
        ("chunks", "chunks", []),
        ("json", "json", []),
        ("markdown", "markdown", ""),
        ("text", "text", ""),
        ("html", "html", ""),
    ],
)
def test_empty_upstream_payloads_are_valid(output_format: str, payload_field: str, payload: object) -> None:
    result = TokenizerFromUpstream(output_format=output_format, **{payload_field: payload})

    assert result.model_dump(by_alias=True)[payload_field] == payload


@pytest.mark.parametrize("output_format", ["chunks", "json", "markdown", "text", "html"])
def test_missing_upstream_payload_is_rejected(output_format: str) -> None:
    with pytest.raises(ValidationError):
        TokenizerFromUpstream(output_format=output_format)
