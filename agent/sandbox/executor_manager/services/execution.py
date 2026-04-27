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
import asyncio
import base64
import json
import os
import time
import uuid
from core.config import TIMEOUT
from core.container import allocate_container_blocking, release_container
from core.logger import logger
from models.enums import ResourceLimitType, ResultStatus, RuntimeErrorType, SupportLanguage, UnauthorizedAccessType
from models.schemas import ArtifactItem, CodeExecutionRequest, CodeExecutionResult, ExecutionStructuredResult
from utils.common import async_run_command

RESULT_MARKER_PREFIX = "__RAGFLOW_RESULT__:"


def _extract_result_envelope(stdout: str) -> tuple[str, ExecutionStructuredResult | None]:
    if not stdout:
        return "", None

    cleaned_lines: list[str] = []
    envelope: ExecutionStructuredResult | None = None

    for line in str(stdout).splitlines():
        if line.startswith(RESULT_MARKER_PREFIX):
            payload_b64 = line[len(RESULT_MARKER_PREFIX) :].strip()
            if not payload_b64:
                continue
            try:
                payload = base64.b64decode(payload_b64).decode("utf-8")
                envelope = ExecutionStructuredResult.model_validate_json(payload)
            except Exception as exc:
                logger.warning(f"Failed to decode structured result marker: {exc}")
                cleaned_lines.append(line)
            continue
        cleaned_lines.append(line)

    cleaned_stdout = "\n".join(cleaned_lines)
    if stdout.endswith("\n") and cleaned_stdout and not cleaned_stdout.endswith("\n"):
        cleaned_stdout += "\n"
    return cleaned_stdout, envelope


def _build_execution_bundle(req: CodeExecutionRequest, workdir: str) -> dict[str, str | bytes]:
    arguments = req.arguments or {}
    args_source = json.dumps(arguments, ensure_ascii=False)
    args_name = "args.json"
    code_bytes = base64.b64decode(req.code_b64)

    if req.language == SupportLanguage.PYTHON:
        code_name = "main.py"
        runner_name = "runner.py"
        runner_source = f"""import base64
import json
import os
import sys

os.makedirs(os.path.join(os.getcwd(), "artifacts"), exist_ok=True)

sys.path.insert(0, os.path.dirname(__file__))
from main import main

RESULT_MARKER_PREFIX = {RESULT_MARKER_PREFIX!r}


def emit_result(value):
    payload = json.dumps(
        {{
            "present": True,
            "value": value,
            "type": "json",
        }},
        ensure_ascii=False,
        separators=(",", ":"),
    )
    print(RESULT_MARKER_PREFIX + base64.b64encode(payload.encode("utf-8")).decode("ascii"))


if __name__ == "__main__":
    with open(os.path.join(os.path.dirname(__file__), "args.json"), encoding="utf-8") as f:
        args = json.load(f)
    result = main(**args)
    emit_result(result)
"""
    elif req.language == SupportLanguage.NODEJS:
        code_name = "main.js"
        runner_name = "runner.js"
        runner_source = """
const fs = require('fs');
const path = require('path');

const args = JSON.parse(fs.readFileSync(path.join(__dirname, 'args.json'), 'utf8'));
const mainPath = path.join(__dirname, 'main.js');
const RESULT_MARKER_PREFIX = '__RESULT_MARKER_PREFIX__';

function isPromise(value) {
    return Boolean(value && typeof value.then === 'function');
}

function emitResult(value) {
    if (typeof value === 'undefined') {
        console.error('Error: main() must return a value. Use null for an empty result.');
        process.exit(1);
    }

    const payload = JSON.stringify({ present: true, value, type: 'json' });
    if (typeof payload === 'undefined') {
        console.error('Error: main() returned a non-JSON-serializable value.');
        process.exit(1);
    }

    console.log(RESULT_MARKER_PREFIX + Buffer.from(payload, 'utf8').toString('base64'));
}

if (fs.existsSync(mainPath)) {
    const mod = require(mainPath);
    const main = typeof mod === 'function' ? mod : mod.main;

    if (typeof main !== 'function') {
        console.error('Error: main is not a function');
        process.exit(1);
    }

    if (typeof args === 'object' && args !== null) {
        try {
            const result = Promise.resolve(main(args));
            if (isPromise(result)) {
                result.then(output => {
                    emitResult(output);
                }).catch(err => {
                    console.error('Error in async main function:', err);
                    process.exit(1);
                });
            } else {
                emitResult(result);
            }
        } catch (err) {
            console.error('Error when executing main:', err);
            process.exit(1);
        }
    } else {
        console.error('Error: args is not a valid object:', args);
        process.exit(1);
    }
} else {
    console.error('main.js not found in the current directory');
    process.exit(1);
}
"""
        runner_source = runner_source.replace("__RESULT_MARKER_PREFIX__", RESULT_MARKER_PREFIX)
    else:
        assert False, "Will never reach here"

    return {
        "code_name": code_name,
        "code_bytes": code_bytes,
        "runner_name": runner_name,
        "runner_source": runner_source,
        "args_name": args_name,
        "args_source": args_source,
    }


def _build_container_run_args(language: SupportLanguage, task_id: str, container: str, runner_name: str) -> list[str]:
    run_args = [
        "docker",
        "exec",
        "--workdir",
        f"/workspace/{task_id}",
        container,
        "timeout",
        str(TIMEOUT),
        language,
    ]
    if language == SupportLanguage.PYTHON:
        run_args.extend(["-I", "-B"])
    run_args.append(runner_name)
    return run_args


async def execute_code(req: CodeExecutionRequest):
    language = req.language
    container = await allocate_container_blocking(language)
    if not container:
        return CodeExecutionResult(
            status=ResultStatus.PROGRAM_RUNNER_ERROR,
            stdout="",
            stderr="Container pool is busy",
            exit_code=-10,
            detail="no_available_container",
        )

    task_id = str(uuid.uuid4())
    workdir = f"/tmp/sandbox_{task_id}"
    os.makedirs(workdir, mode=0o700, exist_ok=True)

    try:
        bundle = _build_execution_bundle(req, workdir)
        code_name = str(bundle["code_name"])
        runner_name = str(bundle["runner_name"])

        code_path = os.path.join(workdir, code_name)
        with open(code_path, "wb") as f:
            f.write(bundle["code_bytes"])

        runner_path = os.path.join(workdir, runner_name)
        with open(runner_path, "w", encoding="utf-8") as f:
            f.write(str(bundle["runner_source"]))

        args_path = os.path.join(workdir, str(bundle["args_name"]))
        with open(args_path, "w", encoding="utf-8") as f:
            f.write(str(bundle["args_source"]))

        returncode, _, stderr = await async_run_command("docker", "exec", container, "mkdir", "-p", f"/workspace/{task_id}", timeout=5)
        if returncode != 0:
            raise RuntimeError(f"Directory creation failed: {stderr}")

        tar_proc = await asyncio.create_subprocess_exec(
            "tar", "czf", "-", "-C", workdir, code_name, runner_name, str(bundle["args_name"]), stdout=asyncio.subprocess.PIPE
        )
        tar_stdout, _ = await tar_proc.communicate()

        docker_proc = await asyncio.create_subprocess_exec(
            "docker", "exec", "-i", container, "tar", "xzf", "-", "-C", f"/workspace/{task_id}", stdin=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.PIPE
        )
        stdout, stderr = await docker_proc.communicate(input=tar_stdout)

        if docker_proc.returncode != 0:
            raise RuntimeError(stderr.decode())

        start_time = time.time()
        try:
            arguments = req.arguments or {}
            logger.info("Passed in args keys=%s size_bytes=%s", list(arguments.keys()), len(json.dumps(arguments, ensure_ascii=False).encode("utf-8")))
            run_args = _build_container_run_args(language=language, task_id=task_id, container=container, runner_name=runner_name)

            returncode, stdout, stderr = await async_run_command(
                *run_args,
                timeout=TIMEOUT + 5,
            )

            time_used_ms = (time.time() - start_time) * 1000

            logger.info("----------------------------------------------")
            logger.info(f"Code: {str(base64.b64decode(req.code_b64))}")
            logger.info(f"{returncode=}")
            logger.info(f"{stdout=}")
            logger.info(f"{stderr=}")

            if returncode == 0:
                clean_stdout, structured_result = _extract_result_envelope(stdout)
                artifacts = await _collect_artifacts(container, task_id, workdir)
                return CodeExecutionResult(
                    status=ResultStatus.SUCCESS,
                    stdout=clean_stdout,
                    stderr=stderr,
                    exit_code=0,
                    time_used_ms=time_used_ms,
                    artifacts=artifacts,
                    result=structured_result,
                )
            elif returncode == 124:
                return CodeExecutionResult(
                    status=ResultStatus.RESOURCE_LIMIT_EXCEEDED,
                    stdout="",
                    stderr="Execution timeout",
                    exit_code=-124,
                    resource_limit_type=ResourceLimitType.TIME,
                    time_used_ms=time_used_ms,
                )
            elif returncode == 137:
                return CodeExecutionResult(
                    status=ResultStatus.RESOURCE_LIMIT_EXCEEDED,
                    stdout="",
                    stderr="Memory limit exceeded (killed by OOM)",
                    exit_code=-137,
                    resource_limit_type=ResourceLimitType.MEMORY,
                    time_used_ms=time_used_ms,
                )
            return analyze_error_result(stderr, returncode)

        except asyncio.TimeoutError:
            await async_run_command("docker", "exec", container, "pkill", "-9", language)
            return CodeExecutionResult(
                status=ResultStatus.RESOURCE_LIMIT_EXCEEDED,
                stdout="",
                stderr="Execution timeout",
                exit_code=-1,
                resource_limit_type=ResourceLimitType.TIME,
                time_used_ms=(time.time() - start_time) * 1000,
            )

    except Exception as e:
        logger.error(f"Execution exception: {str(e)}")
        return CodeExecutionResult(status=ResultStatus.PROGRAM_RUNNER_ERROR, stdout="", stderr=str(e), exit_code=-3, detail="internal_error")

    finally:
        cleanup_tasks = [async_run_command("docker", "exec", container, "rm", "-rf", f"/workspace/{task_id}"), async_run_command("rm", "-rf", workdir)]
        await asyncio.gather(*cleanup_tasks, return_exceptions=True)
        await release_container(container, language)


ALLOWED_ARTIFACT_EXTENSIONS = {
    ".png": "image/png",
    ".jpg": "image/jpeg",
    ".jpeg": "image/jpeg",
    ".svg": "image/svg+xml",
    ".pdf": "application/pdf",
    ".csv": "text/csv",
    ".json": "application/json",
    ".html": "text/html",
}
MAX_ARTIFACT_COUNT = 10
MAX_ARTIFACT_SIZE = 10 * 1024 * 1024  # 10MB per file


async def _collect_artifacts(container: str, task_id: str, host_workdir: str) -> list[ArtifactItem]:
    artifacts_path = f"/workspace/{task_id}/artifacts"

    # List files in the artifacts directory inside the container
    returncode, stdout, _ = await async_run_command(
        "docker", "exec", container, "find", artifacts_path,
        "-maxdepth", "1", "-type", "f", timeout=5,
    )
    if returncode != 0 or not stdout.strip():
        return []

    raw_names = [line.split("/")[-1] for line in stdout.strip().splitlines() if line.strip()]
    # Sanitize: reject names with path traversal or control characters
    filenames = [n for n in raw_names if n and "/" not in n and "\\" not in n and ".." not in n and not n.startswith(".")]
    if not filenames:
        return []

    items: list[ArtifactItem] = []

    for fname in filenames[:MAX_ARTIFACT_COUNT]:
        ext = os.path.splitext(fname)[1].lower()
        mime_type = ALLOWED_ARTIFACT_EXTENSIONS.get(ext)
        if not mime_type:
            logger.warning(f"Skipping artifact with disallowed extension: {fname}")
            continue

        file_path = f"{artifacts_path}/{fname}"

        # Check file size inside the container
        returncode, size_str, _ = await async_run_command(
            "docker", "exec", container, "stat", "-c", "%s", file_path, timeout=5,
        )
        if returncode != 0:
            logger.warning(f"Failed to stat artifact {fname}")
            continue

        file_size = int(size_str.strip())
        if file_size > MAX_ARTIFACT_SIZE:
            logger.warning(f"Artifact {fname} too large ({file_size} bytes), skipping")
            continue
        if file_size == 0:
            continue

        # Read file content via docker exec (docker cp doesn't work with gVisor tmpfs)
        returncode, content_b64, stderr = await async_run_command(
            "docker", "exec", container, "base64", file_path, timeout=30,
        )
        if returncode != 0:
            logger.warning(f"Failed to read artifact {fname}: {stderr}")
            continue

        content_b64 = content_b64.replace("\n", "").strip()

        items.append(ArtifactItem(
            name=fname,
            mime_type=mime_type,
            size=file_size,
            content_b64=content_b64,
        ))
        logger.info(f"Collected artifact: {fname} ({file_size} bytes, {mime_type})")

    return items


def analyze_error_result(stderr: str, exit_code: int) -> CodeExecutionResult:
    """Analyze the error result and classify it"""
    if "Permission denied" in stderr:
        return CodeExecutionResult(
            status=ResultStatus.UNAUTHORIZED_ACCESS,
            stdout="",
            stderr=stderr,
            exit_code=exit_code,
            unauthorized_access_type=UnauthorizedAccessType.FILE_ACCESS,
        )
    elif "Operation not permitted" in stderr:
        return CodeExecutionResult(
            status=ResultStatus.UNAUTHORIZED_ACCESS,
            stdout="",
            stderr=stderr,
            exit_code=exit_code,
            unauthorized_access_type=UnauthorizedAccessType.DISALLOWED_SYSCALL,
        )
    elif "MemoryError" in stderr:
        return CodeExecutionResult(
            status=ResultStatus.RESOURCE_LIMIT_EXCEEDED,
            stdout="",
            stderr=stderr,
            exit_code=exit_code,
            resource_limit_type=ResourceLimitType.MEMORY,
        )
    else:
        return CodeExecutionResult(
            status=ResultStatus.PROGRAM_ERROR,
            stdout="",
            stderr=stderr,
            exit_code=exit_code,
            runtime_error_type=RuntimeErrorType.NONZERO_EXIT,
        )
