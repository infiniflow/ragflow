from typing import Any, Literal

from pydantic import BaseModel, ConfigDict, Field, model_validator


class TokenizerFromUpstream(BaseModel):
    created_time: float | None = Field(default=None, alias="_created_time")
    elapsed_time: float | None = Field(default=None, alias="_elapsed_time")

    name: str = ""
    blob: bytes

    output_format: Literal["json", "markdown", "text", "html"]

    chunks: list[dict[str, Any]] | None = None

    json_result: list[dict[str, Any]] | None = Field(default=None, alias="json")
    markdown_result: str | None = Field(default=None, alias="markdown")
    text_result: str | None = Field(default=None, alias="text")
    html_result: str | None = Field(default=None, alias="html")

    model_config = ConfigDict(populate_by_name=True, extra="forbid")

    @model_validator(mode="after")
    def _check_payloads(self) -> "TokenizerFromUpstream":
        if self.chunks:
            return self

        if self.output_format in {"markdown", "text"}:
            if self.output_format == "markdown" and not self.markdown_result:
                raise ValueError("output_format=markdown requires a markdown payload (field: 'markdown' or 'markdown_result').")
            if self.output_format == "text" and not self.text_result:
                raise ValueError("output_format=text requires a text payload (field: 'text' or 'text_result').")
        else:
            if not self.json_result:
                raise ValueError("When no chunks are provided and output_format is not markdown/text, a JSON list payload is required (field: 'json' or 'json_result').")
        return self
