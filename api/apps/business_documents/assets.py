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

import hashlib
import json
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
    "create_document_v3": "create_document.v3.schema.json",
    "command": "command.v1.schema.json",
    "question_batch": "question_batch.v1.schema.json",
    "document_draft": "document_draft.v1.schema.json",
    "change_plan": "change_plan.v1.schema.json",
    "review_plan": "review_plan.v1.schema.json",
    "eva_change_draft": "eva_change_draft.v1.schema.json",
}
_PROMPT_FILES = {
    "intake": ("intake.v1.md", "1"),
    "review": ("review.v1.md", "1"),
    "draft": ("draft.v1.md", "1"),
    "change_planner": ("change_planner.v1.md", "1"),
    "eva_change": ("eva_change.v1.md", "1"),
}
_JOB_PROMPTS = {
    "ASSESS_INTAKE": "intake",
    "ASSESS_REVIEW": "review",
    "GENERATE_DRAFT": "draft",
    "PLAN_CHANGES": "change_planner",
    "GENERATE_EVA_CHANGE": "eva_change",
}
_MARKDOWN_SECTION_HEADING = re.compile(r"^(#{1,6})\s+([0-9]+(?:\.[0-9]+)*)\.\s+(.+?)\s*$")
_FENCED_CODE_BLOCK = re.compile(
    r"```(?P<language>[A-Za-z0-9_-]*)[ \t]*\n(?P<source>.*?)(?:\n)?```",
    re.DOTALL,
)
_MAX_PARAGRAPH_SIZE = 20_000


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


def import_document_markdown(markdown: object) -> dict[str, Any]:
    """Convert a previously exported business document back to its governed AST.

    EVA pages are accepted only when their numbered headings still match the
    published template. This avoids silently assigning arbitrary page content to
    the wrong semantic sections while keeping the import deterministic and free
    of an LLM rewrite.
    """

    if not isinstance(markdown, str) or not markdown.strip():
        raise ValidationError("EVA_DOCUMENT_EMPTY", "Страница EVA не содержит опубликованного текста")
    normalized = markdown.replace("\r\n", "\n").replace("\r", "\n").strip()
    template = published_template()
    template_sections = template["sections"]
    expected = {section["id"]: section for section in template_sections}
    base_level = int(template.get("rendering", {}).get("body_heading_base_level", 2))

    found: list[tuple[str, int, int]] = []
    offset = 0
    for line in normalized.splitlines(keepends=True):
        heading = _MARKDOWN_SECTION_HEADING.match(line.rstrip("\n"))
        if heading:
            section_id = heading.group(2)
            template_section = expected.get(section_id)
            expected_level = base_level + section_id.count(".")
            if template_section is not None and len(heading.group(1)) == expected_level and heading.group(3).strip() == template_section["title"]:
                found.append((section_id, offset, offset + len(line)))
        offset += len(line)

    actual_ids = [item[0] for item in found]
    expected_ids = [section["id"] for section in template_sections]
    if actual_ids != expected_ids:
        raise ValidationError(
            "EVA_DOCUMENT_TEMPLATE_MISMATCH",
            "Страница EVA не соответствует текущему шаблону бизнес-документа",
            {"expected_section_ids": expected_ids, "actual_section_ids": actual_ids},
        )
    if normalized[: found[0][1]].strip():
        raise ValidationError(
            "EVA_DOCUMENT_TEMPLATE_MISMATCH",
            "Перед первым разделом страницы EVA найден текст вне шаблона",
        )

    sections = []
    for index, (section_id, _, body_start) in enumerate(found):
        body_end = found[index + 1][1] if index + 1 < len(found) else len(normalized)
        body = normalized[body_start:body_end].strip()
        sections.append(
            {
                "id": section_id,
                "title": expected[section_id]["title"],
                "blocks": _import_markdown_blocks(body),
            }
        )
    return validate_document_ast(
        {
            "schema_version": "1",
            "document_type": "business_requirements",
            "template_version": template["template_version"],
            "sections": sections,
        }
    )


def _import_markdown_blocks(markdown: str) -> list[dict[str, Any]]:
    blocks: list[dict[str, Any]] = []
    cursor = 0
    for match in _FENCED_CODE_BLOCK.finditer(markdown):
        source = match.group("source").strip()
        language = match.group("language").casefold()
        is_plantuml = language == "plantuml" or (source.startswith("@startuml") and source.endswith("@enduml"))
        if not is_plantuml:
            continue
        blocks.extend(_paragraph_blocks(markdown[cursor : match.start()].strip()))
        if source:
            blocks.append({"type": "plantuml", "source": source})
        cursor = match.end()
    blocks.extend(_paragraph_blocks(markdown[cursor:].strip()))
    return blocks


def _paragraph_blocks(text: str) -> list[dict[str, str]]:
    blocks: list[dict[str, str]] = []
    remaining = text
    while remaining:
        if len(remaining) <= _MAX_PARAGRAPH_SIZE:
            blocks.append({"type": "paragraph", "text": remaining})
            break
        split_at = remaining.rfind("\n", 0, _MAX_PARAGRAPH_SIZE + 1)
        if split_at <= 0:
            split_at = _MAX_PARAGRAPH_SIZE
        blocks.append({"type": "paragraph", "text": remaining[:split_at]})
        remaining = remaining[split_at:].lstrip("\n")
    return blocks


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


def _validate_conceptual_diagram(section: dict[str, Any]) -> None:
    diagrams = [block for block in section["blocks"] if block.get("type") == "plantuml"]
    if not diagrams:
        raise ValidationError(
            "CONCEPTUAL_DIAGRAM_REQUIRED",
            "Section 4.1 must contain a PlantUML conceptual diagram",
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
