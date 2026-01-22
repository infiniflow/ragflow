import faulthandler
import os
import re
import secrets
import signal
import time
from contextlib import contextmanager
from pathlib import Path
from urllib.parse import urljoin

import pytest
from playwright.sync_api import TimeoutError as PlaywrightTimeoutError
from playwright.sync_api import expect, sync_playwright

ARTIFACTS_DIR = Path(__file__).resolve().parent / "artifacts"
BASE_URL_DEFAULT = "http://localhost:9222"
LOGIN_PATH_DEFAULT = "/login"
DEFAULT_TIMEOUT_MS = 15000
DEFAULT_HANG_TIMEOUT_S = 120
AUTH_READY_TIMEOUT_MS_DEFAULT = 15000
REG_EMAIL_BASE_DEFAULT = "qa@infiniflow.org"
REG_NICKNAME_DEFAULT = "qa"
REG_PASSWORD_DEFAULT = "123"
REG_EMAIL_LOCAL_RE = re.compile(r"^[A-Za-z0-9_.-]+$")
REG_EMAIL_BACKEND_RE = re.compile(r"^[\w\._-]{1,}@([\w_-]+\.)+[\w-]{2,}$")


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
    hang_timeout = _env_int("HANG_TIMEOUT_S", DEFAULT_HANG_TIMEOUT_S)
    if hang_timeout > 0:
        faulthandler.dump_traceback_later(hang_timeout, repeat=True)
    try:
        faulthandler.register(signal.SIGUSR1, all_threads=True)
    except (AttributeError, ValueError):
        pass


def pytest_sessionfinish(session, exitstatus):
    try:
        faulthandler.cancel_dump_traceback_later()
    except Exception:
        pass


@pytest.fixture(scope="session")
def base_url() -> str:
    value = os.getenv("BASE_URL")
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


@pytest.fixture
def page(context, request):
    timeout_ms = _env_int("PW_TIMEOUT_MS", DEFAULT_TIMEOUT_MS)
    page_instance = context.new_page()
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

    try:
        yield page_instance
    finally:
        _write_artifacts_if_failed(page_instance, context, request)
        page_instance.close()


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


@pytest.fixture
def reg_nickname() -> str:
    return REG_NICKNAME_DEFAULT


@pytest.fixture
def snap(page, request):
    base_dir = ARTIFACTS_DIR / _sanitize_filename(request.node.nodeid)
    base_dir.mkdir(parents=True, exist_ok=True)
    counter = {"value": 0}

    def _snap(label: str):
        counter["value"] += 1
        safe_label = _sanitize_filename(label) or "step"
        filename = f"{counter['value']:02d}_{safe_label}.png"
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
              isActive: form.getAttribute('data-qa-active') === 'true',
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
def auth_debug_dump(page):
    def _dump(label: str, submit_locator=None) -> None:
        _debug_dump_auth_state(page, label, submit_locator)

    return _dump


def _write_artifacts_if_failed(page, context, request) -> None:
    report = getattr(request.node, "_rep_call", None)
    if not report or not report.failed:
        return

    timestamp = time.strftime("%Y%m%d-%H%M%S")
    safe_name = _sanitize_filename(request.node.nodeid)
    screenshot_path = ARTIFACTS_DIR / f"{safe_name}_{timestamp}.png"
    html_path = ARTIFACTS_DIR / f"{safe_name}_{timestamp}.html"
    events_path = ARTIFACTS_DIR / f"{safe_name}_{timestamp}.log"
    trace_path = ARTIFACTS_DIR / f"{safe_name}_{timestamp}.zip"

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
    safe_name = _sanitize_filename(getattr(request.node, "nodeid", "auth_ready"))
    screenshot_path = ARTIFACTS_DIR / f"{safe_name}_auth_ready_{timestamp}.png"
    html_path = ARTIFACTS_DIR / f"{safe_name}_auth_ready_{timestamp}.html"
    summary_path = ARTIFACTS_DIR / f"{safe_name}_auth_ready_{timestamp}.log"

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
    timeout_ms = _env_int("AUTH_READY_TIMEOUT_MS", AUTH_READY_TIMEOUT_MS_DEFAULT)
    email_selector = (
        "input[autocomplete='email'], input[type='email'], input[name*='email' i]"
    )
    visible_forms = page.locator("form:visible")
    ready_forms = (
        visible_forms.filter(has=page.locator("input[type='password']")).filter(
            has=page.locator(email_selector)
        )
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
    timeout_ms = _env_int("AUTH_READY_TIMEOUT_MS", AUTH_READY_TIMEOUT_MS_DEFAULT)
    submit_button = form.locator("button[type='submit']")
    try:
        expect(submit_button).to_have_count(1, timeout=timeout_ms)
        expect(submit_button).to_be_visible()
    except AssertionError as exc:
        _write_auth_ready_diagnostics(
            page, request, "active auth form submit not clickable"
        )
        _debug_dump_auth_state(page, "active_form_not_clickable", submit_button)
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
        timeout_ms = _env_int("AUTH_READY_TIMEOUT_MS", AUTH_READY_TIMEOUT_MS_DEFAULT)
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
    def _mark_active_form() -> None:
        timeout_ms = _env_int("AUTH_READY_TIMEOUT_MS", AUTH_READY_TIMEOUT_MS_DEFAULT)
        try:
            page.wait_for_function(
                """
                () => {
                  const forms = Array.from(document.querySelectorAll('form'))
                    .filter((el) => el.querySelector('input[autocomplete="email"]'));
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
                  const submit = pick.el.querySelector('button[type="submit"]');
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
        _mark_active_form()
        form = page.locator("form[data-qa-active='true']")
        timeout_ms = _env_int("AUTH_READY_TIMEOUT_MS", AUTH_READY_TIMEOUT_MS_DEFAULT)
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
        card = form.locator("xpath=..")
        return form, card

    return _get
