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
from agent.component.fillup import UserFillUpParam, UserFillUp
from api.db.services.file_service import FileService


class BeginParam(UserFillUpParam):

    """
    Define the Begin component parameters.
    """
    def __init__(self):
        super().__init__()
        self.mode = "conversational"
        self.prologue = "Hi! I'm your smart assistant. What can I do for you?"

    def check(self):
        self.check_valid_value(self.mode, "The 'mode' should be either `conversational` or `task`", ["conversational", "task","Webhook"])

    def get_input_form(self) -> dict[str, dict]:
        return getattr(self, "inputs")


class Begin(UserFillUp):
    component_name = "Begin"

    def _invoke(self, **kwargs):
        if self.check_if_canceled("Begin processing"):
            return

        for k, v in kwargs.get("inputs", {}).items():
            if self.check_if_canceled("Begin processing"):
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
            self.set_input_value(k, v)

    def thoughts(self) -> str:
        return ""
