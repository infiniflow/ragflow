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

PROJECT_BASE = None


def _default_project_base_directory():
    return os.path.abspath(
        os.path.join(
            os.path.dirname(os.path.realpath(__file__)),
            os.pardir,
        )
    )


def get_project_base_directory(*args):
    global PROJECT_BASE

    project_base = os.getenv("RAG_PROJECT_BASE") or os.getenv("RAG_DEPLOY_BASE")
    if not project_base:
        if PROJECT_BASE is None:
            PROJECT_BASE = _default_project_base_directory()
        project_base = PROJECT_BASE

    if args:
        return os.path.join(project_base, *args)
    return project_base


def traversal_files(base):
    for root, ds, fs in os.walk(base):
        for f in fs:
            fullname = os.path.join(root, f)
            yield fullname
