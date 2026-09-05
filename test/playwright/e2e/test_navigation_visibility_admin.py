import json
import re
from urllib.parse import urlparse

import pytest
from playwright.sync_api import expect

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


def _install_admin_session(page):
    page.add_init_script(
        """
        localStorage.setItem('Authorization', 'Bearer browser-test-admin');
        localStorage.setItem('token', 'browser-test-admin');
        localStorage.setItem('userInfo', JSON.stringify({
          access_token: 'browser-test-admin',
          email: 'browser-admin@example.test',
          nickname: 'Browser Admin'
        }));
        localStorage.setItem('lng', 'ru');
        """
    )


def _install_russian_locale(page):
    page.add_init_script("localStorage.setItem('lng', 'ru');")


class NavigationApiStub:
    def __init__(
        self,
        visible_sections=None,
        *,
        admin_get_status=200,
        admin_put_status=200,
        system_visible_sections=None,
    ):
        initial_sections = ALL_SECTIONS if visible_sections is None else visible_sections
        self.visible_sections = list(initial_sections)
        self.admin_get_status = admin_get_status
        self.admin_put_status = admin_put_status
        self.system_visible_sections = system_visible_sections
        self.put_payloads = []
        self.admin_navigation_requests = []

    def __call__(self, route):
        request = route.request
        path = urlparse(request.url).path.rstrip("/")

        if path == "/api/v1/admin/navigation":
            self.admin_navigation_requests.append(request.method)
            if request.method == "GET":
                if self.admin_get_status != 200:
                    _fulfill_json(
                        route,
                        _envelope(
                            None,
                            code=self.admin_get_status,
                            message="Navigation settings unavailable",
                        ),
                        status=self.admin_get_status,
                    )
                    return
                _fulfill_json(
                    route,
                    _envelope({"visible_sections": self.visible_sections}),
                )
                return

            payload = request.post_data_json
            self.put_payloads.append(payload)
            if self.admin_put_status != 200:
                _fulfill_json(
                    route,
                    _envelope(
                        None,
                        code=self.admin_put_status,
                        message="Navigation settings rejected",
                    ),
                    status=self.admin_put_status,
                )
                return

            self.visible_sections = list(payload["visible_sections"])
            _fulfill_json(
                route,
                _envelope({"visible_sections": self.visible_sections}),
            )
            return

        if path == "/api/v1/admin/version":
            _fulfill_json(route, _envelope({"version": "browser-test"}))
            return

        if path == "/api/v1/system/config":
            visible_sections = self.visible_sections if self.system_visible_sections is None else self.system_visible_sections
            _fulfill_json(
                route,
                _envelope(
                    {
                        "registerEnabled": 0,
                        "disablePasswordLogin": False,
                        "visibleSections": visible_sections,
                    }
                ),
            )
            return

        if path == "/api/v1/users/me":
            _fulfill_json(
                route,
                _envelope(
                    {
                        "id": "browser-user",
                        "email": "browser-user@example.test",
                        "nickname": "Browser User",
                        "language": "ru",
                        "avatar": None,
                    }
                ),
            )
            return

        if path == "/api/v1/tenants":
            _fulfill_json(route, _envelope([]))
            return

        if path == "/api/v1/business-documents":
            _fulfill_json(
                route,
                _envelope(
                    {
                        "items": [],
                        "page": 1,
                        "page_size": 20,
                        "total": 0,
                    }
                ),
            )
            return

        if path == "/api/v1/datasets":
            _fulfill_json(route, _envelope([]))
            return

        _fulfill_json(route, _envelope({}))


def _open_admin_visibility(page, base_url, stub):
    _install_admin_session(page)
    page.route("**/api/v1/**", stub)
    page.goto(f"{base_url.rstrip('/')}/admin/navigation")
    expect(page.get_by_test_id("navigation-visibility-admin")).to_be_visible()


def _open_business_documents(page, base_url):
    page.set_viewport_size({"width": 1920, "height": 1080})
    page.goto(f"{base_url.rstrip('/')}/business-documents")
    expect(page.locator("[data-testid='nav-chat']:visible")).to_be_visible()


@pytest.mark.p1
@pytest.mark.auth
def test_admin_selective_visibility_save_updates_desktop_navigation(page, base_url):
    stub = NavigationApiStub()
    _open_admin_visibility(page, base_url, stub)

    panel = page.get_by_test_id("navigation-visibility-admin")
    switches = panel.get_by_role("switch")
    expect(switches).to_have_count(len(ALL_SECTIONS))
    for section in ALL_SECTIONS:
        expect(page.get_by_test_id(f"navigation-section-{section}")).to_be_checked()

    page.get_by_test_id("navigation-section-home").click()
    page.get_by_test_id("navigation-section-search").click()
    page.get_by_test_id("navigation-section-business_documents").click()

    save = page.get_by_test_id("navigation-visibility-save")
    expect(save).to_be_enabled()
    with page.expect_request(lambda request: request.method == "PUT" and urlparse(request.url).path.rstrip("/") == "/api/v1/admin/navigation"):
        save.click()

    expected = [section for section in ALL_SECTIONS if section not in {"home", "search", "business_documents"}]
    assert stub.put_payloads == [{"visible_sections": expected}]
    expect(save).to_be_disabled()

    _open_business_documents(page, base_url)
    expect(page.locator("[data-testid='nav-home']")).to_have_count(0)
    expect(page.locator("[data-testid='nav-search']")).to_have_count(0)
    expect(page.locator("[data-testid='nav-business-documents']")).to_have_count(0)
    expect(page.locator("[data-testid='nav-agent']:visible")).to_be_visible()


@pytest.mark.p1
@pytest.mark.auth
def test_admin_hide_all_and_show_all_use_canonical_payloads(page, base_url):
    stub = NavigationApiStub()
    _open_admin_visibility(page, base_url, stub)

    page.get_by_test_id("navigation-visibility-hide-all").click()
    panel = page.get_by_test_id("navigation-visibility-admin")
    unchecked = panel.locator("[role='switch'][data-state='unchecked']")
    expect(unchecked).to_have_count(len(ALL_SECTIONS))
    page.get_by_test_id("navigation-visibility-save").click()
    expect(page.get_by_test_id("navigation-visibility-save")).to_be_disabled()

    page.get_by_test_id("navigation-visibility-show-all").click()
    checked = panel.locator("[role='switch'][data-state='checked']")
    expect(checked).to_have_count(len(ALL_SECTIONS))
    page.get_by_test_id("navigation-visibility-save").click()
    expect(page.get_by_test_id("navigation-visibility-save")).to_be_disabled()

    assert stub.put_payloads == [
        {"visible_sections": []},
        {"visible_sections": ALL_SECTIONS},
    ]


@pytest.mark.p1
@pytest.mark.auth
def test_mobile_navigation_shows_only_configured_sections(page, base_url):
    stub = NavigationApiStub(visible_sections=["chat", "memory"])
    _install_admin_session(page)
    page.route("**/api/v1/**", stub)
    page.set_viewport_size({"width": 390, "height": 844})
    page.goto(f"{base_url.rstrip('/')}/business-documents")

    page.get_by_role("button", name="Menu").click()
    expect(page.get_by_role("link", name="Чат", exact=True)).to_be_visible()
    expect(page.get_by_role("link", name="Память", exact=True)).to_be_visible()
    expect(page.get_by_role("link", name="Главная", exact=True)).to_have_count(0)
    expect(page.get_by_role("link", name="Поиск", exact=True)).to_have_count(0)
    expect(page.get_by_role("link", name="Документы", exact=True)).to_have_count(0)


@pytest.mark.p1
@pytest.mark.auth
def test_unauthenticated_admin_route_redirects_without_loading_settings(page, base_url):
    stub = NavigationApiStub()
    _install_russian_locale(page)
    page.route("**/api/v1/**", stub)

    page.goto(f"{base_url.rstrip('/')}/admin/navigation")

    expect(page).to_have_url(re.compile(r"/admin/?$"))
    expect(page.get_by_role("heading", name="Административная консоль")).to_be_visible()
    assert stub.admin_navigation_requests == []


@pytest.mark.p2
@pytest.mark.auth
def test_admin_settings_load_failure_cannot_be_saved(page, base_url):
    stub = NavigationApiStub(admin_get_status=500)
    _open_admin_visibility(page, base_url, stub)

    save = page.get_by_test_id("navigation-visibility-save")
    expect(save).to_be_disabled()
    page.get_by_test_id("navigation-section-search").click()
    expect(save).to_be_disabled()
    assert stub.put_payloads == []


@pytest.mark.p2
@pytest.mark.auth
def test_failed_update_keeps_server_navigation_and_allows_retry(page, base_url):
    stub = NavigationApiStub(admin_put_status=500)
    _open_admin_visibility(page, base_url, stub)

    page.get_by_test_id("navigation-section-search").click()
    save = page.get_by_test_id("navigation-visibility-save")
    save.click()

    expect(save).to_be_enabled()
    assert len(stub.put_payloads) == 1
    assert "search" not in stub.put_payloads[0]["visible_sections"]
    assert "search" in stub.visible_sections

    _open_business_documents(page, base_url)
    expect(page.locator("[data-testid='nav-search']:visible")).to_be_visible()


@pytest.mark.p2
@pytest.mark.auth
def test_malformed_public_visibility_config_falls_back_to_all_sections(page, base_url):
    stub = NavigationApiStub(system_visible_sections="not-an-array")
    _install_admin_session(page)
    page.route("**/api/v1/**", stub)

    _open_business_documents(page, base_url)

    expect(page.locator("[data-testid='nav-search']:visible")).to_be_visible()
    expect(page.locator("[data-testid='nav-business-documents']:visible")).to_be_visible()
    expect(page.locator("[data-testid='nav-agent']:visible")).to_be_visible()
