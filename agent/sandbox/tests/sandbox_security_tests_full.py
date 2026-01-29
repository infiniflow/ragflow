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
import base64
import os
import textwrap
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from enum import Enum
from typing import Dict, Optional

import requests
from pydantic import BaseModel

API_URL = os.getenv("SANDBOX_API_URL", "http://localhost:9385/run")
TIMEOUT = 15
MAX_WORKERS = 5


class ResultStatus(str, Enum):
    SUCCESS = "success"
    PROGRAM_ERROR = "program_error"
    RESOURCE_LIMIT_EXCEEDED = "resource_limit_exceeded"
    UNAUTHORIZED_ACCESS = "unauthorized_access"
    RUNTIME_ERROR = "runtime_error"
    PROGRAM_RUNNER_ERROR = "program_runner_error"


class ResourceLimitType(str, Enum):
    TIME = "time"
    MEMORY = "memory"
    OUTPUT = "output"


class UnauthorizedAccessType(str, Enum):
    DISALLOWED_SYSCALL = "disallowed_syscall"
    FILE_ACCESS = "file_access"
    NETWORK_ACCESS = "network_access"


class RuntimeErrorType(str, Enum):
    SIGNALLED = "signalled"
    NONZERO_EXIT = "nonzero_exit"


class ExecutionResult(BaseModel):
    status: ResultStatus
    stdout: str
    stderr: str
    exit_code: int
    detail: Optional[str] = None
    resource_limit_type: Optional[ResourceLimitType] = None
    unauthorized_access_type: Optional[UnauthorizedAccessType] = None
    runtime_error_type: Optional[RuntimeErrorType] = None


class TestResult(BaseModel):
    name: str
    passed: bool
    duration: float
    expected_failure: bool = False
    result: Optional[ExecutionResult] = None
    error: Optional[str] = None
    validation_error: Optional[str] = None


def encode_code(code: str) -> str:
    return base64.b64encode(code.encode("utf-8")).decode("utf-8")


def execute_single_test(name: str, code: str, language: str, arguments: dict, expect_fail: bool = False) -> TestResult:
    """Execute a single test case"""
    payload = {
        "code_b64": encode_code(textwrap.dedent(code)),
        "language": language,
        "arguments": arguments,
    }

    test_result = TestResult(name=name, passed=False, duration=0, expected_failure=expect_fail)

    really_processed = False
    try:
        while not really_processed:
            start_time = time.perf_counter()

            resp = requests.post(API_URL, json=payload, timeout=TIMEOUT)
            resp.raise_for_status()
            response_data = resp.json()
            if response_data["exit_code"] == -429:  # too many request
                print(f"[{name}] Reached request limit, retring...")
                time.sleep(0.5)
                continue
            really_processed = True

            print("-------------------")
            print(f"{name}:\n{response_data}")
            print("-------------------")

            test_result.duration = time.perf_counter() - start_time
            test_result.result = ExecutionResult(**response_data)

            # Validate test result expectations
            validate_test_result(name, expect_fail, test_result)

    except requests.exceptions.RequestException as e:
        test_result.duration = time.perf_counter() - start_time
        test_result.error = f"Request failed: {str(e)}"
        test_result.result = ExecutionResult(
            status=ResultStatus.PROGRAM_RUNNER_ERROR,
            stdout="",
            stderr=str(e),
            exit_code=-999,
            detail="request_failed",
        )

    return test_result


def validate_test_result(name: str, expect_fail: bool, test_result: TestResult):
    """Validate if the test result meets expectations"""
    if not test_result.result:
        test_result.passed = False
        test_result.validation_error = "No result returned"
        return

    test_result.passed = test_result.result.status == ResultStatus.SUCCESS
    # General validation logic
    if expect_fail:
        # Tests expected to fail should return a non-success status
        if test_result.passed:
            test_result.validation_error = "Expected failure but actually succeeded"
    else:
        # Tests expected to succeed should return a success status
        if not test_result.passed:
            test_result.validation_error = f"Unexpected failure (status={test_result.result.status})"


def get_test_cases() -> Dict[str, dict]:
    """Return test cases (code, whether expected to fail)"""
    return {
        "1 Infinite loop: Should be forcibly terminated": {
            "code": """
def main():
    while True:
        pass
            """,
            "should_fail": True,
            "arguments": {},
            "language": "python",
        },
        "2 Infinite loop: Should be forcibly terminated": {
            "code": """
def main():
    while True:
        pass
            """,
            "should_fail": True,
            "arguments": {},
            "language": "python",
        },
        "3 Infinite loop: Should be forcibly terminated": {
            "code": """
def main():
    while True:
        pass
            """,
            "should_fail": True,
            "arguments": {},
            "language": "python",
        },
        "4 Infinite loop: Should be forcibly terminated": {
            "code": """
def main():
    while True:
        pass
            """,
            "should_fail": True,
            "arguments": {},
            "language": "python",
        },
        "5 Infinite loop: Should be forcibly terminated": {
            "code": """
def main():
    while True:
        pass
            """,
            "should_fail": True,
            "arguments": {},
            "language": "python",
        },
        "6 Infinite loop: Should be forcibly terminated": {
            "code": """
def main():
    while True:
        pass
            """,
            "should_fail": True,
            "arguments": {},
            "language": "python",
        },
        "7 Normal test: Python without dependencies": {
            "code": """
def main():
    return {"data": "hello, world"}
            """,
            "should_fail": False,
            "arguments": {},
            "language": "python",
        },
        "8 Normal test: Python with pandas, should pass without any error": {
            "code": """
import pandas as pd

def main():
    data = {'Name': ['Alice', 'Bob', 'Charlie'],
            'Age': [25, 30, 35]}
    df = pd.DataFrame(data)
            """,
            "should_fail": False,
            "arguments": {},
            "language": "python",
        },
        "9 Normal test: Nodejs without dependencies, should pass without any error": {
            "code": """
const https = require('https');

async function main(args) {
  return new Promise((resolve, reject) => {
    const req = https.get('https://example.com/', (res) => {
      let data = '';

      res.on('data', (chunk) => {
        data += chunk;
      });

      res.on('end', () => {
        clearTimeout(timeout);
        console.log('Body:', data);
        resolve(data);
      });
    });

    const timeout = setTimeout(() => {
      req.destroy(new Error('Request timeout after 10s'));
    }, 10000);

    req.on('error', (err) => {
      clearTimeout(timeout);
      console.error('Error:', err.message);
      reject(err);
    });
  });
}

module.exports = { main };
            """,
            "should_fail": False,
            "arguments": {},
            "language": "nodejs",
        },
        "10 Normal test: Nodejs with axios, should pass without any error": {
            "code": """
const axios = require('axios');

async function main(args) {
  try {
    const response = await axios.get('https://example.com/', {
      timeout: 10000
    });
    console.log('Body:', response.data);
  } catch (error) {
    console.error('Error:', error.message);
  }
}

module.exports = { main };
            """,
            "should_fail": False,
            "arguments": {},
            "language": "nodejs",
        },
        "11 Dangerous import: Should fail due to os module import": {
            "code": """
import os

def main():
    pass
            """,
            "should_fail": True,
            "arguments": {},
            "language": "python",
        },
        "12 Dangerous import from subprocess: Should fail due to subprocess import": {
            "code": """
from subprocess import Popen

def main():
    pass
            """,
            "should_fail": True,
            "arguments": {},
            "language": "python",
        },
        "13 Dangerous call: Should fail due to eval function call": {
            "code": """
def main():
    eval('os.system("echo hello")')
            """,
            "should_fail": True,
            "arguments": {},
            "language": "python",
        },
        "14 Dangerous attribute access: Should fail due to shutil.rmtree": {
            "code": """
import shutil

def main():
    shutil.rmtree('/some/path')
            """,
            "should_fail": True,
            "arguments": {},
            "language": "python",
        },
        "15 Dangerous binary operation: Should fail due to unsafe concatenation leading to eval": {
            "code": """
def main():
    dangerous_string = "os." + "system"
    eval(dangerous_string + '("echo hello")')
            """,
            "should_fail": True,
            "arguments": {},
            "language": "python",
        },
        "16 Dangerous function definition: Should fail due to user-defined eval function": {
            "code": """
def eval_function():
    eval('os.system("echo hello")')

def main():
    eval_function()
            """,
            "should_fail": True,
            "arguments": {},
            "language": "python",
        },
        "17 Memory exhaustion(256m): Should fail due to exceeding memory limit(try to allocate 300m)": {
            "code": """
def main():
    x = ['a' * 1024 * 1024] * 300  # 300MB
""",
            "should_fail": True,
            "arguments": {},
            "language": "python",
        },
    }


def print_test_report(results: Dict[str, TestResult]):
    print("\n=== 🔍 Test Report ===")

    max_name_len = max(len(name) for name in results)

    for name, result in results.items():
        status = "✅" if result.passed else "❌"
        if result.expected_failure:
            status = "⚠️" if result.passed else "✓"  # Expected failure case

        print(f"{status} {name.ljust(max_name_len)} {result.duration:.2f}s")

        if result.error:
            print(f"   REQUEST ERROR: {result.error}")
        if result.validation_error:
            print(f"   VALIDATION ERROR: {result.validation_error}")

        if result.result and not result.passed:
            print(f"   STATUS: {result.result.status}")
            if result.result.stderr:
                print(f"   STDERR: {result.result.stderr[:200]}...")
            if result.result.detail:
                print(f"   DETAIL: {result.result.detail}")

    passed = sum(1 for r in results.values() if ((not r.expected_failure and r.passed) or (r.expected_failure and not r.passed)))
    failed = len(results) - passed

    print("\n=== 📊 Statistics ===")
    print(f"✅ Passed: {passed}")
    print(f"❌ Failed: {failed}")
    print(f"📌 Total: {len(results)}")


def main():
    print(f"🔐 Starting sandbox security tests (API: {API_URL})")
    print(f"🚀 Concurrent threads: {MAX_WORKERS}")

    test_cases = get_test_cases()
    results = {}

    with ThreadPoolExecutor(max_workers=MAX_WORKERS) as executor:
        futures = {}
        for name, detail in test_cases.items():
            # ✅ Log when a task is submitted
            print(f"✅ Task submitted: {name}")
            time.sleep(0.4)
            future = executor.submit(execute_single_test, name, detail["code"], detail["language"], detail["arguments"], detail["should_fail"])
            futures[future] = name

        print("\n=== 🚦 Test Progress ===")
        for i, future in enumerate(as_completed(futures)):
            name = futures[future]
            print(f"  {i + 1}/{len(test_cases)} completed: {name}")
            try:
                results[name] = future.result()
            except Exception as e:
                print(f"⚠️ Test {name} execution exception: {str(e)}")
                results[name] = TestResult(name=name, passed=False, duration=0, error=f"Execution exception: {str(e)}")

    print_test_report(results)

    if any(not r.passed and not r.expected_failure for r in results.values()):
        exit(1)


if __name__ == "__main__":
    main()
