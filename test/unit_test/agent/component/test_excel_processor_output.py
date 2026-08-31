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

import importlib.util
import sys
from io import BytesIO
from pathlib import Path
from types import ModuleType, SimpleNamespace

import openpyxl


def _load_excel_processor(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    class ComponentBase:
        pass

    class ComponentParamBase:
        def __init__(self):
            self.outputs = {}

    base = ModuleType("agent.component.base")
    base.ComponentBase = ComponentBase
    base.ComponentParamBase = ComponentParamBase
    monkeypatch.setitem(sys.modules, "agent.component.base", base)

    file_service = ModuleType("api.db.services.file_service")
    file_service.FileService = SimpleNamespace()
    monkeypatch.setitem(sys.modules, "api.db.services.file_service", file_service)

    api_utils = ModuleType("api.utils.api_utils")
    api_utils.timeout = lambda _seconds: lambda func: func
    monkeypatch.setitem(sys.modules, "api.utils.api_utils", api_utils)

    storage = SimpleNamespace(put=lambda *_args, **_kwargs: None)
    common = ModuleType("common")
    common.settings = SimpleNamespace(STORAGE_IMPL=storage)
    monkeypatch.setitem(sys.modules, "common", common)

    misc_utils = ModuleType("common.misc_utils")
    misc_utils.get_uuid = lambda: "attachment-id"
    monkeypatch.setitem(sys.modules, "common.misc_utils", misc_utils)

    module_path = repo_root / "agent" / "component" / "excel_processor.py"
    spec = importlib.util.spec_from_file_location("test_excel_processor_module", module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "test_excel_processor_module", module)
    spec.loader.exec_module(module)
    return module


def test_output_excel_sanitizes_and_deduplicates_sheet_names(monkeypatch):
    module = _load_excel_processor(monkeypatch)
    stored = {}
    module.settings.STORAGE_IMPL.put = lambda tenant_id, doc_id, data: stored.update(tenant_id=tenant_id, doc_id=doc_id, data=data)

    long_sheet_name = "A" * 40
    data = {
        "Q1: Sales": [{"amount": 10}],
        "Q1/ Sales": [{"amount": 20}],
        "'Sales'": [{"amount": 30}],
        "History": [{"amount": 40}],
        "": [{"amount": 50}],
        long_sheet_name: [{"amount": 60}],
        "Nul\x00Name": [{"amount": 70}],
    }
    component = module.ExcelProcessor.__new__(module.ExcelProcessor)
    component._canvas = SimpleNamespace(
        _tenant_id="tenant-1",
        get_variable_value=lambda _ref: data,
    )
    component._param = SimpleNamespace(
        transform_data="source@data",
        output_format="xlsx",
        output_filename="quarterly",
    )
    component.set_input_value = lambda *_args, **_kwargs: None
    outputs = {}
    component.set_output = outputs.__setitem__

    component._output_excel()

    assert outputs["attachment"] == {
        "doc_id": "attachment-id",
        "format": "xlsx",
        "file_name": "quarterly.xlsx",
    }
    assert stored["tenant_id"] == "tenant-1"
    workbook = openpyxl.load_workbook(BytesIO(stored["data"]), read_only=True)
    try:
        assert workbook.sheetnames == [
            "Q1_ Sales",
            "Q1_ Sales_2",
            "Sales",
            "History_",
            "Sheet",
            "A" * 31,
            "Nul_Name",
        ]
    finally:
        workbook.close()
