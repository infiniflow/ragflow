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

import json

import pandas as pd
import pytest

from agent.component.base import ComponentParamBase


class _ParamWithDataFrame(ComponentParamBase):
    """Minimal param class with a DataFrame attribute for testing."""

    def __init__(self):
        super().__init__()
        self.data = pd.DataFrame({"a": [1, 2], "b": [3, 4]})

    def check(self):
        pass  # Minimal param for testing; no validation needed


class _ParamWithDataFrameInOutputs(ComponentParamBase):
    """Param with outputs dict containing a DataFrame as value (simulates agent output)."""

    def __init__(self):
        super().__init__()
        self.outputs = {
            "result": {"value": pd.DataFrame({"x": [10], "y": [20]}), "type": "DataFrame"}
        }

    def check(self):
        pass  # Minimal param for testing; no validation needed


class TestComponentParamBaseAsDictDataFrame:
    """Tests for ComponentParamBase.as_dict() with pandas DataFrame (fixes #4588)."""

    def test_dataframe_attribute_converted_to_json_serializable(self):
        """DataFrame in param attribute is converted to list of dicts, not raw DataFrame."""
        param = _ParamWithDataFrame()
        result = param.as_dict()
        assert "data" in result
        assert result["data"] == [{"a": 1, "b": 3}, {"a": 2, "b": 4}]
        # Must be JSON serializable (no DataFrame remains)
        json_str = json.dumps(result, ensure_ascii=False)
        parsed = json.loads(json_str)
        assert parsed["data"] == [{"a": 1, "b": 3}, {"a": 2, "b": 4}]

    def test_dataframe_in_outputs_converted_to_json_serializable(self):
        """DataFrame in outputs dict (e.g. from agent output) is converted correctly."""
        param = _ParamWithDataFrameInOutputs()
        result = param.as_dict()
        assert "outputs" in result
        assert result["outputs"]["result"]["value"] == [{"x": 10, "y": 20}]
        json_str = json.dumps(result, ensure_ascii=False)
        parsed = json.loads(json_str)
        assert parsed["outputs"]["result"]["value"] == [{"x": 10, "y": 20}]

    def test_str_uses_as_dict_without_typeerror(self):
        """__str__ (json.dumps(as_dict())) does not raise TypeError for DataFrame."""
        param = _ParamWithDataFrame()
        s = str(param)
        parsed = json.loads(s)
        assert parsed["data"] == [{"a": 1, "b": 3}, {"a": 2, "b": 4}]
