# Playwright Test README

## One-line command (run everything)

```bash
BASE_URL=http://localhost:9222 E2E_ADMIN_EMAIL=admin@ragflow.io E2E_ADMIN_PASSWORD=admin PW_FIXTURE_DEBUG=1 uv run pytest -q test/playwright -s --junitxml=/tmp/playwright-full.xml
```

## Common commands

Run smoke subset:

```bash
BASE_URL=http://localhost:9222 E2E_ADMIN_EMAIL=admin@ragflow.io E2E_ADMIN_PASSWORD=admin uv run pytest -q test/playwright -m smoke -s --junitxml=/tmp/playwright-smoke.xml
```

Run full suite:

```bash
BASE_URL=http://localhost:9222 E2E_ADMIN_EMAIL=admin@ragflow.io E2E_ADMIN_PASSWORD=admin uv run pytest -q test/playwright -s --junitxml=/tmp/playwright-full.xml
```

Run one file in isolation:

```bash
BASE_URL=http://localhost:9222 E2E_ADMIN_EMAIL=admin@ragflow.io E2E_ADMIN_PASSWORD=admin uv run pytest -q test/playwright/e2e/test_next_apps_agent.py -s --junitxml=/tmp/playwright-agent.xml
```

Run one test case in isolation:

```bash
BASE_URL=http://localhost:9222 E2E_ADMIN_EMAIL=admin@ragflow.io E2E_ADMIN_PASSWORD=admin uv run pytest -q test/playwright/e2e/test_next_apps_chat.py::test_chat_create_select_dataset_and_receive_answer_flow -s -x --junitxml=/tmp/playwright-chat-one.xml
```

## Argument reference

- `uv run`: run `pytest` inside the project-managed Python environment.
- `pytest`: test runner.
- `-q`: quieter output.
- `test/playwright`: run the whole Playwright suite folder.
- `test/playwright/...py`: run one file only.
- `::test_name`: run one test function only.
- `-m smoke`: run tests with `@pytest.mark.smoke`.
- `-s`: show `print()` and fixture logs live.
- `-x`: stop at first failure.
- `--junitxml=/tmp/<name>.xml`: write machine-readable results to XML.

## Environment variables used in commands

- `BASE_URL`: app URL (this suite is currently run against `http://localhost:9222`).
- `E2E_ADMIN_EMAIL`: login email for authenticated flows.
- `E2E_ADMIN_PASSWORD`: login password for authenticated flows.
- `PW_FIXTURE_DEBUG=1`: optional; prints fixture provisioning details.

## Output and artifacts

- JUnit XML files are written to `/tmp/...` from `--junitxml`.
- Screenshots and diagnostics are written under:
  - `test/playwright/artifacts/`
