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

import logging
import sys

from rag.svr.task_executor_cli import log_task_executor_index_usage, parse_task_executor_args, resolve_task_executor_index


def test_positional_index_is_accepted(caplog, monkeypatch):
    monkeypatch.setattr(sys, "argv", ["task_executor.py", "3"])
    args = parse_task_executor_args()

    with caplog.at_level(logging.INFO):
        log_task_executor_index_usage(args)

    assert resolve_task_executor_index(args) == "3"
    assert "Using legacy positional task executor index: 3" in caplog.messages


def test_index_option_is_accepted(caplog, monkeypatch):
    monkeypatch.setattr(sys, "argv", ["task_executor.py", "-i", "5"])
    args = parse_task_executor_args()

    with caplog.at_level(logging.INFO):
        log_task_executor_index_usage(args)

    assert resolve_task_executor_index(args) == "5"
    assert not caplog.messages


def test_matching_index_option_and_positional_index_do_not_warn(caplog, monkeypatch):
    monkeypatch.setattr(sys, "argv", ["task_executor.py", "-i", "3", "3"])
    args = parse_task_executor_args()

    with caplog.at_level(logging.WARNING):
        log_task_executor_index_usage(args)

    assert resolve_task_executor_index(args) == "3"
    assert not caplog.messages


def test_conflicting_indices_use_positional_index_and_warn(caplog, monkeypatch):
    monkeypatch.setattr(sys, "argv", ["task_executor.py", "-i", "5", "3"])
    args = parse_task_executor_args()

    with caplog.at_level(logging.WARNING):
        log_task_executor_index_usage(args)

    assert resolve_task_executor_index(args) == "3"
    assert "Conflicting task executor indices: -i/--index=5, positional=3; using positional index" in caplog.messages
