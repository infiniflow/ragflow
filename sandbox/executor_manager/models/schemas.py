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
import base64
from typing import Optional

from pydantic import BaseModel, Field, field_validator

from models.enums import ResourceLimitType, ResultStatus, RuntimeErrorType, SupportLanguage, UnauthorizedAccessType


class CodeExecutionResult(BaseModel):
    status: ResultStatus
    stdout: str
    stderr: str
    exit_code: int
    detail: Optional[str] = None

    # Resource usage
    time_used_ms: Optional[float] = None
    memory_used_kb: Optional[float] = None

    # Error details
    resource_limit_type: Optional[ResourceLimitType] = None
    unauthorized_access_type: Optional[UnauthorizedAccessType] = None
    runtime_error_type: Optional[RuntimeErrorType] = None


class CodeExecutionRequest(BaseModel):
    code_b64: str = Field(..., description="Base64 encoded code string")
    language: SupportLanguage = Field(default=SupportLanguage.PYTHON, description="Programming language")
    arguments: Optional[dict] = Field(default={}, description="Arguments")

    @field_validator("code_b64")
    @classmethod
    def validate_base64(cls, v: str) -> str:
        try:
            base64.b64decode(v, validate=True)
            return v
        except Exception as e:
            raise ValueError(f"Invalid base64 encoding: {str(e)}")
