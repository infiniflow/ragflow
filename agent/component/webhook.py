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
from agent.component.base import ComponentParamBase, ComponentBase


class WebhookParam(ComponentParamBase):

    """
    Define the Begin component parameters.
    """
    def __init__(self):
        super().__init__()

    def get_input_form(self) -> dict[str, dict]:
        return getattr(self, "inputs")


class Webhook(ComponentBase):
    component_name = "Webhook"

    def _invoke(self, **kwargs):
        pass

    def thoughts(self) -> str:
        return ""
