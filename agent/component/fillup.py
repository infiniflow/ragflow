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

    def check(self) -> bool:
        return True


class UserFillUp(ComponentBase):
    component_name = "UserFillUp"

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
                content = re.sub(r"\{%s\}"%k, ans, content)

            self.set_output("tips", content)
        for k, v in kwargs.get("inputs", {}).items():
            if self.check_if_canceled("UserFillUp processing"):
                return
            if isinstance(v, dict) and v.get("type", "").lower().find("file") >= 0:
                if v.get("optional") and v.get("value", None) is None:
                    v = None
                else:
                    file_value = v["value"]
                    # Support both single file (backward compatibility) and multiple files
                    files = file_value if isinstance(file_value, list) else [file_value]
                    v = FileService.get_files(files)
            else:
                v = v.get("value")
            self.set_output(k, v)

    def thoughts(self) -> str:
        return "Waiting for your input..."
