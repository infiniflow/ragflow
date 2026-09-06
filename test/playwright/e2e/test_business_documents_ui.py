import json
import re

import pytest
from playwright.sync_api import expect

from test.playwright.helpers._auth_helpers import ensure_authed
from test.playwright.helpers._next_apps_helpers import RESULT_TIMEOUT_MS, _goto_home


def _envelope(data):
    return {"code": 0, "data": data}


def _projection(*, allowed_commands=None):
    return {
        "document_id": "doc-ui-1",
        "title": "Переводы одной кнопкой",
        "document_type": "business_requirements",
        "state_version": 18,
        "lifecycle_state": "REVIEW",
        "operation_state": "IDLE",
        "current_revision": {
            "revision_id": "revision-3",
            "revision_number": 3,
            "document_ast": {
                "schema_version": "1",
                "document_type": "business_requirements",
                "template_version": "1.0.0",
                "sections": [
                    {
                        "id": "1",
                        "title": "Цель",
                        "blocks": [
                            {
                                "type": "paragraph",
                                "text": "Сократить время перевода до одной минуты.",
                            }
                        ],
                    }
                ],
            },
            "section_texts": {
                "1": "Сократить время перевода до одной минуты."
            },
            "body_markdown": "## 1. Цель\nСократить время перевода до одной минуты.",
            "content_hash": "sha256:ui-test",
        },
        "active_review_cycle": 2,
        "protocol": {
            "questions": [
                {
                    "question_id": "question-1",
                    "sequence_number": 1,
                    "target_section_id": "1",
                    "text": "Как измеряется успех?",
                    "options": [
                        {"option_id": "time", "label": "По времени"},
                        {"option_id": "quality", "label": "По качеству"},
                    ],
                    "allow_custom_answer": True,
                    "status": "OPEN",
                }
            ],
            "proposals": [
                {
                    "proposal_id": "proposal-1",
                    "target_section_id": "1",
                    "text": "Добавить целевую метрику",
                    "rationale": "Требование должно быть измеримым",
                    "decision": "PENDING",
                }
            ],
            "comments": [],
        },
        "allowed_commands": allowed_commands
        or ["ANSWER_QUESTION", "DECIDE_PROPOSAL", "ADD_COMMENT", "APPLY_CHANGES"],
        "latest_exports": [
            {
                "artifact_id": "artifact-md-r3",
                "format": "MARKDOWN",
                "filename": "requirements.md",
                "revision_id": "revision-3",
                "revision_number": 3,
                "created_at": 1,
            }
        ],
    }


def _authenticate(
    page,
    login_url,
    active_auth_context,
    auth_click,
    seeded_user_credentials,
):
    ensure_authed(
        page,
        login_url,
        active_auth_context,
        auth_click,
        seeded_user_credentials=seeded_user_credentials,
    )


@pytest.mark.p1
@pytest.mark.auth
def test_business_documents_list_validation_and_create_navigation(
    page,
    base_url,
    login_url,
    active_auth_context,
    auth_click,
    seeded_user_credentials,
):
    _authenticate(
        page,
        login_url,
        active_auth_context,
        auth_click,
        seeded_user_credentials,
    )
    created_payloads = []

    def route_business_documents(route):
        request = route.request
        if request.method == "GET":
            route.fulfill(
                status=200,
                content_type="application/json",
                body=json.dumps(
                    _envelope(
                        {
                            "items": [
                                {
                                    "document_id": "doc-ui-1",
                                    "title": "Сохранённые требования",
                                    "lifecycle_state": "REVIEW",
                                    "operation_state": "IDLE",
                                    "current_revision_number": 2,
                                    "update_time": 1_756_000_000,
                                }
                            ],
                            "page": 1,
                            "page_size": 20,
                            "total": 1,
                        }
                    ),
                    ensure_ascii=False,
                ),
            )
            return
        created_payloads.append(request.post_data_json)
        route.fulfill(
            status=200,
            content_type="application/json",
            body=json.dumps(_envelope(_projection()), ensure_ascii=False),
        )

    page.route("**/api/v1/business-documents**", route_business_documents)
    page.route(
        "**/api/v1/datasets**",
        lambda route: route.fulfill(
            status=200,
            content_type="application/json",
            body=json.dumps(_envelope([])),
        ),
    )

    _goto_home(page, base_url)
    expect(page.locator("[data-testid='nav-business-documents']").first).to_have_attribute(
        "href", "/business-documents"
    )
    page.goto(f"{base_url.rstrip('/')}/business-documents")
    expect(page.locator("[data-testid='business-documents-create']")).to_be_visible(
        timeout=RESULT_TIMEOUT_MS
    )
    expect(page.locator("[data-testid='business-document-list-item']")).to_contain_text(
        "Сохранённые требования"
    )

    submit = page.get_by_role("button", name="Начать работу")
    expect(submit).to_be_disabled()
    page.get_by_label("Название документа").fill("  Новый регламент  ")
    page.get_by_label("Описание идеи").fill("  Согласовать единый процесс.  ")
    expect(submit).to_be_enabled()
    submit.click()

    expect(page).to_have_url(re.compile(r"/business-documents/doc-ui-1$"))
    assert created_payloads == [
        {
            "schema_version": "2",
            "document_type": "business_requirements",
            "title": "Новый регламент",
            "idea": "Согласовать единый процесс.",
            "dataset_ids": [],
        }
    ]


@pytest.mark.p1
@pytest.mark.auth
def test_business_document_workbench_commands_and_mobile_layout(
    page,
    base_url,
    login_url,
    active_auth_context,
    auth_click,
    seeded_user_credentials,
):
    _authenticate(
        page,
        login_url,
        active_auth_context,
        auth_click,
        seeded_user_credentials,
    )
    commands = []

    def route_document(route):
        request = route.request
        if request.method == "GET":
            route.fulfill(
                status=200,
                content_type="application/json",
                body=json.dumps(_envelope(_projection()), ensure_ascii=False),
            )
            return
        commands.append(request.post_data_json)
        route.fulfill(
            status=200,
            content_type="application/json",
            body=json.dumps(
                _envelope(
                    {
                        "accepted": True,
                        "document_id": "doc-ui-1",
                        "state_version": 19,
                        "lifecycle_state": "REVIEW",
                        "operation_state": "IDLE",
                    }
                )
            ),
        )

    page.route("**/api/v1/business-documents/doc-ui-1", route_document)
    page.route("**/api/v1/business-documents/doc-ui-1/commands", route_document)
    page.goto(f"{base_url.rstrip('/')}/business-documents/doc-ui-1")

    workbench = page.locator("[data-testid='business-document-workbench']")
    expect(workbench).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    expect(page.get_by_role("heading", name="Переводы одной кнопкой")).to_be_visible()
    expect(page.locator("[data-testid='business-document-pane']")).to_contain_text(
        "Сократить время перевода"
    )
    expect(page.locator("[data-testid='business-document-protocol']")).to_contain_text(
        "Как измеряется успех?"
    )
    expect(page.get_by_role("link", name="Markdown r3")).to_have_attribute(
        "href",
        "/api/v1/business-documents/doc-ui-1/exports/artifact-md-r3/download",
    )

    page.get_by_label("По времени").check()
    with page.expect_request("**/api/v1/business-documents/doc-ui-1/commands") as request_info:
        page.locator("[data-testid='answer-question-question-1']").click()
    submitted_command = request_info.value.post_data_json
    assert submitted_command["type"] == "ANSWER_QUESTION"
    assert submitted_command["expected_state_version"] == 18
    assert submitted_command["payload"] == {
        "question_id": "question-1",
        "selected_option_id": "time",
        "custom_answer": None,
    }
    assert commands == [submitted_command]

    page.set_viewport_size({"width": 390, "height": 844})
    expect(page.locator("[data-testid='business-document-header']")).to_be_visible()
    expect(page.locator("[data-testid='business-document-actions']")).to_be_visible()
    expect(page.locator("[data-testid='apply-changes-button']")).to_be_visible()
