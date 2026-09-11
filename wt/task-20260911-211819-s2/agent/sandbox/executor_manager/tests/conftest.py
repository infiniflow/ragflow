#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#
import os
import sys
from pathlib import Path

# executor_manager modules use top-level imports (e.g. `from api.handlers
# import ...`) resolved against the executor_manager directory (WORKDIR /app
# in Docker). Make that directory importable regardless of the pytest rootdir.
EXECUTOR_MANAGER_ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(EXECUTOR_MANAGER_ROOT))

# Keep tests hermetic: a small window makes the 429 case cheap to exercise.
# Must be set before `services.limiter` / `services.preauth` are first imported
# (module-level env reads). The pre-auth budget stays generous by default so
# ordinary auth tests never trip it; the dedicated pre-auth tests shrink the
# limiter explicitly.
os.environ["SANDBOX_RUN_RATE_LIMIT"] = os.environ.get("SANDBOX_TEST_RATE_LIMIT_OVERRIDE", "3/minute")
os.environ["SANDBOX_RUN_PREAUTH_RATE_LIMIT"] = os.environ.get("SANDBOX_TEST_PREAUTH_RATE_LIMIT_OVERRIDE", "1000/minute")

import pytest  # noqa: E402  (env vars above must be set before these imports)
from services.limiter import limiter  # noqa: E402
from services.preauth import preauth_limiter  # noqa: E402


@pytest.fixture(autouse=True)
def _reset_rate_limiters():
    limiter.reset()
    preauth_limiter.reset()
    yield
    preauth_limiter.reset()
    limiter.reset()


@pytest.fixture(autouse=True)
def _clean_token_env(monkeypatch):
    monkeypatch.delenv("SANDBOX_EXECUTOR_MANAGER_API_TOKEN", raising=False)
    yield
