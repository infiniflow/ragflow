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

"""Service-free checks for full collection and isolated SDK execution."""

import os
import subprocess
import sys
from types import SimpleNamespace
from unittest.mock import MagicMock, Mock, call

import pytest

from tools.ci import run_sdk_tests as runner


@pytest.mark.parametrize(
    "level,expected",
    [
        ("p1", {"p1"}),
        ("p2", {"p1", "p2"}),
        ("p3", {"p1", "p2", "p3"}),
        ("all", {"p1", "p2", "p3", "unmarked"}),
    ],
)
def test_priority_collection_includes_unmarked_only_with_all(tmp_path, level, expected):
    (tmp_path / "pytest.ini").write_text("[pytest]\nmarkers =\n    p1\n    p2\n    p3\n")
    (tmp_path / "test_priority.py").write_text(
        "import pytest\n@pytest.mark.p1\ndef test_p1(): pass\n@pytest.mark.p2\ndef test_p2(): pass\n@pytest.mark.p3\ndef test_p3(): pass\ndef test_unmarked(): pass\n"
    )
    result = subprocess.run(
        [
            sys.executable,
            "-m",
            "pytest",
            "-q",
            "--collect-only",
            "-p",
            "tools.ci.pytest_options",
            "--level",
            level,
            "-c",
            str(tmp_path / "pytest.ini"),
            "--confcutdir",
            str(tmp_path),
            str(tmp_path),
        ],
        cwd=runner.ROOT,
        env={**os.environ, "PYTEST_DISABLE_PLUGIN_AUTOLOAD": "1"},
        capture_output=True,
        text=True,
    )
    assert result.returncode == 0, result.stdout + result.stderr
    collected = {line.split("::test_", 1)[1] for line in result.stdout.splitlines() if "::test_" in line}
    assert collected == expected


@pytest.fixture
def isolated_suite(tmp_path, monkeypatch):
    suite = tmp_path / "sdk"
    folders = [suite / "test_a", suite / "test_b"]
    for folder in folders:
        folder.mkdir(parents=True)
        (folder / "pytest.ini").write_text("[pytest]\n")
        (folder / "test_contract.py").write_text("def test_contract(): pass\n")
    monkeypatch.setattr(runner, "ROOT", tmp_path)
    monkeypatch.setattr(runner, "SUITE", suite)
    prepare = Mock(side_effect=lambda name: {"RAGFLOW_SDK_API_KEY": name})
    monkeypatch.setitem(sys.modules, "tools.ci.sdk_test_environment", SimpleNamespace(prepare_environment=prepare))
    execute = Mock()
    monkeypatch.setattr(runner.subprocess, "Popen", execute)
    return folders, prepare, execute


def child_process(code=0, output=()):
    process = MagicMock()
    process.__enter__.return_value = process
    process.stdout = output
    process.wait.return_value = code
    return process


@pytest.mark.parametrize("codes,expected", [([0, 0], 0), ([1, 0], 1), ([0, 5], 1)])
def test_runner_attempts_every_folder_and_propagates_failures(isolated_suite, codes, expected):
    folders, prepare, execute = isolated_suite
    environments = []

    def run_folder(command, *, cwd, env, stdout, stderr, text):
        environments.append(env.copy())
        return child_process(codes[len(environments) - 1])

    execute.side_effect = run_folder
    assert runner.run(["--level=all"]) == expected
    assert prepare.call_args_list == [call(folder.name) for folder in folders]
    assert execute.call_count == len(folders)
    for invocation, folder, environment in zip(execute.call_args_list, folders, environments):
        relative = str(folder.relative_to(runner.ROOT))
        command = invocation.args[0]
        assert command[:3] == [sys.executable, "-m", "pytest"]
        assert command[command.index("-c") + 1] == f"{relative}/pytest.ini"
        assert command[command.index("--confcutdir") + 1] == relative
        assert relative in command
        assert "--level=all" in command
        assert invocation.kwargs["cwd"] == runner.ROOT
        assert invocation.kwargs["stdout"] == subprocess.PIPE
        assert invocation.kwargs["stderr"] == subprocess.STDOUT
        assert invocation.kwargs["text"] is True
        assert environment["RAGFLOW_SDK_API_KEY"] == folder.name


def test_collection_does_not_provision_backend(isolated_suite):
    folders, prepare, execute = isolated_suite
    execute.return_value = child_process()
    assert runner.run(["--collect-only", "--level=all"]) == 0
    prepare.assert_not_called()
    assert execute.call_count == len(folders)
    assert all("--collect-only" in invocation.args[0] for invocation in execute.call_args_list)


def test_each_folder_receives_only_its_own_provisioned_environment(isolated_suite, monkeypatch):
    _, prepare, execute = isolated_suite
    monkeypatch.delenv("RAGFLOW_SDK_PIPELINE_ID", raising=False)
    prepare.side_effect = [{"RAGFLOW_SDK_PIPELINE_ID": "first-tenant-pipeline"}, {}]
    execute.return_value = child_process()
    assert runner.run(["--level=all"]) == 0
    first, second = execute.call_args_list
    assert first.kwargs["env"]["RAGFLOW_SDK_PIPELINE_ID"] == "first-tenant-pipeline"
    assert "RAGFLOW_SDK_PIPELINE_ID" not in second.kwargs["env"]


def test_redacts_credentials_before_stdout_and_preserves_child_environment(isolated_suite, monkeypatch, capsys):
    folders, prepare, execute = isolated_suite
    secret = "sdk%token\r\nprivate"
    prepare.side_effect = None
    prepare.return_value = {"RAGFLOW_SDK_API_KEY": secret, "HOST_ADDRESS": "http://sdk.invalid"}
    monkeypatch.setenv("ZHIPU_AI_API_KEY", "provider-secret")
    environments = []

    def run_folder(command, *, cwd, env, stdout, stderr, text):
        environments.append(env.copy())
        return child_process(output=["header sdk%token\n", "private provider-secret\n", "valid http://sdk.invalid\n"])

    execute.side_effect = run_folder
    assert runner.run(["--level=all"]) == 0
    assert execute.call_count == len(folders)
    output = capsys.readouterr().out
    for value in ("sdk%token", "private", "provider-secret", "::add-mask::"):
        assert value not in output
    assert "header ***\n*** ***\nvalid http://sdk.invalid\n" in output
    assert all(environment["RAGFLOW_SDK_API_KEY"] == secret for environment in environments)


@pytest.mark.parametrize("empty_suite", [False, True])
def test_runner_rejects_uncovered_tests_or_no_folders(isolated_suite, empty_suite):
    folders, prepare, execute = isolated_suite
    if empty_suite:
        for folder in folders:
            (folder / "pytest.ini").unlink()
    else:
        (runner.SUITE / "test_uncovered.py").write_text("def test_uncovered(): pass\n")
    with pytest.raises(RuntimeError, match="Every SDK test"):
        runner.run(["--level=all"])
    prepare.assert_not_called()
    execute.assert_not_called()
