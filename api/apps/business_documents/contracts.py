#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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

from __future__ import annotations

from dataclasses import dataclass
from enum import StrEnum
from typing import Any

from api.apps.business_documents.assets import validate_contract
from api.apps.business_documents.errors import ValidationError


class LifecycleState(StrEnum):
    INTAKE = "INTAKE"
    REVIEW = "REVIEW"
    AGREED = "AGREED"
    ARCHIVED = "ARCHIVED"


class CommandType(StrEnum):
    REQUEST_INTAKE_ASSESSMENT = "REQUEST_INTAKE_ASSESSMENT"
    REQUEST_REVIEW_ASSESSMENT = "REQUEST_REVIEW_ASSESSMENT"
    ANSWER_QUESTION = "ANSWER_QUESTION"
    REQUEST_DRAFT = "REQUEST_DRAFT"
    DECIDE_PROPOSAL = "DECIDE_PROPOSAL"
    ADD_COMMENT = "ADD_COMMENT"
    APPLY_CHANGES = "APPLY_CHANGES"
    START_REVIEW = "START_REVIEW"
    REQUEST_EXPORT = "REQUEST_EXPORT"
    ARCHIVE = "ARCHIVE"


@dataclass(frozen=True)
class CommandEnvelope:
    schema_version: str
    command_id: str
    idempotency_key: str
    expected_state_version: int
    type: CommandType
    payload: dict[str, Any]

    @classmethod
    def parse(cls, raw: object) -> "CommandEnvelope":
        validate_contract("command", raw)
        assert isinstance(raw, dict)
        try:
            command_type = CommandType(raw["type"])
        except (TypeError, ValueError) as exc:
            raise ValidationError("UNKNOWN_COMMAND", "Unsupported command type", {"type": raw.get("type")}) from exc
        return cls(
            schema_version="1",
            command_id=raw["command_id"].strip(),
            idempotency_key=raw["idempotency_key"].strip(),
            expected_state_version=raw["expected_state_version"],
            type=command_type,
            payload=raw["payload"],
        )
