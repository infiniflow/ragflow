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

import json

from api.db.services.system_settings_service import SystemSettingsService


NAVIGATION_VISIBILITY_SETTING = "navigation.visible_sections"
NAVIGATION_VISIBILITY_VERSION = 2
NAVIGATION_SECTIONS = (
    "home",
    "dataset",
    "chat",
    "search",
    "agent",
    "memory",
    "catalog",
    "business_documents",
    "file_manager",
)
ACCESS_CONTROLLED_NAVIGATION_SECTIONS = tuple(section for section in NAVIGATION_SECTIONS if section != "home")


def validate_visible_sections(value: object) -> list[str]:
    if not isinstance(value, list):
        raise ValueError("visible_sections must be an array")
    if not all(isinstance(section, str) for section in value):
        raise ValueError("visible_sections must contain only strings")

    unknown_sections = sorted(set(value) - set(NAVIGATION_SECTIONS))
    if unknown_sections:
        raise ValueError(f"Unknown navigation sections: {', '.join(unknown_sections)}")
    if len(value) != len(set(value)):
        raise ValueError("visible_sections must not contain duplicates")

    selected = set(value)
    return [section for section in NAVIGATION_SECTIONS if section in selected]


def get_visible_sections() -> list[str]:
    settings = SystemSettingsService.get_by_name(NAVIGATION_VISIBILITY_SETTING)
    if len(settings) != 1:
        return list(NAVIGATION_SECTIONS)

    try:
        stored = json.loads(settings[0].value)
        if isinstance(stored, dict):
            if stored.get("version") != NAVIGATION_VISIBILITY_VERSION:
                raise ValueError("Unknown navigation visibility version")
            return validate_visible_sections(stored.get("visible_sections"))

        # Settings written before the home toggle existed implicitly kept Home
        # visible. Preserve that behavior until the administrator saves v2.
        return validate_visible_sections(["home", *stored])
    except (TypeError, ValueError, json.JSONDecodeError):
        return list(NAVIGATION_SECTIONS)


def serialize_visible_sections(value: object) -> str:
    return json.dumps(
        {
            "version": NAVIGATION_VISIBILITY_VERSION,
            "visible_sections": validate_visible_sections(value),
        }
    )
