"""Browser regression for centrally managed user settings.

The suite uses a real Chromium page with an isolated API contract. It verifies
navigation, direct-route protection, the two permitted user settings surfaces,
and admin visibility without reading or mutating live credentials.
"""

import json
import re
from urllib.parse import urlparse

import pytest
from playwright.sync_api import expect


HIDDEN_SETTINGS = {
    "data-sources": "/user-setting/data-source",
    "chat-channels": "/user-setting/chat-channel",
    "mcp": "/user-setting/mcp",
    "team": "/user-setting/team",
    "api": "/user-setting/api",
}
FORBIDDEN_API_PREFIXES = (
    "/api/v1/connectors",
    "/api/v1/chat-channels",
    "/api/v1/mcp/servers",
    "/api/v1/system/tokens",
)


def _envelope(data, *, code=0, message=""):
    return {"code": code, "data": data, "message": message}


def _install_session(page, *, is_superuser=False):
    user_info = json.dumps(
        {
            "id": "settings-user",
            "email": "settings-user@example.test",
            "nickname": "Settings User",
            "is_superuser": is_superuser,
        }
    )
    page.add_init_script(
        f"""
        (() => {{
          const userInfo = {user_info};
          localStorage.setItem('Authorization', 'Bearer settings-browser-test');
          localStorage.setItem('token', 'settings-browser-test');
          localStorage.setItem('userInfo', JSON.stringify(userInfo));
          localStorage.setItem('lng', 'ru');
        }})()
        """
    )


class ManagedSettingsApiStub:
    def __init__(self, *, is_superuser=False, authenticated=True):
        self.is_superuser = is_superuser
        self.authenticated = authenticated
        self.paths = []
        self.model_updates = []
        self.eva_mutations = []
        self.eva_configured = False
        self.default_model = "chat-primary"

    def _user(self):
        return {
            "id": "settings-user",
            "email": "settings-user@example.test",
            "nickname": "Settings User",
            "language": "ru",
            "timezone": "Europe/Moscow",
            "avatar": None,
            "password": "",
            "is_superuser": self.is_superuser,
        }

    def __call__(self, route):
        request = route.request
        path = urlparse(request.url).path.rstrip("/")
        path = path or "/"
        self.paths.append((request.method, path))

        public_paths = {
            "/api/v1/system/config",
            "/api/v1/auth/login/channels",
        }
        if not self.authenticated and path not in public_paths:
            route.fulfill(
                status=401,
                json=_envelope(None, code=401, message="Session expired"),
            )
            return

        if path == "/api/v1/system/config":
            route.fulfill(
                json=_envelope(
                    {
                        "registerEnabled": 0,
                        "disablePasswordLogin": False,
                        "visibleSections": ["home", "dataset", "chat"],
                    }
                )
            )
            return
        if path == "/api/v1/auth/login/channels":
            route.fulfill(json=_envelope([]))
            return
        if path == "/api/v1/users/me":
            route.fulfill(json=_envelope(self._user()))
            return
        if path == "/api/v1/system/version":
            route.fulfill(json=_envelope("settings-browser-test"))
            return
        if path == "/api/v1/users/me/models":
            route.fulfill(
                json=_envelope(
                    {
                        "tenant_id": "settings-user",
                        "llm_id": "chat-primary@default@OpenAI",
                        "embd_id": "embedding-central@default@OpenAI",
                        "parser_ids": "naive",
                    }
                )
            )
            return
        if path == "/api/v1/users/me/eva-credentials":
            route.fulfill(
                json=_envelope(
                    {
                        "items": [
                            {
                                "connector_id": "eva-1",
                                "scope": "EVA QA",
                                "configured": self.eva_configured,
                            }
                        ]
                    }
                )
            )
            return
        if path == "/api/v1/users/me/eva-credentials/eva-1":
            if request.method == "PUT":
                payload = request.post_data_json
                self.eva_mutations.append(("PUT", len(payload.get("eva_api_token", ""))))
                self.eva_configured = True
            elif request.method == "DELETE":
                self.eva_mutations.append(("DELETE", 0))
                self.eva_configured = False
            route.fulfill(json=_envelope({"configured": self.eva_configured}))
            return
        if path == "/api/v1/models":
            route.fulfill(
                json=_envelope(
                    [
                        {
                            "model_type": ["chat"],
                            "name": "chat-primary",
                            "provider_id": "provider-1",
                            "provider_name": "OpenAI",
                            "instance_id": "instance-1",
                            "instance_name": "default",
                        },
                        {
                            "model_type": ["chat"],
                            "name": "chat-secondary",
                            "provider_id": "provider-1",
                            "provider_name": "OpenAI",
                            "instance_id": "instance-1",
                            "instance_name": "default",
                        },
                        {
                            "model_type": ["embedding"],
                            "name": "embedding-central",
                            "provider_id": "provider-1",
                            "provider_name": "OpenAI",
                            "instance_id": "instance-1",
                            "instance_name": "default",
                        },
                    ]
                )
            )
            return
        if path == "/api/v1/models/default":
            if request.method == "PATCH":
                payload = request.post_data_json
                self.model_updates.append(payload)
                self.default_model = payload["model_name"]
            route.fulfill(
                json=_envelope(
                    {
                        "models": [
                            {
                                "model_type": "chat",
                                "model_name": self.default_model,
                                "model_instance": "default",
                                "model_provider": "OpenAI",
                                "enable": True,
                            }
                        ]
                    }
                )
            )
            return
        if path == "/api/v1/providers":
            route.fulfill(json=_envelope([]))
            return
        if path == "/api/v1/connectors":
            route.fulfill(json=_envelope([]))
            return
        if path == "/api/v1/tenants":
            route.fulfill(json=_envelope([]))
            return

        route.fulfill(json=_envelope({}))


def _open_settings(page, base_url, stub, path, *, session=True):
    if session:
        _install_session(page, is_superuser=stub.is_superuser)
    else:
        page.add_init_script("localStorage.setItem('lng', 'ru');")
    page.route("**/api/v1/**", stub)
    page.goto(f"{base_url.rstrip('/')}{path}")


@pytest.mark.p1
@pytest.mark.auth
def test_regular_user_sees_only_profile_and_model_settings(page, base_url):
    stub = ManagedSettingsApiStub()
    _open_settings(page, base_url, stub, "/user-setting/profile")

    expect(page.get_by_test_id("settings-eva-profile")).to_be_visible(timeout=15000)
    expect(page.get_by_test_id("settings-nav-profile")).to_be_visible()
    expect(page.get_by_test_id("settings-nav-model-providers")).to_be_visible()
    for key in HIDDEN_SETTINGS:
        expect(page.get_by_test_id(f"settings-nav-{key}")).to_have_count(0)


@pytest.mark.p1
@pytest.mark.auth
@pytest.mark.parametrize(
    "path",
    [
        "/user-setting",
        *HIDDEN_SETTINGS.values(),
        "/user-setting/data-source/data-source-detail-page?id=connector-1",
        "/user-setting/profile-extra",
        "/user-setting/model/private",
    ],
)
def test_regular_user_direct_hidden_settings_routes_redirect_to_profile(
    page,
    base_url,
    path,
):
    stub = ManagedSettingsApiStub()
    _open_settings(page, base_url, stub, path)

    expect(page).to_have_url(
        re.compile(r"/user-setting/profile/?$"),
        timeout=15000,
    )
    expect(page.get_by_test_id("settings-eva-profile")).to_be_visible()
    requested_paths = [request_path for _, request_path in stub.paths]
    assert not any(request_path.startswith(prefix) for request_path in requested_paths for prefix in FORBIDDEN_API_PREFIXES)


@pytest.mark.p1
@pytest.mark.auth
def test_regular_user_can_add_and_remove_personal_eva_token_at_length_boundary(
    page,
    base_url,
):
    stub = ManagedSettingsApiStub()
    _open_settings(page, base_url, stub, "/user-setting/profile")

    page.get_by_test_id("eva-token-edit-eva-1").click()
    expect(page.get_by_test_id("eva-token-edit-modal")).to_be_visible()
    save = page.get_by_test_id("eva-token-save")
    expect(save).to_be_disabled()

    token_input = page.get_by_test_id("eva-token-input")
    token_input.fill("x" * 4097)
    expect(token_input).to_have_value("x" * 4096)
    save.click()

    expect(page.get_by_test_id("eva-token-delete-eva-1")).to_be_visible()
    assert stub.eva_mutations == [("PUT", 4096)]

    page.get_by_test_id("eva-token-delete-eva-1").click()
    expect(page.get_by_test_id("eva-token-delete-modal")).to_be_visible()
    page.get_by_test_id("eva-token-delete-confirm").click()
    expect(page.get_by_test_id("eva-token-edit-eva-1")).to_be_visible()
    assert stub.eva_mutations == [("PUT", 4096), ("DELETE", 0)]


@pytest.mark.p1
@pytest.mark.auth
def test_regular_user_can_select_only_personal_chat_default(page, base_url):
    stub = ManagedSettingsApiStub()
    _open_settings(page, base_url, stub, "/user-setting/model")

    panel = page.get_by_test_id("user-default-model-settings")
    expect(panel).to_be_visible()
    expect(panel.locator("[data-testid^='settings-default-']")).to_have_count(1)
    expect(page.get_by_test_id("settings-default-llm_id")).to_be_visible()
    expect(page.get_by_test_id("settings-default-embd_id")).to_have_count(0)

    page.get_by_test_id("settings-default-llm_id").click()
    page.get_by_text("chat-secondary", exact=True).click()

    expect(page.get_by_test_id("settings-default-llm_id")).to_contain_text("chat-secondary")
    assert stub.model_updates == [
        {
            "model_provider": "OpenAI",
            "model_instance": "default",
            "model_name": "chat-secondary",
            "model_type": "chat",
        }
    ]


@pytest.mark.p1
@pytest.mark.auth
def test_superuser_keeps_all_settings_tabs(page, base_url):
    stub = ManagedSettingsApiStub(is_superuser=True)
    _open_settings(page, base_url, stub, "/user-setting/profile")

    for key in HIDDEN_SETTINGS:
        expect(page.get_by_test_id(f"settings-nav-{key}")).to_be_visible()
    expect(page.get_by_test_id("settings-nav-profile")).to_be_visible()
    expect(page.get_by_test_id("settings-nav-model-providers")).to_be_visible()


@pytest.mark.p1
@pytest.mark.auth
def test_unauthenticated_direct_settings_route_redirects_to_login(page, base_url):
    stub = ManagedSettingsApiStub(authenticated=False)
    _open_settings(
        page,
        base_url,
        stub,
        "/user-setting/model",
        session=False,
    )

    expect(page).to_have_url(re.compile(r"/login(?:\?.*)?$"), timeout=15000)
    expect(page.locator("form[data-active='true']")).to_be_visible(timeout=15000)
