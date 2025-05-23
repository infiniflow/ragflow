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
from agent.component.base import ComponentBase, ComponentParamBase
from jinja2 import Template as Jinja2Template


class MessageParam(ComponentParamBase):
    """
    Define the Message component parameters.
    """
    def __init__(self):
        super().__init__()
        self.content = ""
        self.stream = True
        self.outputs = {
            "content": {
                "type": "str"
            }
        }

    def check(self):
        self.check_empty(self.content, "[Message] Content")
        self.check_boolean(self.stream, "[Message] stream")
        return True


class Message(ComponentBase):
    component_name = "Message"

    def get_kwargs(self) -> dict[str, str]:
        res = {}
        for k,v in self.get_input_elements_from_text(self._param.content).items():
            v = v["value"]
            ans = ""
            if isinstance(v, partial):
                for t in v():
                    ans += t
            else:
                if not isinstance(v, str):
                    try:
                        v = json.dumps(v, ensure_ascii=False)
                    except Exception:
                        pass
                ans = v
            res[k] = ans
        return res

    def _stream(self):
        s = 0
        all_content = ""
        for r in re.finditer(r"\{([a-z:0-9]+@[a-z0-9_.-]+|sys\.[a-z_]+)\}", self._param.content, flags=re.DOTALL):
            all_content += self._param.content[s: r.start()]
            yield self._param.content[s: r.start()]
            s = r.end()
            exp = r.group(1)
            v = self._canvas.get_variable_value(exp)
            if isinstance(v, partial):
                for t in v():
                    all_content += t
                    yield t
            else:
                if not isinstance(v, str):
                    try:
                        v = json.dumps(v, ensure_ascii=False)
                    except Exception:
                        pass
                all_content += v
                yield v

        if s < len(self._param.content):
            all_content += self._param.content[s: ]
            yield self._param.content[s: ]

        self.set_output("content", all_content)

    async def _invoke(self):
        if self._param.stream:
            self.set_output("content", partial(self._stream))
            return

        template = Jinja2Template(self._param.content)
        kwargs = self.get_kwargs()

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

        self.set_output("content", content)

