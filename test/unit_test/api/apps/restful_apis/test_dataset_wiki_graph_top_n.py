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
"""Isolated route regression for #20006; runs without backend services."""

import ast
import asyncio
import logging
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import AsyncMock, Mock


class TestWikiGraphTopN(unittest.TestCase):
    def setUp(self):
        # Execute the real route body without importing the server or its dependencies.
        path = Path(__file__).resolve().parents[5] / "api/apps/restful_apis/dataset_api.py"
        tree = ast.parse(path.read_text(encoding="utf-8"))
        route = next(node for node in tree.body if isinstance(node, ast.AsyncFunctionDef) and node.name == "get_wiki_graph")
        route.decorator_list = []
        self.args = {}
        self.service = AsyncMock(return_value=(True, {"entities": [], "relations": []}))
        self.argument_error = Mock()
        self.success = Mock()
        namespace = {
            "request": SimpleNamespace(args=self.args),
            "dataset_api_service": SimpleNamespace(get_wiki_graph=self.service),
            "get_error_argument_result": self.argument_error,
            "get_error_data_result": Mock(),
            "get_result": self.success,
            "logging": logging,
        }
        exec(compile(ast.Module(body=[route], type_ignores=[]), str(path), "exec"), namespace)  # noqa: S102 - trusted repository source
        self.route = namespace["get_wiki_graph"]

    def test_invalid_budget_is_rejected_before_querying(self):
        invalid_values = ("not-a-number", "1.5", "", " ")
        cases = [{name: value} for name in ("top_n", "topN") for value in invalid_values]
        cases.extend({"top_n": value, "topN": "64"} for value in invalid_values)
        for args in cases:
            with self.subTest(args=args):
                self.args.clear()
                self.args.update(args)
                self.argument_error.reset_mock()
                result = asyncio.run(self.route("tenant-1", "dataset-1"))
                self.argument_error.assert_called_once_with("top_n must be an integer")
                self.assertIs(result, self.argument_error.return_value)
                self.service.assert_not_called()
                self.success.assert_not_called()

    def test_omitted_and_integer_budgets_are_forwarded(self):
        cases = [({}, None), ({"top_n": "32", "topN": "64"}, 32), ({"top_n": "32", "topN": "bad"}, 32)]
        cases.extend(({name: str(value)}, value) for name in ("top_n", "topN") for value in (1, 128, 0, -1, 2048))
        for args, expected in cases:
            with self.subTest(args=args):
                self.args.clear()
                self.args.update(args, node=" entity ", keywords="query")
                self.service.reset_mock()
                self.success.reset_mock()
                result = asyncio.run(self.route("tenant-1", "dataset-1"))
                self.service.assert_awaited_once_with("dataset-1", "tenant-1", node="entity", keywords="query", top_n=expected)
                self.success.assert_called_once_with(data=self.service.return_value[1])
                self.assertIs(result, self.success.return_value)
                self.argument_error.assert_not_called()


if __name__ == "__main__":
    unittest.main()
