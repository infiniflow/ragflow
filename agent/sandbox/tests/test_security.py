#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
import sys
from pathlib import Path


EXECUTOR_MANAGER_ROOT = Path(__file__).resolve().parents[1] / "executor_manager"
if str(EXECUTOR_MANAGER_ROOT) not in sys.path:
    sys.path.insert(0, str(EXECUTOR_MANAGER_ROOT))

from models.enums import SupportLanguage  # noqa: E402
from services.security import analyze_code_security  # noqa: E402


def test_javascript_child_process_is_rejected():
    is_safe, issues = analyze_code_security(
        "const cp = require('child_process'); async function main() { return 'ok'; }",
        SupportLanguage.NODEJS,
    )

    assert is_safe is False
    assert any("child_process" in issue for issue, _ in issues)


def test_javascript_eval_is_rejected():
    is_safe, issues = analyze_code_security(
        "async function main() { return eval('1+1'); }",
        SupportLanguage.NODEJS,
    )

    assert is_safe is False
    assert any("eval" in issue.lower() for issue, _ in issues)


def test_javascript_safe_code_still_passes():
    is_safe, issues = analyze_code_security(
        "async function main(args) { return { answer: args.value ?? null }; }",
        SupportLanguage.NODEJS,
    )

    assert is_safe is True
    assert issues == []
