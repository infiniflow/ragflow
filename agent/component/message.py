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
from agent.component.base import ComponentBase, ComponentParamBase
from jinja2 import Template as Jinja2Template


class MessageParam(ComponentParamBase):
    """
    Define the Message component parameters.
    """
    def __init__(self):
        super().__init__()
        self.content = ""
        self.outputs = {
            "content": {
                "type": "str"
            }
        }

    def check(self):
        self.check_empty(self.content, "[Message] Content")
        return True


class Message(ComponentBase):
    component_name = "Message"

    def get_input_elements(self):
        return self.get_input_elements_from_text(self._param.content)

    def _invoke(self):
        content = self._param.content
        kwargs = {}
        for k in self._param.inputs.keys():
            if self._param.inputs[k]["value"] is None:
                self.set_input_value(k, self._canvas.get_variable_value(k))
            kwargs[k] = self._param.inputs[k]["value"]

        template = Jinja2Template(content)

        try:
            content = template.render(kwargs)
        except Exception:
            pass

        for n, v in kwargs.items():
            if not isinstance(v, str):
                try:
                    v = json.dumps(v, ensure_ascii=False)
                except Exception:
                    pass
            content = re.sub(
                r"\{%s\}" % re.escape(n), v, content
            )
            content = re.sub(
                r"(#+)", r" \1 ", content
            )

        self._param.outputs["content"]["value"] = content

