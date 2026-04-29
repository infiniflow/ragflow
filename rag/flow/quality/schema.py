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
#

from typing import Any, Dict, List, Literal, Optional

from pydantic import BaseModel, ConfigDict, Field


class QualityAnalyzerFromUpstream(BaseModel):
    created_time: float | None = Field(default=None, alias="_created_time")
    elapsed_time: float | None = Field(default=None, alias="_elapsed_time")

    name: str | None = Field(default=None)
    file: dict | None = Field(default=None)
    chunks: list[dict[str, Any]] | None = Field(default=None)

    output_format: Literal["json", "markdown", "text", "html", "chunks"] | None = Field(default=None)

    json_result: list[dict[str, Any]] | None = Field(default=None, alias="json")
    markdown_result: str | None = Field(default=None, alias="markdown")
    text_result: str | None = Field(default=None, alias="text")
    html_result: str | None = Field(default=None, alias="html")

    model_config = ConfigDict(populate_by_name=True, extra="ignore")


class QualityIssueModel(BaseModel):
    issue_type: str
    risk_level: str
    description: str
    details: Dict[str, Any] = Field(default_factory=dict)
    suggestion: str = ""

    model_config = ConfigDict(extra="forbid")


class ChunkQualityResultModel(BaseModel):
    chunk_index: int
    quality_score: float
    issues: List[QualityIssueModel] = Field(default_factory=list)
    risk_level: str
    metadata: Dict[str, Any] = Field(default_factory=dict)

    model_config = ConfigDict(extra="forbid")


class BatchQualitySummaryModel(BaseModel):
    total_chunks: int
    average_quality: float
    risk_distribution: Dict[str, int]
    issue_distribution: Dict[str, int]
    high_risk_count: int
    high_risk_indices: List[int]

    model_config = ConfigDict(extra="forbid")
