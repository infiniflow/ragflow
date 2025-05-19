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


class TemplateParam(ComponentParamBase):
    """
    Define the Generate component parameters.
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
        self.check_empty(self.content, "[Template] Content")
        return True


class Template(ComponentBase):
    component_name = "Template"

    def get_input_elements(self):
        res = {}
        for r in re.finditer(r"\{([a-z]+@[a-z0-9_-]+)\}", self._param.content, flags=re.IGNORECASE):
            cpn_id, var_nm = r.group(1).split("@")
            res[r.group(1)] = {
                "name": f"{var_nm}@"+self._canvas.get_component_name(cpn_id),
            }
        return res

    def _run(self):
        content = self._param.content
        kwargs = {}
        for k in self._param.inputs.keys():
            cpn_id, var_nm = k.split("@")
            if self._param.inputs[k]["value"] is None:
                self._param.inputs[k]["value"] = self._canvas.get_component(cpn_id)["obj"].output(var_nm)
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

