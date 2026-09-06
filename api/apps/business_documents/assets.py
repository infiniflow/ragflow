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

import json
import hashlib
import re
from copy import deepcopy
from functools import lru_cache
from pathlib import Path
from typing import Any

from jsonschema import Draft202012Validator

from api.apps.business_documents.errors import ValidationError


_ASSET_ROOT = Path(__file__).resolve().parents[3] / "agent" / "business_requirements"
_CONTRACT_FILES = {
    "create_document": "create_document.v1.schema.json",
    "create_document_v2": "create_document.v2.schema.json",
    "command": "command.v1.schema.json",
    "question_batch": "question_batch.v1.schema.json",
    "document_draft": "document_draft.v1.schema.json",
    "change_plan": "change_plan.v1.schema.json",
    "review_plan": "review_plan.v1.schema.json",
}
_PROMPT_FILES = {
    "intake": ("intake.v1.md", "1"),
    "review": ("review.v1.md", "1"),
    "draft": ("draft.v1.md", "1"),
    "change_planner": ("change_planner.v1.md", "1"),
}
_JOB_PROMPTS = {
    "ASSESS_INTAKE": "intake",
    "ASSESS_REVIEW": "review",
    "GENERATE_DRAFT": "draft",
    "PLAN_CHANGES": "change_planner",
}


@lru_cache(maxsize=None)
def _load_json(relative_path: str) -> dict[str, Any]:
    path = _ASSET_ROOT / relative_path
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise RuntimeError(f"Business requirements asset is unavailable or invalid: {path}") from exc


def process_policy() -> dict[str, Any]:
    return _load_json("policies/process.v1.json")


def rendering_policy() -> dict[str, Any]:
    return _load_json("policies/rendering.v1.json")


def published_template() -> dict[str, Any]:
    template = _load_json("templates/business_requirements.v1.json")
    if template.get("status") != "PUBLISHED":
        raise RuntimeError("Business requirements template must be published")
    return template


def contract_schema(name: str) -> dict[str, Any]:
    try:
        filename = _CONTRACT_FILES[name]
    except KeyError as exc:
        raise RuntimeError(f"Unknown business requirements contract: {name}") from exc
    return _load_json(f"contracts/{filename}")


@lru_cache(maxsize=None)
def prompt_text(name: str) -> str:
    try:
        filename, _ = _PROMPT_FILES[name]
    except KeyError as exc:
        raise RuntimeError(f"Unknown business requirements prompt: {name}") from exc
    path = _ASSET_ROOT / "prompts" / filename
    try:
        return path.read_text(encoding="utf-8")
    except OSError as exc:
        raise RuntimeError(f"Business requirements prompt is unavailable: {path}") from exc


def prompt_descriptor(job_type: str) -> dict[str, str] | None:
    name = _JOB_PROMPTS.get(job_type)
    if name is None:
        return None
    _, version = _PROMPT_FILES[name]
    content = prompt_text(name)
    return {
        "name": name,
        "version": version,
        "content_hash": f"sha256:{hashlib.sha256(content.encode('utf-8')).hexdigest()}",
    }


def validate_contract(name: str, value: object) -> None:
    errors = sorted(Draft202012Validator(contract_schema(name)).iter_errors(value), key=lambda item: list(item.path))
    if not errors:
        return
    error = errors[0]
    path = ".".join(str(part) for part in error.absolute_path)
    raise ValidationError(
        f"INVALID_{name.upper()}",
        f"Contract {name} validation failed{f' at {path}' if path else ''}: {error.message}",
        {"path": path, "validator": error.validator},
    )


def validate_document_ast(document: object) -> dict[str, Any]:
    document = normalize_document_ast(document)
    validate_contract("document_draft", document)
    assert isinstance(document, dict)
    template = published_template()
    if document["template_version"] != template["template_version"]:
        raise ValidationError("TEMPLATE_VERSION_CONFLICT", "Draft does not use the document template version")
    expected = [(section["id"], section["title"]) for section in template["sections"]]
    actual = [(section["id"], section["title"]) for section in document["sections"]]
    if actual != expected:
        raise ValidationError(
            "TEMPLATE_STRUCTURE_MISMATCH",
            "Draft sections must exactly match the published semantic template",
            {"expected_section_ids": [item[0] for item in expected], "actual_section_ids": [item[0] for item in actual]},
        )
    required_ids = {section["id"] for section in template["sections"] if section["required"]}
    allowed_blocks = {section["id"]: set(section["allowed_blocks"]) for section in template["sections"]}
    disallowed = [{"section_id": section["id"], "block_type": block["type"]} for section in document["sections"] for block in section["blocks"] if block["type"] not in allowed_blocks[section["id"]]]
    if disallowed:
        raise ValidationError(
            "BLOCK_TYPE_NOT_ALLOWED",
            "Document block type is not allowed in the target template section",
            {"blocks": disallowed},
        )
    empty_required = [section["id"] for section in document["sections"] if section["id"] in required_ids and not any(_block_has_content(block) for block in section["blocks"])]
    if empty_required:
        raise ValidationError(
            "REQUIRED_SECTION_EMPTY",
            "Required template sections must contain substantive content",
            {"section_ids": empty_required},
        )
    sections = {section["id"]: section for section in document["sections"]}
    _validate_conceptual_diagram(sections["4.1"])
    _validate_client_scenario(sections["4.3"])
    return document


def normalize_document_ast(document: object) -> object:
    """Restore the published optional section skeleton before validation.

    Section presence and ordering are deterministic template concerns.  The
    model owns section content, but it must not be able to accidentally drop
    optional headings from the canonical document outline.
    """

    if not isinstance(document, dict) or not isinstance(document.get("sections"), list):
        return document
    template_sections = published_template()["sections"]
    actual_sections = document["sections"]
    if not all(isinstance(section, dict) and isinstance(section.get("id"), str) for section in actual_sections):
        return document
    expected_ids = [section["id"] for section in template_sections]
    expected_id_set = set(expected_ids)
    flattened_sections = []
    for section in actual_sections:
        section = deepcopy(section)
        blocks = section.get("blocks")
        if isinstance(blocks, list):
            content_blocks = []
            for block in blocks:
                if isinstance(block, dict) and block.get("id") in expected_id_set and isinstance(block.get("blocks"), list):
                    flattened_sections.append(block)
                else:
                    content_blocks.append(block)
            section["blocks"] = content_blocks
        flattened_sections.append(section)
    actual_ids = [section["id"] for section in flattened_sections]
    if len(actual_ids) != len(set(actual_ids)) or not set(actual_ids).issubset(expected_ids):
        return document
    by_id = {section["id"]: section for section in flattened_sections}
    normalized = deepcopy(document)
    normalized_sections = []
    for template_section in template_sections:
        section = deepcopy(
            by_id.get(
                template_section["id"],
                {"id": template_section["id"], "title": template_section["title"], "blocks": []},
            )
        )
        if template_section["required"] and not any(_block_has_content(block) for block in section.get("blocks", [])):
            prefix = f"{template_section['id']}."
            allowed = set(template_section["allowed_blocks"])
            inherited = next(
                (
                    deepcopy(block)
                    for child in flattened_sections
                    if child["id"].startswith(prefix)
                    for block in child.get("blocks", [])
                    if isinstance(block, dict) and block.get("type") in allowed and _block_has_content(block)
                ),
                None,
            )
            if inherited is not None:
                section["blocks"] = [inherited]
        for template_only_field in ("parent_id", "required", "allowed_blocks", "semantic_requirements"):
            section.pop(template_only_field, None)
        normalized_sections.append(section)
    normalized["sections"] = normalized_sections
    return normalized


_NEGATIVE_PATH_PATTERN = re.compile(
    r"(?:негатив\w*|ошиб\w*|отказ\w*|исключ\w*|\bнет\b|недоступ\w*|отсутств\w*|невозмож\w*|negative\w*|error\w*|failure\w*|reject\w*|denied\w*|unavailable\w*|\bno\b)",
    re.IGNORECASE,
)


def _validate_conceptual_diagram(section: dict[str, Any]) -> None:
    diagrams = [block for block in section["blocks"] if block.get("type") == "plantuml"]
    if not diagrams:
        raise ValidationError(
            "CONCEPTUAL_DIAGRAM_REQUIRED",
            "Section 4.1 must contain a PlantUML conceptual diagram",
            {"section_id": "4.1"},
        )
    for diagram in diagrams:
        source = diagram["source"].strip()
        if not source.startswith("@startuml") or not source.endswith("@enduml"):
            raise ValidationError(
                "INVALID_PLANTUML_DIAGRAM",
                "Section 4.1 PlantUML must be bounded by @startuml and @enduml",
                {"section_id": "4.1"},
            )


def _validate_client_scenario(section: dict[str, Any]) -> None:
    diagrams = [block for block in section["blocks"] if block.get("type") == "plantuml"]
    accompanying = [block for block in section["blocks"] if block.get("type") != "plantuml" and _block_has_content(block)]
    if not diagrams or not accompanying:
        raise ValidationError(
            "ACTIVITY_SCENARIO_REQUIRED",
            "Section 4.3 must contain a PlantUML activity diagram and accompanying scenario text",
            {"section_id": "4.3"},
        )
    for diagram in diagrams:
        source = diagram["source"].strip()
        bounded = source.startswith("@startuml") and source.endswith("@enduml")
        has_start = re.search(r"(?im)^\s*start\s*$", source) is not None
        has_end = re.search(r"(?im)^\s*(?:stop|end)\s*$", source) is not None
        has_decision = re.search(r"(?im)^\s*if\s*\(.+\)\s*then(?:\s*\(.+\))?\s*$", source) is not None
        alternative = re.search(
            r"(?ims)^\s*else(?:\s*\((?P<label>[^)]*)\))?\s*\r?\n(?P<body>.*?)^\s*endif\s*$",
            source,
        )
        has_alternative = alternative is not None
        has_decision_end = re.search(r"(?im)^\s*endif\s*$", source) is not None
        negative_branch = "" if alternative is None else f"{alternative.group('label') or ''} {alternative.group('body')}"
        has_negative_path = _NEGATIVE_PATH_PATTERN.search(negative_branch) is not None
        if not all((bounded, has_start, has_end, has_decision, has_alternative, has_decision_end, has_negative_path)):
            raise ValidationError(
                "INCOMPLETE_ACTIVITY_SCENARIO",
                "PlantUML activity diagram must contain start/end, an if/else decision, and an explicitly named negative alternative path",
                {"section_id": "4.3"},
            )


def _block_has_content(block: dict[str, Any]) -> bool:
    block_type = block.get("type")
    if block_type == "paragraph":
        return bool(str(block.get("text", "")).strip())
    if block_type == "list":
        return any(str(item).strip() for item in block.get("items", []))
    if block_type == "table":
        return bool(block.get("headers") or block.get("rows"))
    if block_type == "plantuml":
        return bool(str(block.get("source", "")).strip())
    if block_type in {"image", "reference"}:
        return bool(str(block.get("url", "")).strip())
    return False


def section_hash(section: dict[str, Any]) -> str:
    encoded = json.dumps(section, ensure_ascii=False, separators=(",", ":"), sort_keys=True).encode("utf-8")
    return f"sha256:{hashlib.sha256(encoded).hexdigest()}"


def bind_change_plan_section_hashes(base_document: dict[str, Any], change_plan: object) -> object:
    """Bind concurrency hashes from the immutable server snapshot.

    A language model selects and rewrites sections; it does not calculate the
    optimistic-concurrency token used to protect those sections.
    """

    if not isinstance(change_plan, dict) or not isinstance(change_plan.get("operations"), list):
        return change_plan
    hashes = {section["id"]: section_hash(section) for section in base_document.get("sections", []) if isinstance(section, dict) and isinstance(section.get("id"), str)}
    bound = deepcopy(change_plan)
    for operation in bound["operations"]:
        if not isinstance(operation, dict):
            continue
        expected_hash = hashes.get(operation.get("section_id"))
        if expected_hash is not None:
            operation["expected_section_hash"] = expected_hash
    return bound


def apply_change_plan(base_document: dict[str, Any], change_plan: dict[str, Any]) -> dict[str, Any]:
    """Apply the only supported AST operation and reject stale/duplicate targets."""

    result = deepcopy(base_document)
    sections = {section["id"]: section for section in result["sections"]}
    seen_sections: set[str] = set()
    for operation in change_plan["operations"]:
        section_id = operation["section_id"]
        if section_id in seen_sections:
            raise ValidationError("DUPLICATE_CHANGE_TARGET", "A section may be replaced only once per change plan", {"section_id": section_id})
        seen_sections.add(section_id)
        section = sections.get(section_id)
        if section is None:
            raise ValidationError("CHANGE_SECTION_NOT_FOUND", "Change plan targets a section outside the template", {"section_id": section_id})
        actual_hash = section_hash(section)
        if operation["expected_section_hash"] != actual_hash:
            raise ValidationError(
                "SECTION_HASH_CONFLICT",
                "Change plan targets stale section content",
                {"section_id": section_id, "expected": operation["expected_section_hash"], "actual": actual_hash},
            )
        section["blocks"] = deepcopy(operation["content"]["blocks"])
        if "evidence_refs" in operation:
            section["evidence_refs"] = deepcopy(operation["evidence_refs"])
    return validate_document_ast(result)


def render_section_text(section: dict[str, Any]) -> str:
    """Render the canonical Markdown body for one section, excluding its heading."""

    lines: list[str] = []
    for block in section["blocks"]:
        block_type = block["type"]
        if block_type == "paragraph":
            lines.append(str(block.get("text", "")).strip())
        elif block_type == "list":
            lines.extend(f"- {item}" for item in block.get("items", []))
        elif block_type == "table":
            headers = [_canonical_scalar(item) for item in block.get("headers", [])]
            rows = block.get("rows", [])
            if headers:
                lines.append("| " + " | ".join(headers) + " |")
                lines.append("| " + " | ".join("---" for _ in headers) + " |")
                lines.extend("| " + " | ".join(_canonical_scalar(item) for item in row) + " |" for row in rows)
        elif block_type == "plantuml":
            lines.extend(["```plantuml", str(block.get("source", "")).strip(), "```"])
        elif block_type == "image":
            lines.append(f"![{block.get('alt', '')}]({block.get('url', '')})")
        elif block_type == "reference":
            label = block.get("label") or block.get("url") or "Источник"
            lines.append(f"[{label}]({block.get('url', '')})")
    return "\n".join(lines).strip()


def _canonical_scalar(value: object) -> str:
    if isinstance(value, float) and value.is_integer():
        return str(int(value))
    return str(value)


def render_document_ast(document: dict[str, Any]) -> str:
    base_level = int(published_template().get("rendering", {}).get("body_heading_base_level", 2))
    lines: list[str] = []
    for section in document["sections"]:
        level = base_level + section["id"].count(".")
        lines.append(f"{'#' * level} {section['id']}. {section['title']}")
        section_text = render_section_text(section)
        if section_text:
            lines.extend(section_text.splitlines())
        lines.append("")
    return "\n".join(lines).strip()
