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
from agent.component.base import ComponentBase, ComponentParamBase


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
        for k, v in kwargs.get("inputs", {}).items():
            self.set_output(k, v)

    def thoughts(self) -> str:
        return "Waiting for your input..."


