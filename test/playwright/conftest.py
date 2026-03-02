import sys
from pathlib import Path
_PW_DIR = Path(__file__).resolve().parent
if str(_PW_DIR) not in sys.path:
    sys.path.insert(0, str(_PW_DIR))

import base64
import faulthandler
import json
import os
import re
import secrets
import signal
import time
from contextlib import contextmanager
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.parse import urljoin
from urllib.request import Request, urlopen

import pytest
from playwright.sync_api import TimeoutError as PlaywrightTimeoutError
from playwright.sync_api import expect, sync_playwright

ROOT_DIR = Path(__file__).resolve().parents[2]
PLAYWRIGHT_TEST_DIR = Path(__file__).resolve().parent
ARTIFACTS_DIR = Path(__file__).resolve().parent / "artifacts"
BASE_URL_DEFAULT = "http://127.0.0.1"
LOGIN_PATH_DEFAULT = "/login"
DEFAULT_TIMEOUT_MS = 30000
DEFAULT_HANG_TIMEOUT_S = 1800
AUTH_READY_TIMEOUT_MS_DEFAULT = 15000
REG_EMAIL_BASE_DEFAULT = "qa@infiniflow.org"
REG_NICKNAME_DEFAULT = "qa"
REG_PASSWORD_DEFAULT = "123"
REG_EMAIL_LOCAL_RE = re.compile(r"^[A-Za-z0-9_.-]+$")
REG_EMAIL_BACKEND_RE = re.compile(r"^[\w\._-]{1,}@([\w_-]+\.)+[\w-]{2,}$")
AUTH_FORM_SELECTOR = "form[data-testid='auth-form']"
AUTH_ACTIVE_FORM_SELECTOR = "form[data-testid='auth-form'][data-active='true']"
AUTH_EMAIL_INPUT_SELECTOR = (
    "input[data-testid='auth-email'], [data-testid='auth-email'] input"
)
AUTH_PASSWORD_INPUT_SELECTOR = (
    "input[data-testid='auth-password'], [data-testid='auth-password'] input"
)
AUTH_SUBMIT_SELECTOR = (
    "button[data-testid='auth-submit'], [data-testid='auth-submit'] button, [data-testid='auth-submit']"
)

_PUBLIC_KEY_CACHE = None
_RSA_CIPHER_CACHE = None
_HANG_WATCHDOG_INSTALLED = False


class _RegisterDisabled(RuntimeError):
    pass


def _env_bool(name: str, default: bool = False) -> bool:
    value = os.getenv(name)
    if value is None:
        return default
    return value.strip().lower() in {"1", "true", "yes", "on"}


def _env_int(name: str, default: int) -> int:
    value = os.getenv(name)
    if not value:
        return default
    try:
        return int(value)
    except ValueError:
        return default


def _env_int_with_fallback(primary: str, fallback: str | None, default: int) -> int:
    value = os.getenv(primary)
    if not value and fallback:
        value = os.getenv(fallback)
    if not value:
        return default
    try:
        return int(value)
    except ValueError:
        return default


def _sanitize_timeout_ms(value: int | None, fallback: int | None) -> int | None:
    if value is None or value <= 0:
        return fallback
    return value


def _playwright_action_timeout_ms() -> int | None:
    raw = _env_int_with_fallback(
        "PLAYWRIGHT_ACTION_TIMEOUT_MS", "PW_TIMEOUT_MS", DEFAULT_TIMEOUT_MS
    )
    return _sanitize_timeout_ms(raw, DEFAULT_TIMEOUT_MS)


def _playwright_auth_ready_timeout_ms() -> int | None:
    raw = _env_int_with_fallback(
        "PLAYWRIGHT_AUTH_READY_TIMEOUT_MS",
        "AUTH_READY_TIMEOUT_MS",
        AUTH_READY_TIMEOUT_MS_DEFAULT,
    )
    return _sanitize_timeout_ms(raw, AUTH_READY_TIMEOUT_MS_DEFAULT)


def _playwright_hang_timeout_s() -> int:
    raw = _env_int_with_fallback(
        "PLAYWRIGHT_HANG_TIMEOUT_S", "HANG_TIMEOUT_S", DEFAULT_HANG_TIMEOUT_S
    )
    return raw if raw > 0 else 0




def _failure_text(req) -> str:
    failure = getattr(req, "failure", None)
    if callable(failure):
        try:
            failure = failure()
        except Exception:
            return "unknown"
    if failure is None:
        return "unknown"
    if isinstance(failure, str):
        return failure or "unknown"
    try:
        error_text = getattr(failure, "error_text", None)
        if error_text:
            return str(error_text)
    except Exception:
        pass
    try:
        if isinstance(failure, dict):
            for key in ("errorText", "error_text"):
                value = failure.get(key)
                if value:
                    return str(value)
    except Exception:
        pass
    try:
        getter = getattr(failure, "get", None)
        if callable(getter):
            for key in ("errorText", "error_text"):
                value = getter(key)
                if value:
                    return str(value)
    except Exception:
        pass
    try:
        return str(failure)
    except Exception:
        return "unknown"


def _build_url(base_url: str, path: str) -> str:
    if not base_url:
        return path
    base = base_url.rstrip("/") + "/"
    return urljoin(base, path.lstrip("/"))


def _sanitize_filename(value: str) -> str:
    return re.sub(r"[^A-Za-z0-9_.-]+", "_", value).strip("_")


def _request_test_file(request) -> Path | None:
    node = getattr(request, "node", None)
    if node is None:
        return None

    node_path = getattr(node, "path", None)
    if node_path is not None:
        return Path(str(node_path))

    fspath = getattr(node, "fspath", None)
    if fspath is not None:
        return Path(str(fspath))

    nodeid = getattr(node, "nodeid", "")
    if nodeid:
        return Path(nodeid.split("::", 1)[0])

    return None


def _request_artifacts_dir(request) -> Path:
    test_file = _request_test_file(request)
    if test_file is None:
        base_dir = ARTIFACTS_DIR / "unknown"
        base_dir.mkdir(parents=True, exist_ok=True)
        return base_dir

    try:
        rel_path = test_file.resolve().relative_to(PLAYWRIGHT_TEST_DIR.resolve())
        base_dir = ARTIFACTS_DIR / rel_path.with_suffix("")
    except Exception:
        file_stem = _sanitize_filename(test_file.stem or str(test_file))
        base_dir = ARTIFACTS_DIR / (file_stem or "unknown")
    base_dir.mkdir(parents=True, exist_ok=True)
    return base_dir


def _request_artifact_prefix(request) -> str:
    node = getattr(request, "node", None)
    node_name = getattr(node, "name", "") if node is not None else ""
    safe_name = _sanitize_filename(node_name)
    if safe_name:
        return safe_name
    nodeid = getattr(node, "nodeid", "") if node is not None else ""
    fallback = _sanitize_filename(nodeid)
    return fallback or "node"


def _split_email_base(value: str) -> tuple[str, str]:
    if value.count("@") != 1:
        raise ValueError("REG_EMAIL_BASE must be a single email address")
    local, domain = value.split("@", 1)
    if not local or not domain:
        raise ValueError("REG_EMAIL_BASE must include local part and domain")
    return local, domain


def _unique_email(base: str, suffix: str) -> str:
    local, domain = _split_email_base(base)
    if "+" in local:
        local = local.split("+", 1)[0]
    return f"{local}_{suffix}@{domain}"


def _assert_reg_email(email: str) -> None:
    if "+" in email:
        raise AssertionError(f"Registration email contains '+': {email}")
    try:
        local, _ = _split_email_base(email)
    except ValueError as exc:
        raise AssertionError(f"Registration email is invalid: {email}") from exc
    if not REG_EMAIL_LOCAL_RE.match(local):
        raise AssertionError(f"Registration email local part invalid: {email}")
    if not REG_EMAIL_BACKEND_RE.match(email):
        raise AssertionError(f"Registration email fails backend regex: {email}")


def _api_post_json(url: str, payload: dict, timeout_s: int = 10) -> tuple[int, dict | None]:
    data = json.dumps(payload).encode("utf-8")
    req = Request(
        url,
        data=data,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urlopen(req, timeout=timeout_s) as resp:
            body = resp.read()
            if body:
                try:
                    return resp.status, json.loads(body.decode("utf-8"))
                except Exception:
                    return resp.status, None
            return resp.status, None
    except HTTPError as exc:
        body = exc.read()
        parsed = None
        if body:
            try:
                parsed = json.loads(body.decode("utf-8"))
            except Exception:
                parsed = None
        raise RuntimeError(f"HTTPError {exc.code}: {parsed or body!r}") from exc
    except URLError as exc:
        raise RuntimeError(f"URLError: {exc}") from exc


def _rsa_encrypt_password(password: str) -> str:
    global _PUBLIC_KEY_CACHE
    global _RSA_CIPHER_CACHE
    try:
        from Cryptodome.PublicKey import RSA
        from Cryptodome.Cipher import PKCS1_v1_5 as Cipher_pkcs1_v1_5
    except Exception as exc:
        raise RuntimeError(
            "Cryptodome is required to encrypt passwords for API seeding. "
            "Set RAGFLOW_SEEDING_MODE=ui to skip API seeding."
        ) from exc
    if _PUBLIC_KEY_CACHE is None:
        public_key_path = ROOT_DIR / "conf" / "public.pem"
        if not public_key_path.exists():
            raise RuntimeError(f"Missing RSA public key at {public_key_path}")
        _PUBLIC_KEY_CACHE = public_key_path.read_text(encoding="utf-8")
    if _RSA_CIPHER_CACHE is None:
        rsa_key = RSA.importKey(_PUBLIC_KEY_CACHE, "Welcome")
        _RSA_CIPHER_CACHE = Cipher_pkcs1_v1_5.new(rsa_key)
    password_base64 = base64.b64encode(password.encode("utf-8")).decode("utf-8")
    encrypted_password = _RSA_CIPHER_CACHE.encrypt(password_base64.encode("utf-8"))
    return base64.b64encode(encrypted_password).decode("utf-8")


def _is_register_disabled_message(message: str) -> bool:
    lowered = (message or "").lower()
    return "registration is disabled" in lowered or "register disabled" in lowered


def _api_register_user(base_url: str, email: str, password: str, nickname: str) -> None:
    url = _build_url(base_url, "/v1/user/register")
    encrypted_password = _rsa_encrypt_password(password)
    status, payload = _api_post_json(
        url,
        {"email": email, "password": encrypted_password, "nickname": nickname},
        timeout_s=10,
    )
    if status >= 400:
        raise RuntimeError(f"register failed status={status}")
    if isinstance(payload, dict) and payload.get("code") not in (0, None):
        message = str(payload.get("message") or payload)
        if _is_register_disabled_message(message):
            raise _RegisterDisabled(message)
        raise RuntimeError(f"register failed payload={payload}")


def _api_login_user(base_url: str, email: str, password: str) -> None:
    url = _build_url(base_url, "/v1/user/login")
    encrypted_password = _rsa_encrypt_password(password)
    status, payload = _api_post_json(
        url,
        {"email": email, "password": encrypted_password},
        timeout_s=10,
    )
    if status >= 400:
        raise RuntimeError(f"login failed status={status}")
    if isinstance(payload, dict) and payload.get("code") not in (0, None):
        raise RuntimeError(f"login failed payload={payload}")


def _generate_seeded_email(base_email: str) -> str:
    local, domain = _split_email_base(base_email)
    if "+" in local:
        local = local.split("+", 1)[0]
    suffix = f"{int(time.time() * 1000)}_{secrets.token_hex(3)}"
    return f"{local}_{suffix}@{domain}"


def _auth_form_locator(card, require_nickname: bool = False):
    form = card.locator("form[data-testid='auth-form']")
    form = form.filter(has=card.locator("[data-testid='auth-email']"))
    form = form.filter(has=card.locator("[data-testid='auth-submit']"))
    if require_nickname:
        form = form.filter(has=card.locator("[data-testid='auth-nickname']"))
    return form


def _describe_auth_ui(page, card, register_toggle) -> str:
    lines = []
    if card is None:
        lines.append("auth_card_count=unavailable")
    else:
        try:
            lines.append(f"auth_card_count={card.count()}")
        except Exception as exc:
            lines.append(f"auth_card_count_error={exc}")
    if register_toggle is None:
        lines.append("register_toggle_count=unavailable")
    else:
        try:
            toggle_count = register_toggle.count()
            toggle_visible = False
            if toggle_count:
                try:
                    toggle_visible = register_toggle.first.is_visible()
                except Exception:
                    toggle_visible = False
            lines.append(f"register_toggle_count={toggle_count}")
            lines.append(f"register_toggle_visible={toggle_visible}")
        except Exception as exc:
            lines.append(f"register_toggle_error={exc}")
    try:
        summary = _auth_ready_summary(page)
        lines.append(_format_auth_ready_summary(summary).strip())
    except Exception as exc:
        lines.append(f"auth_summary_error={exc}")
    return "\n".join(line for line in lines if line)


def _wait_for_auth_success(page, card, form) -> None:
    timeout_ms = _playwright_auth_ready_timeout_ms()
    status_marker = page.locator("[data-testid='auth-status']")
    if status_marker.count() > 0:
        try:
            expect(status_marker).to_have_attribute(
                "data-state", "success", timeout=timeout_ms
            )
            return
        except AssertionError:
            pass
    try:
        page.wait_for_function(
            "() => Boolean(localStorage.getItem('token') || localStorage.getItem('Authorization'))",
            timeout=timeout_ms,
        )
        return
    except PlaywrightTimeoutError:
        pass
    try:
        expect(card.locator("[data-testid='auth-nickname']")).to_have_count(
            0, timeout=timeout_ms
        )
    except AssertionError as exc:
        raise RuntimeError(
            "Auth success marker not detected after registration."
        ) from exc


def _ui_register_user(
    browser,
    login_url: str,
    email: str,
    password: str,
    nickname: str,
) -> None:
    context_instance = browser.new_context(ignore_https_errors=True)
    page = _configure_page(context_instance.new_page())
    card = None
    register_toggle = None
    try:
        page.goto(login_url, wait_until="domcontentloaded")
        timeout_ms = _playwright_auth_ready_timeout_ms()
        card = page.locator("[data-testid='auth-card-active']")
        expect(card).to_have_count(1, timeout=timeout_ms)
        register_toggle = card.locator("[data-testid='auth-toggle-register']")
        if register_toggle.count() == 0:
            raise _RegisterDisabled("Register toggle not found; registration disabled?")
        register_toggle.first.click()
        register_form = _auth_form_locator(card, require_nickname=True)
        expect(register_form).to_have_count(1, timeout=timeout_ms)
        nickname_input = register_form.locator("[data-testid='auth-nickname']")
        email_input = register_form.locator("[data-testid='auth-email']")
        password_input = register_form.locator("[data-testid='auth-password']")
        expect(nickname_input).to_have_count(1, timeout=timeout_ms)
        expect(email_input).to_have_count(1, timeout=timeout_ms)
        expect(password_input).to_have_count(1, timeout=timeout_ms)
        nickname_input.fill(nickname)
        email_input.fill(email)
        password_input.fill(password)
        password_input.blur()
        submit_button = register_form.locator(AUTH_SUBMIT_SELECTOR)
        expect(submit_button).to_have_count(1, timeout=timeout_ms)
        submit_button.click()
        _wait_for_auth_success(page, card, register_form)
    except _RegisterDisabled:
        raise
    except Exception as _:
        diagnostics = _describe_auth_ui(page, card, register_toggle)
        if diagnostics:
            print(f"[seeded-ui-register] diagnostics:\n{diagnostics}", flush=True)
        raise
    finally:
        try:
            page.close()
        finally:
            context_instance.close()


def _make_reg_email(base: str, unique: bool) -> str:
    if not unique:
        email = base
    else:
        suffix = f"{int(time.time() * 1000)}_{os.getpid()}_{secrets.randbelow(1000000)}"
        email = _unique_email(base, suffix)
    _assert_reg_email(email)
    return email


@contextmanager
def _step(label: str, enabled: bool) -> None:
    start = time.perf_counter()
    if enabled:
        print(f"[STEP] {label}", flush=True)
    try:
        yield
    finally:
        if enabled:
            elapsed = time.perf_counter() - start
            print(f"[STEP] done in {elapsed:.2f}s: {label}", flush=True)


@pytest.hookimpl(hookwrapper=True)
def pytest_runtest_makereport(item, call):
    outcome = yield
    report = outcome.get_result()
    setattr(item, f"_rep_{report.when}", report)


def pytest_sessionstart(session):
    ARTIFACTS_DIR.mkdir(parents=True, exist_ok=True)
    faulthandler.enable()
    global _HANG_WATCHDOG_INSTALLED
    hang_timeout = _playwright_hang_timeout_s()
    if hang_timeout > 0:
        if not _HANG_WATCHDOG_INSTALLED:
            faulthandler.dump_traceback_later(hang_timeout, repeat=True)
            _HANG_WATCHDOG_INSTALLED = True
            print(
                "Playwright hang watchdog enabled: dumps after "
                f"{hang_timeout}s (set PLAYWRIGHT_HANG_TIMEOUT_S=0 to disable)",
                flush=True,
            )
    else:
        print(
            "Playwright hang watchdog disabled (PLAYWRIGHT_HANG_TIMEOUT_S=0)",
            flush=True,
        )
    try:
        faulthandler.register(signal.SIGUSR1, all_threads=True)
    except (AttributeError, ValueError):
        pass


def pytest_sessionfinish(session, exitstatus):
    try:
        faulthandler.cancel_dump_traceback_later()
    except Exception:
        pass


def pytest_collection_modifyitems(session, config, items):
    ordered_paths = [
        "test/playwright/auth/test_smoke_auth_page.py",
        "test/playwright/auth/test_toggle_login_register.py",
        "test/playwright/auth/test_validation_presence.py",
        "test/playwright/auth/test_sso_optional.py",
        "test/playwright/auth/test_register_success_optional.py",
        "test/playwright/auth/test_login_success_optional.py",
        "test/playwright/e2e/test_model_providers_zhipu_ai_defaults.py",
        "test/playwright/e2e/test_dataset_upload_parse.py",
        "test/playwright/e2e/test_next_apps_chat.py",
        "test/playwright/e2e/test_next_apps_search.py",
        "test/playwright/e2e/test_next_apps_agent.py",
    ]
    order_map = {path: idx for idx, path in enumerate(ordered_paths)}

    def _rel_path(item) -> str:
        try:
            return Path(str(item.fspath)).resolve().relative_to(ROOT_DIR).as_posix()
        except Exception:
            return str(item.fspath)

    indexed = list(enumerate(items))

    def _sort_key(entry):
        orig_idx, item = entry
        rel_path = _rel_path(item)
        order_idx = order_map.get(rel_path)
        if order_idx is not None:
            return (0, order_idx, orig_idx)
        return (1, rel_path, item.name, orig_idx)

    items[:] = [item for _, item in sorted(indexed, key=_sort_key)]


@pytest.fixture(scope="session")
def base_url() -> str:
    value = os.getenv("RAGFLOW_BASE_URL") or os.getenv("BASE_URL")
    if not value:
        value = BASE_URL_DEFAULT
    return value.rstrip("/")


@pytest.fixture(scope="session")
def login_path() -> str:
    value = os.getenv("LOGIN_PATH")
    if not value:
        value = LOGIN_PATH_DEFAULT
    if not value.startswith("/"):
        value = "/" + value
    return value


@pytest.fixture(scope="session")
def login_url(base_url: str, login_path: str) -> str:
    return _build_url(base_url, login_path)


@pytest.fixture(scope="session")
def smoke_login_url(login_url: str) -> str:
    return login_url


@pytest.fixture(scope="session")
def browser():
    browser_name = os.getenv("PW_BROWSER", "chromium")
    headless = _env_bool("PW_HEADLESS", True)
    slow_mo = _env_int("PW_SLOWMO_MS", 0)
    with sync_playwright() as playwright:
        if not hasattr(playwright, browser_name):
            raise ValueError(f"Unsupported browser: {browser_name}")
        browser_type = getattr(playwright, browser_name)
        browser_instance = browser_type.launch(headless=headless, slow_mo=slow_mo)
        try:
            yield browser_instance
        finally:
            browser_instance.close()


@pytest.fixture
def context(browser):
    context_instance = browser.new_context(ignore_https_errors=True)
    trace_enabled = _env_bool("PW_TRACE", False)
    if trace_enabled:
        context_instance.tracing.start(screenshots=True, snapshots=True, sources=True)
        context_instance._trace_enabled = True
        context_instance._trace_saved = False
    try:
        yield context_instance
    finally:
        if getattr(context_instance, "_trace_enabled", False) and not getattr(
            context_instance, "_trace_saved", False
        ):
            try:
                context_instance.tracing.stop()
            except Exception:
                pass
        context_instance.close()


def _configure_page(page_instance):
    timeout_ms = _playwright_action_timeout_ms()
    if timeout_ms is not None:
        page_instance.set_default_timeout(timeout_ms)
        page_instance.set_default_navigation_timeout(timeout_ms)
    page_instance._diag = {
        "console_errors": [],
        "page_errors": [],
        "request_failed": [],
    }

    net_log = _env_bool("PW_NET_LOG", False)

    def on_console(msg):
        if msg.type != "error":
            return
        entry = f"console[{msg.type}]: {msg.text}"
        page_instance._diag["console_errors"].append(entry)
        if net_log:
            print(entry, flush=True)

    def on_page_error(err):
        entry = f"pageerror: {err}"
        page_instance._diag["page_errors"].append(entry)
        if net_log:
            print(entry, flush=True)

    def on_request_failed(req):
        try:
            failure_text = _failure_text(req)
            entry = f"requestfailed: {req.method} {req.url} -> {failure_text}"
            page_instance._diag["request_failed"].append(entry)
            if net_log:
                print(entry, flush=True)
        except Exception as exc:
            if net_log:
                print(f"requestfailed: <handler_error> {exc}", flush=True)
            return

    page_instance.on("console", on_console)
    page_instance.on("pageerror", on_page_error)
    page_instance.on("requestfailed", on_request_failed)
    return page_instance


@pytest.fixture
def page(context, request):
    page_instance = _configure_page(context.new_page())

    try:
        yield page_instance
    finally:
        _write_artifacts_if_failed(page_instance, context, request)
        page_instance.close()


@pytest.fixture(scope="module")
def flow_context(browser, request):
    try:
        browser_context_args = request.getfixturevalue("browser_context_args")
    except Exception:
        browser_context_args = {}
    if browser_context_args is None:
        browser_context_args = {}
    args = dict(browser_context_args)
    args.setdefault("ignore_https_errors", True)
    ctx = browser.new_context(**args)
    yield ctx
    ctx.close()


@pytest.fixture(scope="module")
def flow_page(flow_context):
    page_instance = _configure_page(flow_context.new_page())
    yield page_instance
    page_instance.close()


@pytest.fixture(scope="module")
def flow_state():
    return {}


@pytest.fixture(autouse=True)
def _flow_artifacts(request):
    if "flow_page" not in request.fixturenames:
        yield
        return
    yield
    try:
        page_instance = request.getfixturevalue("flow_page")
        context = request.getfixturevalue("flow_context")
    except Exception:
        return
    _write_artifacts_if_failed(page_instance, context, request)


@pytest.fixture
def step():
    enabled = _env_bool("PW_STEP_LOG", False)

    def _stepper(label: str):
        return _step(label, enabled)

    return _stepper


@pytest.fixture
def reg_email_base() -> str:
    return os.getenv("REG_EMAIL_BASE", REG_EMAIL_BASE_DEFAULT)


@pytest.fixture
def reg_email_unique() -> bool:
    return _env_bool("REG_EMAIL_UNIQUE", False)


@pytest.fixture
def reg_email_generator(reg_email_base: str, reg_email_unique: bool):
    def _generate(force_unique: bool = False) -> str:
        unique = reg_email_unique or force_unique
        return _make_reg_email(reg_email_base, unique)

    return _generate


@pytest.fixture
def reg_email(reg_email_generator) -> str:
    return reg_email_generator()


@pytest.fixture
def reg_password() -> str:
    return REG_PASSWORD_DEFAULT


@pytest.fixture(scope="session")
def seeded_user_credentials(base_url: str, login_url: str, browser) -> tuple[str, str]:
    env_email = os.getenv("SEEDED_USER_EMAIL")
    env_password = os.getenv("SEEDED_USER_PASSWORD")
    if env_email and env_password:
        return env_email, env_password

    seeding_mode = os.getenv("RAGFLOW_SEEDING_MODE", "auto").strip().lower()
    if seeding_mode not in {"auto", "api", "ui"}:
        if _env_bool("PW_FIXTURE_DEBUG", False):
            print(
                f"[seeded] Unknown RAGFLOW_SEEDING_MODE={seeding_mode!r}; using auto.",
                flush=True,
            )
        seeding_mode = "auto"

    base_email = os.getenv("REG_EMAIL_BASE", REG_EMAIL_BASE_DEFAULT)
    password = os.getenv("SEEDED_USER_PASSWORD") or REG_PASSWORD_DEFAULT
    nickname = os.getenv("REG_NICKNAME", REG_NICKNAME_DEFAULT)
    email = _generate_seeded_email(base_email)
    _assert_reg_email(email)

    seed_errors = []
    seeded_via = None
    if seeding_mode in {"auto", "api"}:
        seeded_via = "api"
        try:
            _api_register_user(base_url, email, password, nickname)
            try:
                _api_login_user(base_url, email, password)
            except Exception as exc:
                if _env_bool("PW_FIXTURE_DEBUG", False):
                    print(f"[seeded] api login verification failed: {exc}", flush=True)
        except _RegisterDisabled as exc:
            seed_errors.append(f"api: {exc}")
            seeded_via = None
        except Exception as exc:
            seed_errors.append(f"api: {exc}")
            seeded_via = None
            if seeding_mode == "api":
                details = "; ".join(seed_errors)
                raise RuntimeError(
                    f"Failed to seed user via API registration. {details}"
                ) from exc

    if seeded_via is None and seeding_mode in {"auto", "ui"}:
        seeded_via = "ui"
        try:
            _ui_register_user(browser, login_url, email, password, nickname)
        except _RegisterDisabled as exc:
            seed_errors.append(f"ui: {exc}")
            default_email = os.getenv("DEFAULT_SUPERUSER_EMAIL", "admin@ragflow.io")
            raise RuntimeError(
                "User registration is disabled and no default account is available. "
                f"Known superuser defaults ({default_email}) cannot be used with the "
                "normal login endpoint. Enable registration or seed a test account."
            ) from exc
        except Exception as ui_exc:
            seed_errors.append(f"ui: {ui_exc}")
            details = "; ".join(seed_errors)
            raise RuntimeError(
                f"Failed to seed user via API or UI registration. {details}"
            ) from ui_exc

    os.environ["SEEDED_USER_EMAIL"] = email
    os.environ["SEEDED_USER_PASSWORD"] = password
    if _env_bool("PW_FIXTURE_DEBUG", False):
        print(f"[seeded] created user via {seeded_via}: {email}", flush=True)
    return email, password


@pytest.fixture
def reg_nickname() -> str:
    return REG_NICKNAME_DEFAULT


@pytest.fixture
def snap(page, request):
    if "flow_page" in request.fixturenames:
        page = request.getfixturevalue("flow_page")
    base_dir = _request_artifacts_dir(request)
    node_prefix = _request_artifact_prefix(request)
    counter = {"value": 0}

    def _snap(label: str):
        counter["value"] += 1
        safe_label = _sanitize_filename(label) or "step"
        filename = f"{node_prefix}__{counter['value']:02d}_{safe_label}.png"
        path = base_dir / filename
        page.screenshot(path=str(path), full_page=True)
        if _env_bool("PW_FIXTURE_DEBUG", False):
            print(f"[artifact] snapshot: {path}", flush=True)
        return path

    _snap.dir = base_dir
    return _snap


def _debug_dump_auth_state(page, label: str, submit_locator=None) -> None:
    if not _env_bool("PW_DEBUG_DUMP", False):
        return
    print(f"[auth-debug] label={label}", flush=True)
    form_count = page.locator("form").count()
    visible_form_count = page.locator("form:visible").count()
    print(
        f"[auth-debug] forms total={form_count} visible={visible_form_count}",
        flush=True,
    )
    forms_info = page.evaluate(
        """
        () => {
          const forms = Array.from(document.querySelectorAll('form'));
          const getFace = (el) => {
            let node = el;
            while (node && node !== document.body) {
              const style = window.getComputedStyle(node);
              if (style && style.backfaceVisibility === 'hidden') {
                return node;
              }
              node = node.parentElement;
            }
            return el;
          };
          const getFlip = (el) => {
            let node = el;
            while (node && node !== document.body) {
              const style = window.getComputedStyle(node);
              if (style && style.transformStyle === 'preserve-3d') {
                return node;
              }
              node = node.parentElement;
            }
            return null;
          };
          const isVisible = (el) => {
            const style = window.getComputedStyle(el);
            if (style && (style.visibility === 'hidden' || style.display === 'none')) {
              return false;
            }
            const rect = el.getBoundingClientRect();
            return rect.width > 0 && rect.height > 0;
          };
          return forms.filter(isVisible).map((form, idx) => {
            const rect = form.getBoundingClientRect();
            const button = form.querySelector('button[type="submit"]');
            const buttonText = button ? (button.textContent || '').trim() : '';
            const face = getFace(form);
            const flip = getFlip(face);
            return {
              index: idx,
              authMode: form.getAttribute('data-auth-mode') || '',
              isActive: form.getAttribute('data-active') === 'true',
              rect: {
                x: rect.x,
                y: rect.y,
                width: rect.width,
                height: rect.height,
              },
              submitText: buttonText.slice(0, 60),
              submitHasContinue: buttonText.toLowerCase().includes('continue'),
              faceTransform: window.getComputedStyle(face).transform,
              faceBackface: window.getComputedStyle(face).backfaceVisibility,
              flipTransform: flip ? window.getComputedStyle(flip).transform : null,
              flipTransformStyle: flip ? window.getComputedStyle(flip).transformStyle : null,
            };
          });
        }
        """
    )
    for info in forms_info:
        print(f"[auth-debug] visible_form={info}", flush=True)

    if submit_locator is None or submit_locator.count() == 0:
        print("[auth-debug] submit button not found", flush=True)
        return
    try:
        bbox = submit_locator.bounding_box()
    except Exception as exc:
        print(f"[auth-debug] submit bounding box failed: {exc}", flush=True)
        return
    if not bbox:
        print("[auth-debug] submit bounding box empty", flush=True)
        return
    center_x = bbox["x"] + bbox["width"] / 2
    center_y = bbox["y"] + bbox["height"] / 2
    element_html = page.evaluate(
        """
        ({ x, y }) => {
          const el = document.elementFromPoint(x, y);
          if (!el) return null;
          return el.outerHTML ? el.outerHTML.slice(0, 500) : String(el);
        }
        """,
        {"x": center_x, "y": center_y},
    )
    print(f"[auth-debug] elementFromPoint={element_html}", flush=True)


@pytest.fixture
def auth_debug_dump(page, request):
    if "flow_page" in request.fixturenames:
        page = request.getfixturevalue("flow_page")
    def _dump(label: str, submit_locator=None) -> None:
        _debug_dump_auth_state(page, label, submit_locator)

    return _dump


def _write_artifacts_if_failed(page, context, request) -> None:
    report = getattr(request.node, "_rep_call", None)
    if not report or not report.failed:
        return

    timestamp = time.strftime("%Y%m%d-%H%M%S")
    base_dir = _request_artifacts_dir(request)
    safe_name = _request_artifact_prefix(request)
    screenshot_path = base_dir / f"{safe_name}_{timestamp}.png"
    html_path = base_dir / f"{safe_name}_{timestamp}.html"
    events_path = base_dir / f"{safe_name}_{timestamp}.log"
    trace_path = base_dir / f"{safe_name}_{timestamp}.zip"

    try:
        page.screenshot(path=str(screenshot_path), full_page=True)
    except Exception as exc:
        print(f"[artifact] screenshot failed: {exc}", flush=True)

    try:
        html_path.write_text(page.content(), encoding="utf-8")
    except Exception as exc:
        print(f"[artifact] html dump failed: {exc}", flush=True)

    try:
        lines = []
        diag = getattr(page, "_diag", {})
        for key in ("console_errors", "page_errors", "request_failed"):
            entries = diag.get(key, [])
            if entries:
                lines.append(f"{key}:")
                lines.extend(entries)
        if lines:
            events_path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    except Exception as exc:
        print(f"[artifact] events dump failed: {exc}", flush=True)

    if getattr(context, "_trace_enabled", False) and not getattr(
        context, "_trace_saved", False
    ):
        try:
            context.tracing.stop(path=str(trace_path))
            context._trace_saved = True
        except Exception as exc:
            print(f"[artifact] trace dump failed: {exc}", flush=True)


def _auth_ready_summary(page) -> dict:
    return page.evaluate(
        """
        () => {
          const summarizeInputs = (form) => {
            const inputs = Array.from(form.querySelectorAll('input'));
            return inputs.map((input) => ({
              type: input.getAttribute('type') || '',
              name: input.getAttribute('name') || '',
              autocomplete: input.getAttribute('autocomplete') || '',
              placeholder: input.getAttribute('placeholder') || '',
            }));
          };
          const allForms = Array.from(document.querySelectorAll('form'));
          const visibleForms = allForms.filter((el) => {
            const style = window.getComputedStyle(el);
            if (style && (style.visibility === 'hidden' || style.display === 'none')) {
              return false;
            }
            const rect = el.getBoundingClientRect();
            return rect.width > 0 && rect.height > 0;
          });
          return {
            formCount: allForms.length,
            visibleFormCount: visibleForms.length,
            visibleFormInputs: visibleForms.map(summarizeInputs),
          };
        }
        """
    )


def _format_auth_ready_summary(summary: dict) -> str:
    lines = [
        f"form_count: {summary.get('formCount')}",
        f"visible_form_count: {summary.get('visibleFormCount')}",
    ]
    visible_inputs = summary.get("visibleFormInputs") or []
    for idx, inputs in enumerate(visible_inputs, start=1):
        input_parts = []
        for item in inputs:
            parts = []
            for key in ("type", "name", "autocomplete", "placeholder"):
                value = item.get(key)
                if value:
                    parts.append(f"{key}={value}")
            input_parts.append("{" + ", ".join(parts) + "}")
        lines.append(f"visible_form_{idx}_inputs: {input_parts}")
    return "\n".join(lines) + "\n"


def _write_auth_ready_diagnostics(page, request, reason: str) -> None:
    timestamp = time.strftime("%Y%m%d-%H%M%S")
    base_dir = _request_artifacts_dir(request)
    safe_name = _request_artifact_prefix(request)
    screenshot_path = base_dir / f"{safe_name}_auth_ready_{timestamp}.png"
    html_path = base_dir / f"{safe_name}_auth_ready_{timestamp}.html"
    summary_path = base_dir / f"{safe_name}_auth_ready_{timestamp}.log"

    try:
        page.screenshot(path=str(screenshot_path), full_page=True)
    except Exception as exc:
        print(f"[auth_ready] screenshot failed: {exc}", flush=True)

    try:
        html_path.write_text(page.content(), encoding="utf-8")
    except Exception as exc:
        print(f"[auth_ready] html dump failed: {exc}", flush=True)

    try:
        summary = _auth_ready_summary(page)
        summary_text = (
            f"reason: {reason}\nurl: {page.url}\ntitle: {page.title()}\n"
            + _format_auth_ready_summary(summary)
        )
        summary_path.write_text(summary_text, encoding="utf-8")
        print(summary_text, flush=True)
    except Exception as exc:
        print(f"[auth_ready] summary failed: {exc}", flush=True)


def _wait_for_auth_ui_ready(page, request) -> None:
    timeout_ms = _playwright_auth_ready_timeout_ms()
    email_selector = AUTH_EMAIL_INPUT_SELECTOR
    password_selector = AUTH_PASSWORD_INPUT_SELECTOR
    submit_selector = AUTH_SUBMIT_SELECTOR
    active_forms = page.locator(AUTH_ACTIVE_FORM_SELECTOR)
    try:
        expect(active_forms).to_have_count(1, timeout=timeout_ms)
    except AssertionError as exc:
        _write_auth_ready_diagnostics(page, request, "auth active form not unique")
        raise AssertionError(
            "Auth UI not ready within "
            f"{timeout_ms}ms. Expected a single active auth form."
        ) from exc
    ready_forms = active_forms.filter(
        has=page.locator(password_selector)
    ).filter(has=page.locator(email_selector)).filter(
        has=page.locator(submit_selector)
    )
    try:
        expect(ready_forms).not_to_have_count(0, timeout=timeout_ms)
    except AssertionError as exc:
        _write_auth_ready_diagnostics(page, request, "auth UI readiness timeout")
        raise AssertionError(
            "Auth UI not ready within "
            f"{timeout_ms}ms. Expected a visible form with email-like and password inputs."
        ) from exc


def _wait_for_active_form_clickable(page, request, form) -> None:
    timeout_ms = _playwright_auth_ready_timeout_ms()
    active_forms = page.locator(AUTH_ACTIVE_FORM_SELECTOR)
    submit_buttons = form.locator(AUTH_SUBMIT_SELECTOR)
    try:
        expect(active_forms).to_have_count(1, timeout=timeout_ms)
        expect(submit_buttons).to_have_count(1, timeout=timeout_ms)
        expect(submit_buttons).to_be_visible()
        expect(submit_buttons).to_be_enabled()
        status = page.locator("[data-testid='auth-status']")
        if status.count() > 0:
            expect(status).not_to_have_attribute("data-state", "loading")
    except AssertionError as exc:
        try:
            total_forms = page.locator(AUTH_FORM_SELECTOR).count()
            active_form_count = active_forms.count()
            forms_info = []
            for idx in range(min(total_forms, 5)):
                form_node = page.locator(AUTH_FORM_SELECTOR).nth(idx)
                try:
                    info = form_node.evaluate(
                        """
                        (el) => {
                          const submit = el.querySelector("button[type='submit'], [data-testid='auth-submit']");
                          const isVisible = (node) => {
                            const style = window.getComputedStyle(node);
                            if (style && (style.visibility === 'hidden' || style.display === 'none')) {
                              return false;
                            }
                            const rect = node.getBoundingClientRect();
                            return rect.width > 0 && rect.height > 0;
                          };
                          return {
                            authMode: el.getAttribute('data-auth-mode') || '',
                            active: el.getAttribute('data-active') || '',
                            submit: submit
                              ? {
                                  tag: submit.tagName,
                                  type: submit.getAttribute('type'),
                                  text: (submit.innerText || '').trim(),
                                  testid: submit.getAttribute('data-testid'),
                                  visible: isVisible(submit),
                                  enabled: !submit.disabled,
                                }
                              : null,
                          };
                        }
                        """
                    )
                except Exception as inner_exc:
                    info = {"error": str(inner_exc)}
                forms_info.append(info)
            print(
                f"[auth-debug] forms total={total_forms} active_forms={active_form_count} details={forms_info}",
                flush=True,
            )
        except Exception:
            pass
        _write_auth_ready_diagnostics(
            page, request, "active auth form submit not clickable"
        )
        _debug_dump_auth_state(page, "active_form_not_clickable", submit_buttons)
        raise AssertionError(
            "Active auth form submit button not clickable within "
            f"{timeout_ms}ms. The flip animation may still be in progress."
        ) from exc


def _locator_is_topmost(locator) -> bool:
    try:
        return bool(
            locator.evaluate(
                """
                (el) => {
                  const rect = el.getBoundingClientRect();
                  const x = rect.left + rect.width / 2;
                  const y = rect.top + rect.height / 2;
                  const top = document.elementFromPoint(x, y);
                  return top && (top === el || el.contains(top));
                }
                """
            )
        )
    except Exception:
        return False


@pytest.fixture
def auth_click():
    def _click(locator, label: str = "click") -> None:
        timeout_ms = _playwright_auth_ready_timeout_ms()
        try:
            locator.click(timeout=timeout_ms)
        except PlaywrightTimeoutError as exc:
            if "intercepts pointer events" in str(exc) and _locator_is_topmost(
                locator
            ):
                if _env_bool("PW_FIXTURE_DEBUG", False):
                    print(f"[auth-click] forcing {label}", flush=True)
                locator.click(force=True, timeout=timeout_ms)
                return
            raise

    return _click


@pytest.fixture
def active_auth_context(page, request):
    if "flow_page" in request.fixturenames:
        page = request.getfixturevalue("flow_page")
    def _mark_active_form() -> None:
        timeout_ms = _playwright_auth_ready_timeout_ms()
        try:
            page.wait_for_function(
                """
                () => {
                  const forms = Array.from(document.querySelectorAll("form[data-testid='auth-form']"))
                    .filter((el) => el.querySelector("[data-testid='auth-email']"));
                  const getFace = (el) => {
                    let node = el;
                    while (node && node !== document.body) {
                      const style = window.getComputedStyle(node);
                      if (style && style.backfaceVisibility === 'hidden') {
                        return node;
                      }
                      node = node.parentElement;
                    }
                    return el;
                  };
                  const getFlip = (el) => {
                    let node = el;
                    while (node && node !== document.body) {
                      const style = window.getComputedStyle(node);
                      if (style && style.transformStyle === 'preserve-3d') {
                        return node;
                      }
                      node = node.parentElement;
                    }
                    return null;
                  };
                  const parseSign = (transform) => {
                    if (!transform || transform === 'none') return 1;
                    const match3d = transform.match(/^matrix3d\\((.+)\\)$/);
                    if (match3d) {
                      const parts = match3d[1].split(',').map((v) => parseFloat(v.trim()));
                      return Number.isFinite(parts[0]) ? Math.sign(parts[0]) : 0;
                    }
                    const match2d = transform.match(/^matrix\\((.+)\\)$/);
                    if (match2d) {
                      const parts = match2d[1].split(',').map((v) => parseFloat(v.trim()));
                      return Number.isFinite(parts[0]) ? Math.sign(parts[0]) : 0;
                    }
                    return 0;
                  };
                  const computeFacing = (el) => {
                    const face = getFace(el);
                    const faceTransform = window.getComputedStyle(face).transform;
                    const faceSign = parseSign(faceTransform);
                    const flip = getFlip(face);
                    const flipTransform = flip
                      ? window.getComputedStyle(flip).transform
                      : 'none';
                    const flipSign = parseSign(flipTransform);
                    return faceSign * flipSign;
                  };
                  if (forms.length > 0) {
                    const firstFace = getFace(forms[0]);
                    const flip = getFlip(firstFace);
                    if (flip) {
                      const flipTransform = window.getComputedStyle(flip).transform;
                      const now = performance.now();
                      const state = window.__qa_flip_state || { transform: null, time: 0 };
                      if (state.transform !== flipTransform) {
                        window.__qa_flip_state = { transform: flipTransform, time: now };
                        return false;
                      }
                      if (now - state.time < 150) {
                        return false;
                      }
                    }
                  }
                  const candidates = forms
                    .map((el) => {
                      const rect = el.getBoundingClientRect();
                      if (!rect.width || !rect.height) return null;
                      return { el, facing: computeFacing(el) };
                    })
                    .filter(Boolean);
                  candidates.sort((a, b) => b.facing - a.facing);
                  let pick = null;
                  if (candidates.length === 1) {
                    pick = candidates[0];
                  } else if (candidates.length > 1 && candidates[0].facing !== candidates[1].facing) {
                    pick = candidates[0];
                  }
                  if (!pick) {
                    const fallback = forms.find((el) => {
                      const rect = el.getBoundingClientRect();
                      if (!rect.width || !rect.height) return false;
                      const x = rect.left + rect.width / 2;
                      const y = rect.top + Math.min(rect.height / 2, 10);
                      const top = document.elementFromPoint(x, y);
                      return top && el.contains(top);
                    });
                    if (fallback) {
                      pick = { el: fallback, facing: computeFacing(fallback) };
                    }
                  }
                  forms.forEach((el) => el.removeAttribute('data-qa-active'));
                  if (!pick || !pick.el) return false;
                  pick.el.setAttribute('data-qa-active', 'true');
                  const submit = pick.el.querySelector("[data-testid='auth-submit']");
                  return Boolean(submit) && pick.facing > 0;
                }
                """,
                timeout=timeout_ms,
            )
        except Exception as exc:
            _write_auth_ready_diagnostics(
                page, request, "active auth form did not become front-facing"
            )
            _debug_dump_auth_state(page, "active_form_not_front_facing")
            raise AssertionError(
                "Active auth form not ready within "
                f"{timeout_ms}ms. The flip animation may not have settled."
            ) from exc

    def _get():
        _wait_for_auth_ui_ready(page, request)
        card = page.locator("[data-testid='auth-card-active']")
        form = page.locator(AUTH_ACTIVE_FORM_SELECTOR)
        timeout_ms = _playwright_auth_ready_timeout_ms()
        try:
            expect(form).to_have_count(1, timeout=timeout_ms)
        except AssertionError as exc:
            _write_auth_ready_diagnostics(
                page, request, "active auth form selection failed"
            )
            raise AssertionError(
                "Active auth form not found. The login card may not be visible or the DOM changed."
            ) from exc
        _wait_for_active_form_clickable(page, request, form)
        return form, card

    return _get
