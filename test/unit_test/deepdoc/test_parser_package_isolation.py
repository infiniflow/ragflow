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

import importlib
import sys
from types import ModuleType

import pytest

from test.unit_test.deepdoc import conftest as deepdoc_conftest


def test_production_parser_package_restores_stub_when_import_fails(monkeypatch):
    deepdoc_package = importlib.import_module("deepdoc")
    parser_stub = ModuleType("deepdoc.parser")
    parser_stub.__path__ = []
    pdf_parser_stub = ModuleType("deepdoc.parser.pdf_parser")

    monkeypatch.setitem(sys.modules, "deepdoc.parser", parser_stub)
    monkeypatch.setitem(sys.modules, "deepdoc.parser.pdf_parser", pdf_parser_stub)
    monkeypatch.setattr(deepdoc_package, "parser", parser_stub, raising=False)

    runtime_import = deepdoc_conftest.importlib.import_module

    def fail_production_parser_import(name):
        if name == "deepdoc.parser":
            raise ImportError("forced production parser import failure")
        return runtime_import(name)

    monkeypatch.setattr(deepdoc_conftest.importlib, "import_module", fail_production_parser_import)

    with pytest.raises(ImportError, match="forced production parser import failure"), deepdoc_conftest._production_parser_package():
        pass

    assert sys.modules["deepdoc.parser"] is parser_stub
    assert sys.modules["deepdoc.parser.pdf_parser"] is pdf_parser_stub
    assert deepdoc_package.parser is parser_stub
