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


def validate_template_payload(req: dict, require_all: bool = True) -> str:
    """Validate a single template payload (kind + config + name)."""
    required = ["name", "kind", "config"] if require_all else []
    for key in required:
        if key not in req:
            return f"Missing required field: {key}."

    name = req.get("name")
    if name is not None and (not isinstance(name, str) or not name.strip() or len(name.encode("utf-8")) > 128):
        return "Invalid template name."

    description = req.get("description")
    if description is not None and (not isinstance(description, str) or len(description) > 1024):
        return "Invalid template description."

    kind = req.get("kind")
    if kind is not None and (not isinstance(kind, str) or not kind):
        return "Invalid template kind."

    config = req.get("config")
    if config is not None and not isinstance(config, dict):
        return "Invalid template config."
    if isinstance(config, dict):
        if len(str(config.get("global_rules") or "")) > 4096:
            return "Global compilation rules is too long."
        for section in ["entity", "relation"]:
            fields = (config.get(section) or {}).get("fields") or []
            seen_types = set()
            for field in fields:
                field_type = str((field or {}).get("type") or "").strip()
                if not field_type:
                    return f"{section.capitalize()} type is required."
                if field_type in seen_types:
                    return f"{section.capitalize()} type can not be duplicated."
                seen_types.add(field_type)
                if not str((field or {}).get("description") or "").strip():
                    return f"{section.capitalize()} field description is required."
                if len(str((field or {}).get("description") or "")) > 1024:
                    return f"{section.capitalize()} field description is too long."
                if len(str((field or {}).get("rule") or "")) > 1024:
                    return f"{section.capitalize()} field rule is too long."
        if config.get("kind") == "artifacts" or req.get("kind") == "artifacts":
            for field in (config.get("claim") or {}).get("fields") or []:
                if not str((field or {}).get("statement") or "").strip():
                    return "Claim statement is required."
                if not str((field or {}).get("subject") or "").strip():
                    return "Claim subject is required."
                if len(str((field or {}).get("statement") or "")) > 1024:
                    return "Claim statement is too long."
                if len(str((field or {}).get("subject") or "")) > 1024:
                    return "Claim subject is too long."
            for field in (config.get("concept") or {}).get("fields") or []:
                if not str((field or {}).get("term") or "").strip():
                    return "Concept term is required."
                if not str((field or {}).get("definition_excerpt") or "").strip():
                    return "Concept definition excerpt is required."
                if len(str((field or {}).get("term") or "")) > 1024:
                    return "Concept term is too long."
                if len(str((field or {}).get("definition_excerpt") or "")) > 1024:
                    return "Concept definition excerpt is too long."

    return ""
