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

from jinja2 import StrictUndefined
from jinja2.sandbox import SandboxedEnvironment

from agent.component.base import ComponentBase, ComponentParamBase


class TemplateParam(ComponentParamBase):
    """
    Define the Template component parameters.
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
        key_set = set([])
        res = []
        for r in re.finditer(r"\{([a-z]+[:@][a-z0-9_-]+)\}", self._param.content, flags=re.IGNORECASE):
            cpn_id = r.group(1)
            if cpn_id in key_set:
                continue
            if cpn_id.lower().find("begin@") == 0:
                parts = cpn_id.split("@", 1)
                if len(parts) != 2:
                    continue
                cpn_id, key = parts
                cpn = self._canvas.get_component(cpn_id)
                if not cpn or "obj" not in cpn or not hasattr(cpn["obj"]._param, "query"):
                    continue
                for p in cpn["obj"]._param.query:
                    if p.get("key") != key:
                        continue
                    res.append({"key": r.group(1), "name": p.get("name", "")})
                    key_set.add(r.group(1))
                continue
            cpn_nm = self._canvas.get_component_name(cpn_id)
            if not cpn_nm:
                continue
            res.append({"key": cpn_id, "name": cpn_nm})
            key_set.add(cpn_id)
        return res

    def _run(self, history, **kwargs):
        content = self._param.content

        self._param.inputs = []
        for para in self.get_input_elements():
            if para["key"].lower().find("begin@") == 0:
                parts = para["key"].split("@", 1)
                if len(parts) != 2:
                    continue
                cpn_id, key = parts
                cpn = self._canvas.get_component(cpn_id)
                if not cpn or "obj" not in cpn or not hasattr(cpn["obj"]._param, "query"):
                    continue
                found = False
                for p in cpn["obj"]._param.query:
                    if p.get("key") == key:
                        value = p.get("value", "")
                        self.make_kwargs(para, kwargs, value)

                        origin_pattern = "{begin@" + key + "}"
                        new_pattern = "begin_" + key
                        content = content.replace(origin_pattern, "{" + new_pattern + "}")
                        if origin_pattern in kwargs:
                            kwargs[new_pattern] = kwargs.pop(origin_pattern)
                        else:
                            kwargs[new_pattern] = value
                        found = True
                        break
                if not found:
                    raise AssertionError(f"Can't find parameter '{key}' for {cpn_id}")
                continue

            component_id = para["key"]
            cpn = self._canvas.get_component(component_id)
            if not cpn or "obj" not in cpn:
                continue
            cpn_obj = cpn["obj"]
            if getattr(cpn_obj, "component_name", "").lower() == "answer":
                hist = self._canvas.get_history(1)
                if hist:
                    hist = hist[0].get("content", "")
                else:
                    hist = ""
                self.make_kwargs(para, kwargs, hist)

                if ":" in component_id:
                    origin_pattern = "{" + component_id + "}"
                    new_pattern = component_id.replace(":", "_")
                    content = content.replace(origin_pattern, "{" + new_pattern + "}")
                    if component_id in kwargs:
                        kwargs[new_pattern] = kwargs.pop(component_id)
                    else:
                        kwargs[new_pattern] = hist
                continue

            output_result = cpn_obj.output(allow_partial=False)
            if isinstance(output_result, tuple) and len(output_result) == 2:
                _, out = output_result
            else:
                out = output_result

            result = ""
            if hasattr(out, "columns") and "content" in getattr(out, "columns", []):
                result = "\n".join([o if isinstance(o, str) else str(o) for o in out["content"]])

            self.make_kwargs(para, kwargs, result)

        env = SandboxedEnvironment(
            autoescape=True,
            undefined=StrictUndefined,
        )
        template = env.from_string(content)

        try:
            content = template.render(kwargs)
        except Exception:
            pass

        for n, v in kwargs.items():
            if not isinstance(v, str):
                try:
                    v = json.dumps(v, ensure_ascii=False)
                except Exception:
                    v = str(v)
            # Process backslashes in strings, Use Lambda function to avoid escape issues
            if isinstance(v, str):
                v = v.replace("\\", "\\\\")
            content = re.sub(r"\{%s\}" % re.escape(n), lambda match: v, content)
            content = re.sub(r"(#+)", r" \1 ", content)

        return Template.be_output(content)

    def make_kwargs(self, para, kwargs, value):
        self._param.inputs.append({"component_id": para["key"], "content": value})
        try:
            value = json.loads(value)
        except Exception:
            pass
        kwargs[para["key"]] = value
