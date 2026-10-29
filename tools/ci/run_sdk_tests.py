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

"""Run every audited SDK folder with its own fixture/import boundary."""

import os
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
SUITE = ROOT / "test/testcases/test_sdk_api"


def run(pytest_args):
    folders = sorted(path.parent for path in SUITE.glob("*/pytest.ini"))
    covered = {path for folder in folders for path in folder.rglob("test_*.py")}
    if not folders or set(SUITE.rglob("test_*.py")) != covered:
        raise RuntimeError("Every SDK test must belong to an isolated pytest folder")
    env = os.environ.copy()
    failed = False
    for folder in folders:
        if "--collect-only" not in pytest_args:
            from tools.ci.sdk_test_environment import prepare_environment

            env = {**os.environ, **prepare_environment(folder.name)}
        relative = folder.relative_to(ROOT)
        print(f"\n=== Python SDK: {relative} ===", flush=True)
        command = [
            sys.executable,
            "-m",
            "pytest",
            "-s",
            "--tb=short",
            "-p",
            "tools.ci.pytest_options",
            "-c",
            str(relative / "pytest.ini"),
            "--confcutdir",
            str(relative),
            str(relative),
            *pytest_args,
        ]
        secrets = sorted(
            {part for key, value in env.items() if key.endswith("_API_KEY") for part in value.splitlines() if part},
            key=len,
            reverse=True,
        )
        # Redact before tee: GitHub's masking does not protect artifact files.
        with subprocess.Popen(command, cwd=ROOT, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True) as process:
            assert process.stdout is not None
            for line in process.stdout:
                for secret in secrets:
                    line = line.replace(secret, "***")
                print(line, end="", flush=True)
            failed |= process.wait() != 0
    return int(failed)


if __name__ == "__main__":
    raise SystemExit(run(sys.argv[1:] or ["--level=all"]))
