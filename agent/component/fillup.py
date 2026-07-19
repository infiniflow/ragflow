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
import json
import re
from functools import partial

from agent.component.base import ComponentParamBase, ComponentBase
from api.db.services.file_service import FileService


class UserFillUpParam(ComponentParamBase):
    def __init__(self):
        super().__init__()
        self.enable_tips = True
        self.tips = "Please fill up the form"
        self.layout_recognize = ""

    def check(self) -> bool:
        return True


class UserFillUp(ComponentBase):
    component_name = "UserFillUp"

    def _merge_runtime_inputs(self, runtime_inputs):
        return runtime_inputs or {}

    def _resolve_input_value(self, value, layout_recognize):
        if isinstance(value, dict) and value.get("type", "").lower().find("file") >= 0:
            if value.get("optional") and value.get("value", None) is None:
                return None

            file_value = value["value"]
            files = file_value if isinstance(file_value, list) else [file_value]
            return FileService.get_files(files, layout_recognize=layout_recognize)

        if isinstance(value, dict):
            raw = value.get("value")
            if value.get("type") == "object" and isinstance(raw, str) and raw.strip():
                try:
                    return json.loads(raw)
                except Exception:
                    return raw
            return raw

        return value

    def _invoke(self, **kwargs):
        if self.check_if_canceled("UserFillUp processing"):
            return

        if self._param.enable_tips:
            content = self._param.tips
            for k, v in self.get_input_elements_from_text(self._param.tips).items():
                v = v["value"]
                ans = ""
                if isinstance(v, partial):
                    for t in v():
                        ans += t
                elif isinstance(v, list):
                    ans = ",".join([str(vv) for vv in v])
                elif not isinstance(v, str):
                    try:
                        ans = json.dumps(v, ensure_ascii=False)
                    except Exception:
                        pass
                else:
                    ans = v
                if not ans:
                    ans = ""
                content = re.sub(r"\{%s\}" % k, ans, content)

            self.set_output("tips", content)
        layout_recognize = self._param.layout_recognize or None
        merged_inputs = self._merge_runtime_inputs(kwargs.get("inputs", {}))
        if not merged_inputs:
            # No fresh user answer was supplied on this entry. Clear any values
            # retained from a previous response so the canvas wait-check treats
            # the form as unsatisfied and pauses for input again. Without this,
            # an Await Response node inside a Loop would only pause on the first
            # iteration and silently reuse the earlier answer afterwards.
            self._clear_form_values()
        for k, v in merged_inputs.items():
            if self.check_if_canceled("UserFillUp processing"):
                return
            resolved = self._resolve_input_value(v, layout_recognize)
            self.set_output(k, resolved)
            self.set_input_value(k, resolved)

    def _clear_form_values(self):
        for field in self.get_input_elements().values():
            if not isinstance(field, dict):
                continue
            field_type = str(field.get("type", "")).lower()
            # An optional file input is already treated as satisfied when empty
            # (see Canvas._is_input_field_satisfied), so clearing it would not
            # force a re-prompt and would only drop a previously uploaded file.
            # Leave it untouched to avoid unexpected data loss.
            if "file" in field_type and field.get("optional"):
                continue
            field["value"] = None

    def thoughts(self) -> str:
        return "Waiting for your input..."
