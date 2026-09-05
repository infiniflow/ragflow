"""Browser auth boundaries against an isolated, intercepted API contract."""

import re
from urllib.parse import urlparse

import pytest
from playwright.sync_api import expect


def _session(page):
    # Seed once: redirects/reloads must not silently restore a removed session.
    page.add_init_script("""
      if (!sessionStorage.getItem('qa-session-seeded')) {
        sessionStorage.setItem('qa-session-seeded', '1');
        localStorage.setItem('Authorization', 'Bearer qa-isolated-session');
        localStorage.setItem('token', 'qa-isolated-session');
        localStorage.setItem('userInfo', JSON.stringify({name: 'QA'}));
      }
      localStorage.setItem('lng', 'en');
    """)


class _AuthApi:
    def __init__(self, *, expired=False, status=401, registration=False):
        self.expired = expired
        self.status = status
        self.registration = registration
        self.logins = 0
        self.logouts = 0
        self.protected_requests = 0

    def __call__(self, route):
        path = urlparse(route.request.url).path
        data = {}
        code = 0
        message = ""
        status = 200
        protected = path in {"/api/v1/users/me", "/api/v1/files", "/api/v1/tenants"}
        if protected:
            self.protected_requests += 1
        if protected and self.expired:
            code, status, message = 401, self.status, "Session expired"
        elif path == "/api/v1/system/config":
            data = {"registerEnabled": int(self.registration), "disablePasswordLogin": False, "visibleSections": ["file_manager", "dataset"]}
        elif path == "/api/v1/auth/login/channels":
            data = []
        elif path == "/api/v1/auth/login":
            assert route.request.method == "POST"
            assert route.request.post_data_json["email"] == "qa@example.test"
            assert route.request.post_data_json["password"]
            self.logins += 1
            code, message = 100, "Invalid email or password"
        elif path == "/api/v1/auth/logout":
            assert route.request.method == "POST"
            self.logouts += 1
            self.expired = True
        elif path == "/api/v1/users/me":
            data = {"id": "qa", "nickname": "QA", "email": "qa@example.test", "language": "en", "avatar": None}
        elif path == "/api/v1/files":
            data = {"files": [], "total": 0, "parent_folder": {}}
        elif path == "/api/v1/tenants":
            data = []
        elif path == "/api/v1/system/version":
            data = "qa-browser"
        route.fulfill(status=status, json={"code": code, "data": data, "message": message})


def _assert_logged_out(page):
    expect(page).to_have_url(re.compile(r"/login(?:\?.*)?$"), timeout=15000)
    expect(page.locator("form[data-active='true']")).to_be_visible(timeout=15000)
    assert page.evaluate("['Authorization', 'token', 'userInfo'].every(k => localStorage.getItem(k) === null)")


def test_wrong_password_does_not_create_session(page, base_url):
    api = _AuthApi()
    page.route("**/api/**", api)
    page.goto(f"{base_url}/login")
    form = page.locator("form[data-active='true']")
    form.get_by_test_id("auth-email").fill("qa@example.test")
    form.get_by_test_id("auth-password").fill("invalid-qa-password")
    form.get_by_test_id("auth-submit").click()
    expect(page.get_by_text("Invalid email or password", exact=True)).to_be_visible()
    expect(form.get_by_test_id("auth-submit")).to_be_enabled()
    assert api.logins == 1
    _assert_logged_out(page)


@pytest.mark.parametrize("http_status", [200, 401], ids=["envelope-401", "http-401"])
def test_expired_session_clears_credentials_and_redirects(page, base_url, http_status):
    _session(page)
    api = _AuthApi(expired=True, status=http_status)
    page.route("**/api/**", api)
    page.goto(f"{base_url}/files")
    _assert_logged_out(page)
    assert api.protected_requests >= 1
    expect(page.get_by_test_id("files-list")).to_have_count(0)


def test_logout_prevents_return_to_protected_page(page, base_url):
    _session(page)
    api = _AuthApi()
    page.route("**/api/**", api)
    page.goto(f"{base_url}/user-setting/profile")
    page.get_by_role("button", name=re.compile("log out", re.I)).click()
    _assert_logged_out(page)
    assert api.logouts == 1
    page.goto(f"{base_url}/files")
    _assert_logged_out(page)
    expect(page.get_by_test_id("files-list")).to_have_count(0)


def test_auth_card_toggle_keeps_inactive_face_out_of_keyboard_and_pointer_input(page, base_url):
    api = _AuthApi(registration=True)
    page.route("**/api/**", api)
    page.goto(f"{base_url}/login")
    active = page.get_by_test_id("auth-card-active")
    active.get_by_test_id("auth-toggle-register").click()
    expect(active.get_by_test_id("auth-nickname")).to_be_visible()
    expect(active).not_to_have_attribute("inert", "")
    inactive = page.locator("[inert][aria-hidden='true']")
    expect(inactive).to_have_count(1)
    expect(inactive).to_have_css("pointer-events", "none")
    active.get_by_test_id("auth-email").focus()
    for _ in range(6):
        page.keyboard.press("Tab")
        assert page.evaluate("document.activeElement.closest('[inert]') === null")
    active.get_by_test_id("auth-toggle-login").click()
    expect(active.get_by_test_id("auth-nickname")).to_have_count(0)
    expect(inactive).to_have_count(1)
    expect(inactive).to_have_css("pointer-events", "none")
    form = page.locator("form[data-active='true']")
    form.get_by_test_id("auth-email").fill("qa@example.test")
    form.get_by_test_id("auth-password").fill("invalid-qa-password")
    form.get_by_test_id("auth-submit").click()
    expect(page.get_by_text("Invalid email or password", exact=True)).to_be_visible()
    assert api.logins == 1
