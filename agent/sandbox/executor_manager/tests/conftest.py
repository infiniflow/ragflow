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
# Must be set before `services.limiter` is first imported (module-level env read).
os.environ["SANDBOX_RUN_RATE_LIMIT"] = os.environ.get("SANDBOX_TEST_RATE_LIMIT_OVERRIDE", "3/minute")

import pytest
from services.limiter import limiter


@pytest.fixture(autouse=True)
def _reset_rate_limiter():
    limiter.reset()
    yield
    limiter.reset()


@pytest.fixture(autouse=True)
def _clean_token_env(monkeypatch):
    monkeypatch.delenv("SANDBOX_EXECUTOR_MANAGER_API_TOKEN", raising=False)
    yield
