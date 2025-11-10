#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
from typing import Any, Literal

from pydantic import BaseModel, ConfigDict, Field, model_validator


class TokenizerFromUpstream(BaseModel):
    created_time: float | None = Field(default=None, alias="_created_time")
    elapsed_time: float | None = Field(default=None, alias="_elapsed_time")

    name: str = ""
    file: dict | None = Field(default=None)

    output_format: Literal["json", "markdown", "text", "html", "chunks"] | None = Field(default=None)

    chunks: list[dict[str, Any]] | None = Field(default=None)

    json_result: list[dict[str, Any]] | None = Field(default=None, alias="json")
    markdown_result: str | None = Field(default=None, alias="markdown")
    text_result: str | None = Field(default=None, alias="text")
    html_result: str | None = Field(default=None, alias="html")

    model_config = ConfigDict(populate_by_name=True, extra="forbid")

    @model_validator(mode="after")
    def _check_payloads(self) -> "TokenizerFromUpstream":
        if self.chunks:
            return self

        if self.output_format in {"markdown", "text", "html"}:
            if self.output_format == "markdown" and not self.markdown_result:
                raise ValueError("output_format=markdown requires a markdown payload (field: 'markdown' or 'markdown_result').")
            if self.output_format == "text" and not self.text_result:
                raise ValueError("output_format=text requires a text payload (field: 'text' or 'text_result').")
            if self.output_format == "html" and not self.html_result:
                raise ValueError("output_format=text requires a html payload (field: 'html' or 'html_result').")
        else:
            if not self.json_result and not self.chunks:
                raise ValueError("When no chunks are provided and output_format is not markdown/text, a JSON list payload is required (field: 'json' or 'json_result').")
        return self
