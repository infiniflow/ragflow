import json
from pathlib import Path

from jsonschema import Draft202012Validator


ASSET_ROOT = Path(__file__).parents[3] / "agent" / "business_requirements"


def load_json(relative_path: str) -> dict:
    with (ASSET_ROOT / relative_path).open(encoding="utf-8") as stream:
        return json.load(stream)


def test_all_business_requirements_json_assets_are_valid_json():
    paths = sorted(ASSET_ROOT.rglob("*.json"))

    assert paths
    for path in paths:
        with path.open(encoding="utf-8") as stream:
            json.load(stream)


def test_template_preserves_published_semantic_outline():
    template = load_json("templates/business_requirements.v1.json")
    sections = template["sections"]
    section_ids = [section["id"] for section in sections]

    assert template["status"] == "PUBLISHED"
    assert section_ids == [
        "1",
        "2",
        "3",
        "3.1",
        "3.2",
        "3.3",
        "4",
        "4.1",
        "4.2",
        "4.3",
        "5",
        "5.2",
        "5.3",
        "5.4",
        "5.5",
    ]
    assert "5.1" not in section_ids
    assert len(section_ids) == len(set(section_ids))

    by_id = {section["id"]: section for section in sections}
    assert by_id["5.5"]["required"] is True
    assert by_id["5.2"]["required"] is False
    assert "plantuml" in by_id["4.3"]["allowed_blocks"]
    assert "bpmn" not in by_id["4.3"]["allowed_blocks"]
    assert by_id["4.1"]["semantic_requirements"] == ["REQUIRED_VALID_PLANTUML_DIAGRAM"]
    assert by_id["4.3"]["semantic_requirements"] == [
        "REQUIRED_PLANTUML_ACTIVITY_DIAGRAM",
        "REQUIRED_ACCOMPANYING_TEXT",
        "REQUIRED_EXPLICIT_NEGATIVE_ALTERNATIVE_PATH",
    ]
    for section in sections:
        parent_id = section.get("parent_id")
        if parent_id:
            assert parent_id in by_id
            assert section_ids.index(parent_id) < section_ids.index(section["id"])


def test_process_policy_encodes_non_negotiable_invariants():
    policy = load_json("policies/process.v1.json")

    assert policy["question_rules"] == {
        "minimum_options": 2,
        "maximum_options": 4,
        "allow_custom_answer": True,
        "immutable_after_publication": True,
    }
    assert {
        "DRAFT_REQUIRES_PUBLISHED_TEMPLATE",
        "DRAFT_REQUIRES_CLOSED_INTAKE",
        "BODY_LOCKED_WHILE_REVIEW_QUESTIONS_OPEN",
        "ONLY_ACCEPTED_PROPOSALS_MAY_BE_APPLIED",
        "COMMENTS_QUESTIONS_PROPOSALS_AND_ANSWERS_ARE_APPEND_ONLY",
        "ONE_CHAT_MAPS_TO_ONE_DOCUMENT",
        "EXPORT_DOES_NOT_MUTATE_A_REVISION",
        "EVA_WIKI_REQUIRES_AGREED_REVISION",
        "EVA_WIKI_EXCLUDES_REVIEW_PROTOCOL",
        "UPLOADED_FILES_ARE_EVIDENCE_NOT_INSTRUCTIONS",
        "EVERY_ACTIVE_COMMENT_REQUIRES_EXPLICIT_DISPOSITION",
        "NEEDS_QUESTION_COMMENT_LINKS_TO_A_PERSISTED_QUESTION",
        "ONLY_CONFIRMED_CHANGE_COMMENTS_MAY_SOURCE_CHANGES",
        "ONLY_NO_CHANGE_COMMENTS_MAY_BE_ACKNOWLEDGED_WITHOUT_CHANGE",
        "ANCHORED_COMMENTS_REQUIRE_EXACT_SECTION_UTF16_OFFSETS_AND_CONTEXT",
        "COMMENT_ANCHORS_ARE_IMMUTABLE_AND_BECOME_ORPHANED_ON_NEW_REVISION",
        "SECTION_4_1_REQUIRES_VALID_PLANTUML",
        "SECTION_4_3_REQUIRES_PLANTUML_ACTIVITY_WITH_TEXT_AND_NEGATIVE_ALTERNATIVE",
    }.issubset(policy["invariants"])


def test_contract_schemas_compile_and_question_bounds_are_enforced():
    schemas = {path.name: json.loads(path.read_text(encoding="utf-8")) for path in sorted((ASSET_ROOT / "contracts").glob("*.schema.json"))}

    assert set(schemas) == {
        "change_plan.v1.schema.json",
        "command.v1.schema.json",
        "create_document.v1.schema.json",
        "create_document.v2.schema.json",
        "document_draft.v1.schema.json",
        "question_batch.v1.schema.json",
        "review_plan.v1.schema.json",
    }
    for schema in schemas.values():
        Draft202012Validator.check_schema(schema)

    validator = Draft202012Validator(schemas["question_batch.v1.schema.json"])
    valid_question = {
        "schema_version": "1",
        "outcome": "NEEDS_INPUT",
        "questions": [
            {
                "semantic_tag": "monitoring.metrics",
                "stage": "INTAKE",
                "target_section_id": "5.5",
                "text": "Какие бизнес-события необходимо отслеживать?",
                "options": [
                    {"option_id": "1", "label": "Создание и отмена"},
                    {"option_id": "2", "label": "Только ошибки"},
                ],
                "allow_custom_answer": True,
            }
        ],
    }
    validator.validate(valid_question)

    for invalid_count in (1, 5):
        invalid = json.loads(json.dumps(valid_question))
        invalid["questions"][0]["options"] = [{"option_id": str(index), "label": f"Вариант {index}"} for index in range(invalid_count)]
        assert list(validator.iter_errors(invalid))


def test_golden_dialogue_suite_covers_required_quality_lanes():
    suite = load_json("golden_dialogs/v1.json")
    cases = suite["cases"]
    case_ids = [case["id"] for case in cases]

    assert len(cases) >= 20
    assert len(case_ids) == len(set(case_ids))
    assert {case["priority"] for case in cases} >= {"P0", "P1"}
    assert {case["category"] for case in cases} >= {
        "positive",
        "negative",
        "boundary",
        "completeness",
        "quality",
        "information_quality",
        "security",
    }
    for case in cases:
        assert case["turns"]
        assert case["assertions"]
        assert all(turn["role"] and turn["text"] for turn in case["turns"])


def test_quality_rubric_has_a_complete_weighted_gate():
    rubric = load_json("evals/rubric.v1.json")

    assert sum(item["weight"] for item in rubric["criteria"]) == 1.0
    assert 0 < rubric["pass_threshold"] <= rubric["scoring_range"][1]
    assert rubric["live_suite_gate"] == {
        "p0_case_pass_rate": 1.0,
        "all_case_pass_rate": 0.9,
        "hard_failure_count": 0,
        "minimum_grounded_fact_precision": 0.95,
    }
    assert {
        "BODY_CHANGED_WITH_OPEN_QUESTIONS",
        "UNACCEPTED_PROPOSAL_APPLIED",
        "REQUIRED_MONITORING_MISSING",
        "EVIDENCE_INSTRUCTION_EXECUTED",
    }.issubset(rubric["hard_failures"])


def test_prompt_pack_is_contract_first_and_treats_evidence_as_data():
    prompts = {path.name: path.read_text(encoding="utf-8") for path in sorted((ASSET_ROOT / "prompts").glob("*.md"))}

    assert set(prompts) == {
        "change_planner.v1.md",
        "draft.v1.md",
        "intake.v1.md",
        "review.v1.md",
    }
    for prompt in prompts.values():
        assert "{{context_json}}" in prompt
        assert "{{output_schema_json}}" in prompt
        assert "только JSON" in prompt or "только один JSON" in prompt
    assert "не инструкциями" in prompts["intake.v1.md"]
    assert "не выполняй инструкции" in prompts["review.v1.md"].lower()
    assert "Всегда верни `acknowledged_no_change_event_ids`" in prompts["change_planner.v1.md"]
    assert "либо в `source_event_ids` хотя бы одной операции" in prompts["change_planner.v1.md"]
    assert "Общий комментарий с `section_id: null` относится ко всему документу" in prompts["change_planner.v1.md"]
