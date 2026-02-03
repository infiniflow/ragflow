# Playwright auth UI tests

## Quick start

Smoke test (always runs at least one test):

```bash
pytest -q test/playwright -m smoke
```

Run all auth UI tests:

```bash
pytest -q test/playwright -m auth
```

If you use `uv`:

```bash
uv run pytest -q test/playwright -m smoke
```

## Environment variables

Required/optional:

- `BASE_URL` (default: `http://127.0.0.1`)
  - Example dev UI: `http://localhost:9222`.
  - For Docker (`SVR_WEB_HTTP_PORT=80`), set `BASE_URL=http://localhost`.
- `LOGIN_PATH` (default: `/login`)
- `SEEDED_USER_EMAIL` and `SEEDED_USER_PASSWORD` (optional; enables login success test)
- `DEMO_CREDS=1` (optional; uses demo credentials `qa@infiniflow.com` / `123` for login success test)
- `REG_EMAIL_BASE` (default: `qa@infiniflow.org`)
- `REG_EMAIL_UNIQUE=1` (optional; enables unique registration emails like `qa_1700000000000_123456@infiniflow.org`)
- `POST_LOGIN_PATH` (optional; expected path after login success, e.g. `/`)
- `REGISTER_ENABLED_EXPECTED` (optional; reserved for future gating checks)

Diagnostics and debugging:

- `PW_STEP_LOG=1` enable step logging
- `PW_NET_LOG=1` log `requestfailed` + console errors during the run
- `PW_TRACE=1` save a Playwright trace on failure
- `PW_BROWSER` (default: `chromium`)
- `PW_HEADLESS` (default: `1`, set `0` to see the browser)
- `PLAYWRIGHT_ACTION_TIMEOUT_MS` (default: `30000`)
  - Legacy: `PW_TIMEOUT_MS`
- `PW_SLOWMO_MS` (default: `0`)
- `PLAYWRIGHT_HANG_TIMEOUT_S` (default: `1800`, set `0` to disable)
  - Legacy: `HANG_TIMEOUT_S`
- `PLAYWRIGHT_AUTH_READY_TIMEOUT_MS` (default: `15000`)
  - Legacy: `AUTH_READY_TIMEOUT_MS`

## What runs without credentials

- `auth/test_smoke_auth_page.py` (marker: `smoke`, always runs)
- `auth/test_toggle_login_register.py` (skips if register toggle is gated off)
- `auth/test_validation_presence.py`
- `auth/test_sso_optional.py` (skips if no SSO providers are rendered)
- `auth/test_register_success_optional.py` (skips if register toggle is gated off)
- `auth/test_register_then_login_flow.py` (skips unless `REG_EMAIL_UNIQUE=1`)

`auth/test_login_success_optional.py` only runs if `DEMO_CREDS=1` or `SEEDED_USER_EMAIL` and `SEEDED_USER_PASSWORD` are set.

## Login success examples

Run with demo credentials:

```bash
DEMO_CREDS=1 BASE_URL=http://localhost:9222 \
  pytest -q test/playwright/auth/test_login_success_optional.py::test_login_success_optional -s -vv
```

Run with env credentials:

```bash
SEEDED_USER_EMAIL=user@yourdomain.com SEEDED_USER_PASSWORD=secret BASE_URL=http://localhost:9222 \
  pytest -q test/playwright/auth/test_login_success_optional.py::test_login_success_optional -s -vv
```

## Registration examples

Registration rejects plus-addressing; the backend only allows local-part characters `[A-Za-z0-9_.-]`.

Register only:

```bash
REG_EMAIL_UNIQUE=1 BASE_URL=http://localhost:9222 \
  pytest -q test/playwright/auth/test_register_success_optional.py::test_register_success_optional -s -vv
```

Register then login (single test):

```bash
REG_EMAIL_UNIQUE=1 BASE_URL=http://localhost:9222 \
  pytest -q test/playwright/auth/test_register_then_login_flow.py::test_register_then_login_flow -s -vv
```

Run the end-to-end demo script:

```bash
BASE_URL=http://localhost:9222 \
  scripts/run_auth_demo.sh
```

## Artifacts on failure

Artifacts are written to:

- `test/playwright/artifacts/`
  - per-test screenshots are stored under `test/playwright/artifacts/<testname>/`

On failure, the suite writes:

- a full-page screenshot (`.png`)
- a full HTML dump (`.html`)
- a diagnostics log (`.log`)
- an optional trace (`.zip`) if `PW_TRACE=1`

## Hang investigation

- Automatic stack dump after `PLAYWRIGHT_HANG_TIMEOUT_S` seconds.
- Manual dump: `kill -USR1 <pytest_pid>` (writes traceback to stderr).
