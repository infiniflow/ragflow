#!/usr/bin/env bash
set -euo pipefail

if [[ ! -d ".venv" ]]; then
  echo "[auth-demo] .venv not found. Create a venv at ./.venv or update this script." >&2
  exit 1
fi

if [[ -z "${BASE_URL:-}" ]]; then
  echo "[auth-demo] BASE_URL is required. See test/playwright/README.md for setup." >&2
  exit 1
fi

: "${REG_EMAIL_BASE:=qa@infiniflow.org}"

LOCAL_PART="${REG_EMAIL_BASE%@*}"
DOMAIN_PART="${REG_EMAIL_BASE#*@}"
UNIQUE_SUFFIX="$(date +%s%3N)_$$_${RANDOM}"
REGISTERED_EMAIL="${LOCAL_PART}_${UNIQUE_SUFFIX}@${DOMAIN_PART}"

export REG_EMAIL_BASE="${REGISTERED_EMAIL}"
export REG_EMAIL_UNIQUE=0
export SEEDED_USER_EMAIL="${REGISTERED_EMAIL}"
export SEEDED_USER_PASSWORD="${SEEDED_USER_PASSWORD:-123}"

# shellcheck disable=SC1091
source ./.venv/bin/activate

if [[ -z "${ZHIPU_AI_API_KEY:-}" ]]; then
  echo "[auth-demo] ZHIPU_AI_API_KEY is required for model providers test." >&2
  exit 1
fi

PYTEST_TARGETS=(
  test/playwright/auth/test_smoke_auth_page.py::test_auth_page_smoke
  test/playwright/auth/test_toggle_login_register.py::test_toggle_login_register
  test/playwright/auth/test_validation_presence.py::test_validation_presence
  test/playwright/auth/test_sso_optional.py::test_sso_optional
  test/playwright/auth/test_register_success_optional.py::test_register_success_optional
  test/playwright/auth/test_login_success_optional.py::test_login_success_optional
  test/playwright/e2e/test_model_providers_zhipu_ai_defaults.py::test_add_zhipu_ai_set_defaults_persist
  test/playwright/e2e/test_dataset_upload_parse.py::test_dataset_upload_parse_and_delete
  test/playwright/e2e/test_next_apps_chat.py::test_chat_create_select_dataset_and_receive_answer
  test/playwright/e2e/test_next_apps_search.py::test_search_create_select_dataset_and_results_nav_appears
  test/playwright/e2e/test_next_apps_agent.py::test_agent_create_then_import_json_then_run_and_wait_idle
)

pytest -q -s -vv -x "${PYTEST_TARGETS[@]}"

echo "[auth-demo] Success. Email: ${REGISTERED_EMAIL}"
echo "[auth-demo] Artifacts: test/playwright/artifacts/"
