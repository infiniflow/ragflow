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
from models.schemas import ArtifactItem, CodeExecutionRequest, CodeExecutionResult
from utils.common import async_run_command


async def execute_code(req: CodeExecutionRequest):
    """Fully asynchronous execution logic"""
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
        if language == SupportLanguage.PYTHON:
            code_name = "main.py"
            # code
            code_path = os.path.join(workdir, code_name)
            with open(code_path, "wb") as f:
                f.write(base64.b64decode(req.code_b64))
            # runner
            runner_name = "runner.py"
            runner_path = os.path.join(workdir, runner_name)
            with open(runner_path, "w") as f:
                f.write("""import json
import os
import sys

os.makedirs(os.path.join(os.getcwd(), "artifacts"), exist_ok=True)

sys.path.insert(0, os.path.dirname(__file__))
from main import main

if __name__ == "__main__":
    args = json.loads(sys.argv[1])
    result = main(**args)
    if result is not None:
        print(result)
""")

        elif language == SupportLanguage.NODEJS:
            code_name = "main.js"
            code_path = os.path.join(workdir, "main.js")
            with open(code_path, "wb") as f:
                f.write(base64.b64decode(req.code_b64))

            runner_name = "runner.js"
            runner_path = os.path.join(workdir, "runner.js")
            with open(runner_path, "w") as f:
                f.write("""
const fs = require('fs');
const path = require('path');

const args = JSON.parse(process.argv[2]);
const mainPath = path.join(__dirname, 'main.js');

function isPromise(value) {
    return Boolean(value && typeof value.then === 'function');
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
            const result = main(args);
            if (isPromise(result)) {
                result.then(output => {
                    if (output !== null) {
                        console.log(output);
                    }
                }).catch(err => {
                    console.error('Error in async main function:', err);
                });
            } else {
                if (result !== null) {
                    console.log(result);
                }
            }
        } catch (err) {
            console.error('Error when executing main:', err);
        }
    } else {
        console.error('Error: args is not a valid object:', args);
    }
} else {
    console.error('main.js not found in the current directory');
}
""")
        # dirs
        returncode, _, stderr = await async_run_command("docker", "exec", container, "mkdir", "-p", f"/workspace/{task_id}", timeout=5)
        if returncode != 0:
            raise RuntimeError(f"Directory creation failed: {stderr}")

        # archive
        tar_proc = await asyncio.create_subprocess_exec("tar", "czf", "-", "-C", workdir, code_name, runner_name, stdout=asyncio.subprocess.PIPE)
        tar_stdout, _ = await tar_proc.communicate()

        # unarchive
        docker_proc = await asyncio.create_subprocess_exec(
            "docker", "exec", "-i", container, "tar", "xzf", "-", "-C", f"/workspace/{task_id}", stdin=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.PIPE
        )
        stdout, stderr = await docker_proc.communicate(input=tar_stdout)

        if docker_proc.returncode != 0:
            raise RuntimeError(stderr.decode())

        # exec
        start_time = time.time()
        try:
            logger.info(f"Passed in args: {req.arguments}")
            args_json = json.dumps(req.arguments or {})
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
            # flags
            if language == SupportLanguage.PYTHON:
                run_args.extend(["-I", "-B"])
            elif language == SupportLanguage.NODEJS:
                run_args.extend([])
            else:
                assert False, "Will never reach here"
            run_args.extend([runner_name, args_json])

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
            logger.info(f"{args_json=}")

            if returncode == 0:
                artifacts = await _collect_artifacts(container, task_id, workdir)
                return CodeExecutionResult(
                    status=ResultStatus.SUCCESS,
                    stdout=str(stdout),
                    stderr=stderr,
                    exit_code=0,
                    time_used_ms=time_used_ms,
                    artifacts=artifacts,
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
        # cleanup
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
