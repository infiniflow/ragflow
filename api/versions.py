#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
import subprocess

RAGFLOW_VERSION_INFO = "unknown"


def get_ragflow_version() -> str:
    global RAGFLOW_VERSION_INFO
    if RAGFLOW_VERSION_INFO != "unknown":
        return RAGFLOW_VERSION_INFO
    version_path = os.path.abspath(
        os.path.join(
            os.path.dirname(os.path.realpath(__file__)), os.pardir, "VERSION"
        )
    )
    if os.path.exists(version_path):
        with open(version_path, "r") as f:
            RAGFLOW_VERSION_INFO = f.read().strip()
    else:
        RAGFLOW_VERSION_INFO = get_closest_tag_and_count()
        LIGHTEN = int(os.environ.get("LIGHTEN", "0"))
        RAGFLOW_VERSION_INFO += " slim" if LIGHTEN == 1 else " full"
    return RAGFLOW_VERSION_INFO


def get_closest_tag_and_count():
    try:
        # Get the current commit hash
        commit_id = (
            subprocess.check_output(["git", "rev-parse", "--short", "HEAD"])
            .strip()
            .decode("utf-8")
        )
        # Get the closest tag
        closest_tag = (
            subprocess.check_output(["git", "describe", "--tags", "--abbrev=0"])
            .strip()
            .decode("utf-8")
        )
        # Get the commit count since the closest tag
        process = subprocess.Popen(
            ["git", "rev-list", "--count", f"{closest_tag}..HEAD"],
            stdout=subprocess.PIPE,
        )
        commits_count, _ = process.communicate()
        commits_count = int(commits_count.strip())

        if commits_count == 0:
            return closest_tag
        else:
            return f"{commit_id}({closest_tag}~{commits_count})"
    except Exception:
        return "unknown"
