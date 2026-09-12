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

import argparse
import logging
from collections.abc import Sequence

logger = logging.getLogger(__name__)


def parse_task_executor_args(argv: Sequence[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Task Executor")
    parser.add_argument("-i", "--index", type=str, default="0")
    parser.add_argument("-t", "--type", type=str, default="common", help="[common, graphrag, raptor, resume]")
    parser.add_argument("legacy_index", nargs="?", default=None)
    return parser.parse_args(argv)


def resolve_task_executor_index(args: argparse.Namespace) -> str:
    return args.legacy_index or args.index


def log_task_executor_index_usage(args: argparse.Namespace) -> None:
    if args.legacy_index is None:
        return

    if args.index not in ("0", args.legacy_index):
        logger.warning(
            "Conflicting task executor indices: -i/--index=%s, positional=%s; using positional index",
            args.index,
            args.legacy_index,
        )
        return

    logger.info("Using legacy positional task executor index: %s", args.legacy_index)
