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
        self.parameters = []

    def check(self):
        self.check_empty(self.content, "[Template] Content")
        return True


class Template(ComponentBase):
    component_name = "Template"

    def get_dependent_components(self):
        inputs = self.get_input_elements()
        cpnts = set([i["key"] for i in inputs if i["key"].lower().find("answer") < 0 and i["key"].lower().find("begin") < 0])
        return list(cpnts)

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

        for k in self._param.inputs.keys():
            cpn_id, var_nm = k.split("@") 

        for para in self.get_input_elements():
            if para["key"].lower().find("begin@") == 0:
                cpn_id, key = para["key"].split("@")
                for p in self._canvas.get_component(cpn_id)["obj"]._param.query:
                    if p["key"] == key:
                        value = p.get("value", "")
                        self.make_kwargs(para, kwargs, value)
                        break
                else:
                    assert False, f"Can't find parameter '{key}' for {cpn_id}"
                continue

            component_id = para["key"]
            cpn = self._canvas.get_component(component_id)["obj"]
            if cpn.component_name.lower() == "answer":
                hist = self._canvas.get_history(1)
                if hist:
                    hist = hist[0]["content"]
                else:
                    hist = ""
                self.make_kwargs(para, kwargs, hist)
                continue

            _, out = cpn.output(allow_partial=False)

            result = ""
            if "content" in out.columns:
                result = "\n".join(
                    [o if isinstance(o, str) else str(o) for o in out["content"]]
                )

            self.make_kwargs(para, kwargs, result)

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

        return Template.be_output(content)

    def make_kwargs(self, para, kwargs, value):
        self._param.inputs.append(
            {"component_id": para["key"], "content": value}
        )
        try:
            value = json.loads(value)
        except Exception:
            pass
        kwargs[para["key"]] = value
