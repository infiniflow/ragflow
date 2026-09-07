import json
import re
from urllib.parse import parse_qs, urlparse

import pytest
from playwright.sync_api import expect

from test.playwright.helpers._next_apps_helpers import RESULT_TIMEOUT_MS


DOCUMENT_ID = "doc-access-1"
ACTOR_ID = "browser-viewer"
CATALOG_ENTRY_ID = "L2-01.01.04.01.01"
CATALOG_ENTRY_TITLE = "Новый продукт"
ALL_SECTIONS = [
    "home",
    "dataset",
    "chat",
    "search",
    "agent",
    "memory",
    "catalog",
    "business_documents",
    "file_manager",
]


def _envelope(data, *, code=0, message=""):
    return {"code": code, "data": data, "message": message}


def _fulfill_json(route, data, *, status=200):
    route.fulfill(
        status=status,
        content_type="application/json",
        body=json.dumps(data, ensure_ascii=False),
    )


def _fulfill_error(route, status, error_code, message):
    _fulfill_json(
        route,
        {
            "code": status,
            "message": message,
            "data": {"error_code": error_code, "details": {}},
        },
        status=status,
    )


def _install_session(page, *, is_superuser=False):
    user_info = json.dumps(
        {
            "access_token": "browser-access-token",
            "id": ACTOR_ID,
            "email": "browser-viewer@example.test",
            "nickname": "Browser Viewer",
            "is_superuser": is_superuser,
        }
    )
    page.add_init_script(
        f"""
        (() => {{
          const userInfo = {user_info};
          localStorage.setItem('Authorization', 'Bearer browser-access-token');
          localStorage.setItem('token', 'browser-access-token');
          localStorage.setItem('userInfo', JSON.stringify(userInfo));
          localStorage.setItem('lng', 'ru');
        }})()
        """
    )


def _capabilities(role):
    create = role in {
        "AUTHOR_CREATOR",
        "MODERATOR_CREATOR",
        "EXTENDED_MODERATOR",
        "ADMIN",
    }
    edit_all = role in {"MODERATOR_CREATOR", "EXTENDED_MODERATOR", "ADMIN"}
    elevated = role in {"EXTENDED_MODERATOR", "ADMIN"}
    return {
        "read": True,
        "create": create,
        "edit_own": True,
        "edit_all": edit_all,
        "delete": elevated,
        "assign": elevated,
    }


def _permissions(role, owner_id):
    capabilities = _capabilities(role)
    return {
        "read": True,
        "edit": capabilities["edit_all"] or owner_id == ACTOR_ID,
        "delete": capabilities["delete"],
        "assign": capabilities["assign"],
    }


def _revision(*, number=3, author_id="author-1", author_name="Первый автор", author_login="author-1@example.test"):
    return {
        "revision_id": f"revision-{number}",
        "revision_number": number,
        "author_id": author_id,
        "author_name": author_name,
        "author_login": author_login,
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
                            "text": "Сократить время обработки до одной минуты.",
                        }
                    ],
                }
            ],
        },
        "section_texts": {"1": "Сократить время обработки до одной минуты."},
        "body_markdown": "## 1. Цель\nСократить время обработки до одной минуты.",
        "content_hash": f"sha256:browser-revision-{number}",
        "source_event_ids": [],
        "created_at": 1_788_200_000 + number,
        "change_basis": [],
    }


def _projection(role, *, owner_id="author-1", state_version=18):
    permissions = _permissions(role, owner_id)
    owner_names = {
        "author-1": "Первый автор",
        "author-2": "Второй автор",
    }
    return {
        "document_id": DOCUMENT_ID,
        "owner_id": owner_id,
        "owner_name": owner_names.get(owner_id),
        "access_role": role,
        "permissions": permissions,
        "title": "Чужой регламент",
        "document_type": "business_requirements",
        "state_version": state_version,
        "lifecycle_state": "REVIEW",
        "operation_state": "IDLE",
        "current_revision": _revision(),
        "active_review_cycle": 2,
        "protocol": {
            "questions": [
                {
                    "question_id": "question-access-1",
                    "sequence_number": 1,
                    "target_section_id": "1",
                    "text": "Как измеряется результат?",
                    "options": [
                        {"option_id": "time", "label": "По времени"},
                        {"option_id": "quality", "label": "По качеству"},
                    ],
                    "allow_custom_answer": True,
                    "status": "OPEN",
                }
            ],
            "proposals": [],
            "comments": [],
        },
        # Deliberately advertise an edit command even to read-only users. The UI
        # must still apply the document permission returned by the API.
        "allowed_commands": ["ANSWER_QUESTION", "ADD_COMMENT", "APPLY_CHANGES"],
        "latest_exports": [],
    }


class BusinessDocumentsAccessStub:
    def __init__(
        self,
        role,
        *,
        owner_id="author-1",
        create_status=201,
        assignment_status=200,
        command_status=200,
        delete_status=200,
        list_status=200,
    ):
        self.role = role
        self.owner_id = owner_id
        self.state_version = 18
        self.create_status = create_status
        self.assignment_status = assignment_status
        self.command_status = command_status
        self.delete_status = delete_status
        self.list_status = list_status
        self.list_scopes = []
        self.mutations = []
        self.document_requests = []

    def _list_data(self, scope):
        capabilities = _capabilities(self.role)
        own = {
            "document_id": "doc-mine",
            "owner_id": ACTOR_ID,
            "access_role": self.role,
            "permissions": _permissions(self.role, ACTOR_ID),
            "title": "Мой документ",
            "lifecycle_state": "REVIEW",
            "operation_state": "IDLE",
            "state_version": 4,
            "current_revision_number": 1,
            "update_time": 1_788_200_000,
        }
        foreign = {
            "document_id": DOCUMENT_ID,
            "owner_id": self.owner_id,
            "access_role": self.role,
            "permissions": _permissions(self.role, self.owner_id),
            "title": "Чужой документ",
            "lifecycle_state": "AGREED",
            "operation_state": "IDLE",
            "state_version": self.state_version,
            "current_revision_number": 3,
            "update_time": 1_788_200_001,
        }
        items = [own] if scope == "mine" else [own, foreign]
        return {
            "items": items,
            "page": 1,
            "page_size": 20,
            "total": len(items),
            "scope": scope,
            "access_role": self.role,
            "capabilities": capabilities,
        }

    def _current_projection(self):
        return _projection(
            self.role,
            owner_id=self.owner_id,
            state_version=self.state_version,
        )

    def __call__(self, route):
        request = route.request
        parsed = urlparse(request.url)
        path = parsed.path.rstrip("/")

        if path == "/api/v1/system/config":
            _fulfill_json(
                route,
                _envelope(
                    {
                        "registerEnabled": 0,
                        "disablePasswordLogin": False,
                        "visibleSections": ALL_SECTIONS,
                    }
                ),
            )
            return
        if path == "/api/v1/users/me":
            _fulfill_json(
                route,
                _envelope(
                    {
                        "id": ACTOR_ID,
                        "email": "browser-viewer@example.test",
                        "nickname": "Browser Viewer",
                        "language": "ru",
                        "avatar": None,
                    }
                ),
            )
            return
        if path in {"/api/v1/tenants", "/api/v1/datasets"}:
            _fulfill_json(route, _envelope([]))
            return
        if path == "/api/v1/users/me/eva-credentials":
            _fulfill_json(route, _envelope({"items": []}))
            return

        if path == "/api/v1/business-documents/catalog":
            _fulfill_json(
                route,
                _envelope(
                    {
                        "items": [
                            {
                                "id": CATALOG_ENTRY_ID,
                                "title": CATALOG_ENTRY_TITLE,
                                "title_en": "New Product",
                                "description": "Разрешённый L5-документ",
                                "capability_level": "L5",
                                "capability_type": "Core",
                                "hierarchy": {},
                            }
                        ],
                        "total": 1,
                    }
                ),
            )
            return

        if path == "/api/v1/business-documents/access/users":
            _fulfill_json(
                route,
                _envelope(
                    {
                        "items": [
                            {
                                "user_id": "author-1",
                                "nickname": "Первый автор",
                                "email": "author-1@example.test",
                                "role": "AUTHOR_CREATOR",
                            },
                            {
                                "user_id": "author-2",
                                "nickname": "Второй автор",
                                "email": "author-2@example.test",
                                "role": "AUTHOR_EDITOR",
                            },
                        ]
                    }
                ),
            )
            return

        if path == f"/api/v1/business-documents/{DOCUMENT_ID}/owner":
            payload = request.post_data_json
            self.mutations.append((request.method, path, payload))
            if self.assignment_status != 200:
                _fulfill_error(
                    route,
                    self.assignment_status,
                    "STATE_VERSION_CONFLICT",
                    "Документ уже изменён другим пользователем",
                )
                return
            self.owner_id = payload["owner_id"]
            self.state_version += 1
            _fulfill_json(route, _envelope(self._current_projection()))
            return

        if path == f"/api/v1/business-documents/{DOCUMENT_ID}/revisions":
            _fulfill_json(
                route,
                _envelope(
                    [
                        _revision(number=2, author_id="legacy-author", author_name=None, author_login=None),
                        _revision(number=3, author_id="author-2", author_name="Мария Авторова", author_login="maria@example.test"),
                    ]
                ),
            )
            return

        if path == f"/api/v1/business-documents/{DOCUMENT_ID}/commands":
            payload = request.post_data_json
            self.mutations.append((request.method, path, payload))
            if self.command_status != 200:
                _fulfill_error(
                    route,
                    self.command_status,
                    "PERMISSION_DENIED",
                    "Недостаточно прав для изменения документа",
                )
                return
            _fulfill_json(
                route,
                _envelope(
                    {
                        "accepted": True,
                        "document_id": DOCUMENT_ID,
                        "state_version": self.state_version + 1,
                        "lifecycle_state": "REVIEW",
                        "operation_state": "IDLE",
                    }
                ),
            )
            return

        if path == f"/api/v1/business-documents/{DOCUMENT_ID}":
            self.document_requests.append(request.method)
            if request.method == "DELETE":
                self.mutations.append((request.method, path, None))
                if self.delete_status != 200:
                    _fulfill_error(
                        route,
                        self.delete_status,
                        "PERMISSION_DENIED",
                        "Удаление документа запрещено",
                    )
                    return
                _fulfill_json(
                    route,
                    _envelope(
                        {
                            "document_id": DOCUMENT_ID,
                            "deleted": True,
                            "deleted_artifacts": 0,
                            "storage_cleanup_failures": 0,
                        }
                    ),
                )
                return
            _fulfill_json(route, _envelope(self._current_projection()))
            return

        if path == "/api/v1/business-documents":
            if request.method == "GET":
                if self.list_status != 200:
                    _fulfill_error(
                        route,
                        self.list_status,
                        "UNAUTHORIZED",
                        "Требуется авторизация",
                    )
                    return
                scope = parse_qs(parsed.query).get("scope", ["all"])[0]
                self.list_scopes.append(scope)
                _fulfill_json(route, _envelope(self._list_data(scope)))
                return
            payload = request.post_data_json
            self.mutations.append((request.method, path, payload))
            if self.create_status != 201:
                _fulfill_error(
                    route,
                    self.create_status,
                    "PERMISSION_DENIED",
                    "Эта роль не может создавать документы",
                )
                return
            _fulfill_json(route, _envelope(self._current_projection()), status=201)
            return

        if path == "/api/v1/business-documents/eva/changes":
            _fulfill_json(
                route,
                _envelope({"items": [], "page": 1, "page_size": 20, "total": 0}),
            )
            return

        _fulfill_json(route, _envelope({}))


class EvaBindingChallengeStub(BusinessDocumentsAccessStub):
    def __init__(self):
        super().__init__("AUTHOR_CREATOR")
        self.matches = [
            {
                "connector_id": "connector-ui",
                "connector_name": "EVA Wiki",
                "eva_origin": "https://eva-api.example.test",
                "id": "eva-available",
                "name": "Новый продукт",
                "code": "BR-42",
                "project_id": "project-main",
                "web_url": "https://eva.example.test/main/Document/BR-42",
                "breadcrumbs": [
                    {"id": "folder-main", "name": "Проекты", "web_url": "https://eva.example.test/main/Document/PROJECTS"},
                    {"id": "eva-available", "name": "Новый продукт", "web_url": "https://eva.example.test/main/Document/BR-42"},
                ],
                "hierarchy": "Проекты › Новый продукт",
                "binding_available": True,
            },
            {
                "connector_id": "connector-ui",
                "connector_name": "EVA Wiki",
                "eva_origin": "https://eva-api.example.test",
                "id": "eva-occupied",
                "name": "Новый продукт",
                "code": "BR-99",
                "project_id": "project-archive",
                "web_url": "https://eva.example.test/archive/Document/BR-99",
                "breadcrumbs": [
                    {"id": "folder-archive", "name": "Архив", "web_url": "https://eva.example.test/archive/Document/ARCHIVE"},
                    {"id": "eva-occupied", "name": "Новый продукт", "web_url": "https://eva.example.test/archive/Document/BR-99"},
                ],
                "hierarchy": "Архив › Новый продукт",
                "binding_available": False,
                "linked_document": {"document_id": "doc-existing", "title": "Занятые требования"},
            },
        ]

    def __call__(self, route):
        request = route.request
        path = urlparse(request.url).path.rstrip("/")
        if path == "/api/v1/business-documents" and request.method == "POST":
            payload = request.post_data_json
            self.mutations.append((request.method, path, payload))
            if payload.get("eva_decision") is None:
                _fulfill_json(
                    route,
                    {
                        "code": 409,
                        "message": "Выберите страницу EVA",
                        "data": {
                            "error_code": "EVA_BINDING_DECISION_REQUIRED",
                            "details": {"matches": self.matches},
                        },
                    },
                    status=409,
                )
                return
            _fulfill_json(route, _envelope(self._current_projection()), status=201)
            return
        super().__call__(route)


class AdminDocumentRoleStub:
    def __init__(self):
        self.role = "AUTHOR_EDITOR"
        self.patches = []

    def __call__(self, route):
        request = route.request
        path = urlparse(request.url).path.rstrip("/")

        if path == "/api/v1/system/config":
            _fulfill_json(
                route,
                _envelope(
                    {
                        "registerEnabled": 0,
                        "disablePasswordLogin": False,
                        "visibleSections": ALL_SECTIONS,
                    }
                ),
            )
            return
        if path == "/api/v1/admin/version":
            _fulfill_json(route, _envelope({"version": "browser-test"}))
            return
        if path == "/api/v1/admin/roles":
            _fulfill_json(route, _envelope({"roles": []}))
            return
        if path == "/api/v1/admin/users":
            _fulfill_json(
                route,
                _envelope(
                    [
                        {
                            "id": ACTOR_ID,
                            "email": "browser-viewer@example.test",
                            "nickname": "Browser Admin",
                            "create_date": "2026-09-04",
                            "is_active": "1",
                            "is_superuser": True,
                            "role": "admin",
                            "business_document_role": "AUTHOR_CREATOR",
                        },
                        {
                            "id": "editor-1",
                            "email": "editor@example.test",
                            "nickname": "Editor User",
                            "create_date": "2026-09-04",
                            "is_active": "1",
                            "is_superuser": False,
                            "role": "user",
                            "business_document_role": self.role,
                        },
                    ]
                ),
            )
            return
        if path == "/api/v1/business-documents/access/users/editor-1":
            payload = request.post_data_json
            self.patches.append(payload)
            self.role = payload["role"]
            _fulfill_json(
                route,
                _envelope(
                    {
                        "user_id": "editor-1",
                        "nickname": "Editor User",
                        "role": self.role,
                    }
                ),
            )
            return
        if path in {"/api/v1/users/me", "/api/v1/tenants"}:
            _fulfill_json(route, _envelope([]))
            return
        _fulfill_json(route, _envelope({}))


def _open_documents(page, base_url, stub):
    _install_session(page, is_superuser=stub.role == "ADMIN")
    page.route("**/api/v1/**", stub)
    page.goto(f"{base_url.rstrip('/')}/business-documents")
    expect(page.get_by_test_id("business-documents-create")).to_be_visible(timeout=RESULT_TIMEOUT_MS)


def _open_document(page, base_url, stub):
    _install_session(page, is_superuser=stub.role == "ADMIN")
    page.route("**/api/v1/**", stub)
    page.goto(f"{base_url.rstrip('/')}/business-documents/{DOCUMENT_ID}")
    expect(page.get_by_test_id("business-document-workbench")).to_be_visible(timeout=RESULT_TIMEOUT_MS)


@pytest.mark.p1
@pytest.mark.auth
@pytest.mark.parametrize(
    ("role", "can_create", "can_edit_foreign", "can_delete", "can_assign"),
    [
        ("AUTHOR_CREATOR", True, False, False, False),
        ("AUTHOR_EDITOR", False, False, False, False),
        ("MODERATOR_CREATOR", True, True, False, False),
        ("EXTENDED_MODERATOR", True, True, True, True),
        ("ADMIN", True, True, True, True),
    ],
)
def test_document_role_matrix_controls_browser_actions(
    page,
    base_url,
    role,
    can_create,
    can_edit_foreign,
    can_delete,
    can_assign,
):
    stub = BusinessDocumentsAccessStub(role)
    _open_documents(page, base_url, stub)

    expect(page.get_by_test_id("new-document-mode")).to_be_enabled(enabled=can_create)
    expect(page.get_by_test_id("business-document-create-denied")).to_have_count(0 if can_create else 1)

    page.goto(f"{base_url.rstrip('/')}/business-documents/{DOCUMENT_ID}")
    expect(page.get_by_test_id("business-document-workbench")).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    option = page.get_by_role("radio", name="По времени")
    expect(option).to_be_enabled(enabled=can_edit_foreign)
    if can_edit_foreign:
        option.check()
    expect(page.get_by_test_id("answer-question-question-access-1")).to_be_enabled(enabled=can_edit_foreign)
    expect(page.get_by_test_id("business-document-delete-detail")).to_have_count(1 if can_delete else 0)
    expect(page.get_by_test_id("business-document-owner-select")).to_have_count(1 if can_assign else 0)


@pytest.mark.p1
@pytest.mark.auth
def test_document_filters_request_mine_then_all_and_render_their_scopes(page, base_url):
    stub = BusinessDocumentsAccessStub("AUTHOR_CREATOR")
    _open_documents(page, base_url, stub)

    expect(page.get_by_text("Мой документ", exact=True)).to_be_visible()
    expect(page.get_by_text("Чужой документ", exact=True)).to_have_count(0)
    page.get_by_test_id("business-documents-filter-all").click()
    expect(page.get_by_text("Чужой документ", exact=True)).to_be_visible()
    assert stub.list_scopes[:2] == ["mine", "all"]


@pytest.mark.p1
@pytest.mark.auth
def test_read_only_author_cannot_issue_mutations_even_if_server_advertises_commands(page, base_url):
    stub = BusinessDocumentsAccessStub("AUTHOR_CREATOR")
    _open_document(page, base_url, stub)

    expect(page.get_by_text("Сократить время обработки до одной минуты.")).to_be_visible()
    expect(page.get_by_role("radio", name="По времени")).to_be_disabled()
    expect(page.get_by_test_id("answer-question-question-access-1")).to_be_disabled()
    expect(page.get_by_test_id("apply-changes-button")).to_have_count(0)
    expect(page.get_by_test_id("business-document-delete-detail")).to_have_count(0)
    expect(page.get_by_test_id("business-document-owner-select")).to_have_count(0)
    assert stub.mutations == []


@pytest.mark.p1
@pytest.mark.auth
def test_moderator_edits_foreign_document_but_cannot_delete_or_assign(page, base_url):
    stub = BusinessDocumentsAccessStub("MODERATOR_CREATOR")
    _open_document(page, base_url, stub)

    page.get_by_role("radio", name="По времени").check()
    page.get_by_test_id("answer-question-question-access-1").click()
    expect(page.get_by_test_id("business-document-delete-detail")).to_have_count(0)
    expect(page.get_by_test_id("business-document-owner-select")).to_have_count(0)
    assert len(stub.mutations) == 1
    method, path, payload = stub.mutations[0]
    assert method == "POST"
    assert path.endswith("/commands")
    assert payload["type"] == "ANSWER_QUESTION"
    assert payload["expected_state_version"] == 18


@pytest.mark.p1
@pytest.mark.auth
def test_extended_moderator_assigns_foreign_document_to_another_author(page, base_url):
    stub = BusinessDocumentsAccessStub("EXTENDED_MODERATOR")
    _open_document(page, base_url, stub)

    page.get_by_test_id("business-document-owner-select").click()
    page.get_by_test_id("business-document-owner-option-author-2").click()
    page.get_by_test_id("business-document-assign-owner").click()

    expect(page.get_by_text("Владелец: Второй автор", exact=True)).to_be_visible()
    assert stub.mutations == [
        (
            "PUT",
            f"/api/v1/business-documents/{DOCUMENT_ID}/owner",
            {"owner_id": "author-2", "expected_state_version": 18},
        )
    ]


@pytest.mark.p1
@pytest.mark.auth
def test_owner_assignment_conflict_keeps_previous_owner_and_shows_error(page, base_url):
    stub = BusinessDocumentsAccessStub("EXTENDED_MODERATOR", assignment_status=409)
    _open_document(page, base_url, stub)

    page.get_by_test_id("business-document-owner-select").click()
    page.get_by_test_id("business-document-owner-option-author-2").click()
    page.get_by_test_id("business-document-assign-owner").click()

    expect(page.get_by_role("alert")).to_contain_text("Документ уже изменён другим пользователем")
    expect(page.get_by_text("Владелец: Первый автор", exact=True)).to_be_visible()
    assert stub.owner_id == "author-1"


@pytest.mark.p1
@pytest.mark.auth
def test_revision_history_displays_named_and_unavailable_change_authors(page, base_url):
    stub = BusinessDocumentsAccessStub("AUTHOR_CREATOR")
    _open_document(page, base_url, stub)

    page.get_by_test_id("business-document-history-toggle").click()
    history = page.get_by_test_id("business-document-history")
    expect(history).to_be_visible()
    expect(history).to_contain_text("Автор изменений: Мария Авторова (maria@example.test)")
    expect(history).to_contain_text("Автор изменений: Пользователь недоступен")
    expect(history).not_to_contain_text("legacy-author")


@pytest.mark.p1
@pytest.mark.auth
def test_create_permission_error_preserves_form_and_does_not_navigate(page, base_url):
    stub = BusinessDocumentsAccessStub("AUTHOR_CREATOR", create_status=403)
    _open_documents(page, base_url, stub)

    title = page.get_by_label("Название документа")
    idea = page.get_by_label("Описание идеи")
    title.select_option(CATALOG_ENTRY_ID)
    idea.fill("Проверить отрицательный сценарий")
    page.get_by_role("button", name="Начать работу").click()

    expect(page.get_by_role("alert")).to_contain_text("Эта роль не может создавать документы")
    expect(title).to_have_value(CATALOG_ENTRY_ID)
    expect(idea).to_have_value("Проверить отрицательный сценарий")
    expect(page).to_have_url(re.compile(r"/business-documents$"))
    assert stub.mutations[0][0] == "POST"


@pytest.mark.p1
@pytest.mark.auth
def test_eva_title_challenge_requires_page_selection_and_replace_confirmation(page, base_url):
    stub = EvaBindingChallengeStub()
    _open_documents(page, base_url, stub)
    page.get_by_label("Название документа").select_option(CATALOG_ENTRY_ID)
    page.get_by_label("Описание идеи").fill("Создать новый клиентский сценарий")
    page.get_by_role("button", name="Начать работу").click()

    banner = page.get_by_test_id("eva-title-match-banner")
    expect(banner).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    expect(banner).to_contain_text("Занятые требования")
    expect(page.get_by_role("link", name="Проекты")).to_have_attribute("href", stub.matches[0]["breadcrumbs"][0]["web_url"])
    expect(page.get_by_role("link", name="Архив")).to_have_attribute("href", stub.matches[1]["breadcrumbs"][0]["web_url"])
    bind_buttons = page.get_by_role("button", name="Привязать")
    expect(bind_buttons).to_have_count(2)
    expect(bind_buttons.nth(1)).to_be_disabled()

    bind_buttons.nth(0).click()
    confirmation = page.get_by_test_id("eva-replace-confirmation")
    expect(confirmation).to_contain_text("будет полностью заменено содержимым документа")
    page.get_by_role("button", name="Отмена").click()
    expect(confirmation).to_be_hidden()
    assert len(stub.mutations) == 1

    bind_buttons.nth(0).click()
    page.get_by_test_id("confirm-eva-replace").click()
    expect(page).to_have_url(re.compile(f"/business-documents/{DOCUMENT_ID}$"), timeout=RESULT_TIMEOUT_MS)
    create_payloads = [payload for method, path, payload in stub.mutations if method == "POST" and path == "/api/v1/business-documents"]
    assert create_payloads[1]["schema_version"] == "3"
    assert create_payloads[1]["catalog_entry_id"] == CATALOG_ENTRY_ID
    assert create_payloads[1]["eva_page_url"] == stub.matches[0]["web_url"]
    assert create_payloads[1]["eva_decision"] == {"mode": "BIND", "confirm_replace": True}


@pytest.mark.p1
@pytest.mark.auth
def test_extended_moderator_deletes_foreign_document_after_confirmation(page, base_url):
    stub = BusinessDocumentsAccessStub("EXTENDED_MODERATOR")
    _open_document(page, base_url, stub)

    page.get_by_test_id("business-document-delete-detail").click()
    dialog = page.get_by_role("alertdialog")
    expect(dialog).to_contain_text("Удалить документ?")
    dialog.get_by_role("button", name="Удалить", exact=True).click()

    expect(page).to_have_url(re.compile(r"/business-documents$"))
    assert (
        "DELETE",
        f"/api/v1/business-documents/{DOCUMENT_ID}",
        None,
    ) in stub.mutations


@pytest.mark.p1
@pytest.mark.auth
def test_admin_changes_document_role_and_cannot_downgrade_self_in_users_table(page, base_url):
    stub = AdminDocumentRoleStub()
    _install_session(page, is_superuser=True)
    page.route("**/api/v1/**", stub)
    page.goto(f"{base_url.rstrip('/')}/admin/users")

    expect(page.get_by_text("editor@example.test", exact=True)).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    expect(page.get_by_text("Администратор", exact=True)).to_be_visible()
    role_select = page.get_by_label("Роль в документах: editor@example.test")
    role_select.click()
    page.get_by_role("option", name="Модератор-создатель").click()

    expect(role_select).to_contain_text("Модератор-создатель")
    assert stub.patches == [{"role": "MODERATOR_CREATOR"}]


@pytest.mark.p1
@pytest.mark.auth
def test_unauthenticated_document_list_shows_api_denial_without_mutations(page, base_url):
    stub = BusinessDocumentsAccessStub("AUTHOR_CREATOR", list_status=401)
    page.add_init_script("localStorage.setItem('lng', 'ru');")
    page.route("**/api/v1/**", stub)

    page.goto(f"{base_url.rstrip('/')}/business-documents")

    expect(page).to_have_url(re.compile(r"/login(?:\?.*)?$"))
    assert stub.list_scopes == []
    assert stub.document_requests == []
    assert stub.mutations == []
