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
import os
import random
import re
import logging
import tempfile
from functools import partial
from typing import Any

from agent.component.base import ComponentBase, ComponentParamBase
from jinja2 import Template as Jinja2Template

from common.connection_utils import timeout
from common.misc_utils import get_uuid
from common import settings


class MessageParam(ComponentParamBase):
    """
    Define the Message component parameters.
    """
    def __init__(self):
        super().__init__()
        self.content = []
        self.stream = True
        self.output_format = None  # default output format
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

    def get_input_elements(self) -> dict[str, Any]:
        return self.get_input_elements_from_text("".join(self._param.content))

    def get_kwargs(self, script:str, kwargs:dict = {}, delimiter:str=None) -> tuple[str, dict[str, str | list | Any]]:
        for k,v in self.get_input_elements_from_text(script).items():
            if k in kwargs:
                continue
            v = v["value"]
            if not v:
                v = ""
            ans = ""
            if isinstance(v, partial):
                for t in v():
                    ans += t
            elif isinstance(v, list) and delimiter:
                ans = delimiter.join([str(vv) for vv in v])
            elif not isinstance(v, str):
                try:
                    ans = json.dumps(v, ensure_ascii=False)
                except Exception:
                    pass
            else:
                ans = v
            if not ans:
                ans = ""
            kwargs[k] = ans
            self.set_input_value(k, ans)

        _kwargs = {}
        for n, v in kwargs.items():
            _n = re.sub("[@:.]", "_", n)
            script = re.sub(r"\{%s\}" % re.escape(n), _n, script)
            _kwargs[_n] = v
        return script, _kwargs

    def _stream(self, rand_cnt:str):
        s = 0
        all_content = ""
        cache = {}
        for r in re.finditer(self.variable_ref_patt, rand_cnt, flags=re.DOTALL):
            if self.check_if_canceled("Message streaming"):
                return

            all_content += rand_cnt[s: r.start()]
            yield rand_cnt[s: r.start()]
            s = r.end()
            exp = r.group(1)
            if exp in cache:
                yield cache[exp]
                all_content += cache[exp]
                continue

            v = self._canvas.get_variable_value(exp)
            if v is None:
                v = ""
            if isinstance(v, partial):
                cnt = ""
                for t in v():
                    if self.check_if_canceled("Message streaming"):
                        return

                    all_content += t
                    cnt += t
                    yield t
                self.set_input_value(exp, cnt)
                continue
            elif not isinstance(v, str):
                try:
                    v = json.dumps(v, ensure_ascii=False)
                except Exception:
                    v = str(v)
            yield v
            self.set_input_value(exp, v)
            all_content += v
            cache[exp] = v

        if s < len(rand_cnt):
            if self.check_if_canceled("Message streaming"):
                return

            all_content += rand_cnt[s: ]
            yield rand_cnt[s: ]

        self.set_output("content", all_content)
        self._convert_content(all_content)

    def _is_jinjia2(self, content:str) -> bool:
        patt = [
            r"\{%.*%\}", "{{", "}}"
        ]
        return any([re.search(p, content) for p in patt])

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 10*60)))
    def _invoke(self, **kwargs):
        if self.check_if_canceled("Message processing"):
            return

        rand_cnt = random.choice(self._param.content)
        if self._param.stream and not self._is_jinjia2(rand_cnt):
            self.set_output("content", partial(self._stream, rand_cnt))
            return

        rand_cnt, kwargs = self.get_kwargs(rand_cnt, kwargs)
        template = Jinja2Template(rand_cnt)
        try:
            content = template.render(kwargs)
        except Exception:
            pass

        if self.check_if_canceled("Message processing"):
            return

        for n, v in kwargs.items():
            content = re.sub(n, v, content)

        self.set_output("content", content)
        self._convert_content(content)

    def thoughts(self) -> str:
        return ""

    def _convert_content(self, content):
        if not self._param.output_format:
            return

        import pypandoc
        doc_id = get_uuid()
        
        if self._param.output_format.lower() not in {"markdown", "html", "pdf", "docx"}:
            self._param.output_format = "markdown"

        try:
            if self._param.output_format in {"markdown", "html"}:
                if isinstance(content, str):
                    converted = pypandoc.convert_text(
                        content,
                        to=self._param.output_format,
                        format="markdown",
                    )
                else:
                    converted = pypandoc.convert_file(
                        content,
                        to=self._param.output_format,
                        format="markdown",
                    )

                binary_content = converted.encode("utf-8")

            else:  # pdf, docx
                with tempfile.NamedTemporaryFile(suffix=f".{self._param.output_format}", delete=False) as tmp:
                    tmp_name = tmp.name

                try:
                    if isinstance(content, str):
                        pypandoc.convert_text(
                            content,
                            to=self._param.output_format,
                            format="markdown",
                            outputfile=tmp_name,
                        )
                    else:
                        pypandoc.convert_file(
                            content,
                            to=self._param.output_format,
                            format="markdown",
                            outputfile=tmp_name,
                        )

                    with open(tmp_name, "rb") as f:
                        binary_content = f.read()

                finally:
                    if os.path.exists(tmp_name):
                        os.remove(tmp_name)

            settings.STORAGE_IMPL.put(self._canvas._tenant_id, doc_id, binary_content)
            self.set_output("attachment", {
                "doc_id":doc_id, 
                "format":self._param.output_format, 
                "file_name":f"{doc_id[:8]}.{self._param.output_format}"})

            logging.info(f"Converted content uploaded as {doc_id} (format={self._param.output_format})")

        except Exception as e:
            logging.error(f"Error converting content to {self._param.output_format}: {e}")