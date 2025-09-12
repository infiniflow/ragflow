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
import logging
import os
import re
import time
from abc import ABC
import requests

from api.utils.api_utils import timeout
from deepdoc.parser import HtmlParser
from agent.component.base import ComponentBase, ComponentParamBase


class InvokeParam(ComponentParamBase):
    """
    Define the Crawler component parameters.
    """

    def __init__(self):
        super().__init__()
        self.proxy = None
        self.headers = ""
        self.method = "get"
        self.variables = []
        self.url = ""
        self.timeout = 60
        self.clean_html = False
        self.datatype = "json"  # New parameter to determine data posting type

    def check(self):
        self.check_valid_value(self.method.lower(), "Type of content from the crawler", ['get', 'post', 'put'])
        self.check_empty(self.url, "End point URL")
        self.check_positive_integer(self.timeout, "Timeout time in second")
        self.check_boolean(self.clean_html, "Clean HTML")
        self.check_valid_value(self.datatype.lower(), "Data post type", ['json', 'formdata'])  # Check for valid datapost value


class Invoke(ComponentBase, ABC):
    component_name = "Invoke"

    @timeout(os.environ.get("COMPONENT_EXEC_TIMEOUT", 3))
    def _invoke(self, **kwargs):
        args = {}
        for para in self._param.variables:
            if para.get("value"):
                args[para["key"]] = para["value"]
            else:
                args[para["key"]] = self._canvas.get_variable_value(para["ref"])

        url = self._param.url.strip()
        if url.find("http") != 0:
            url = "http://" + url

        method = self._param.method.lower()
        headers = {}
        if self._param.headers:
            headers = json.loads(self._param.headers)
        proxies = None
        if re.sub(r"https?:?/?/?", "", self._param.proxy):
            proxies = {"http": self._param.proxy, "https": self._param.proxy}

        last_e = ""
        for _ in range(self._param.max_retries+1):
            try:
                if method == 'get':
                    response = requests.get(url=url,
                                            params=args,
                                            headers=headers,
                                            proxies=proxies,
                                            timeout=self._param.timeout)
                    if self._param.clean_html:
                        sections = HtmlParser()(None, response.content)
                        self.set_output("result", "\n".join(sections))
                    else:
                        self.set_output("result", response.text)

                if method == 'put':
                    if self._param.datatype.lower() == 'json':
                        response = requests.put(url=url,
                                                json=args,
                                                headers=headers,
                                                proxies=proxies,
                                                timeout=self._param.timeout)
                    else:
                        response = requests.put(url=url,
                                                data=args,
                                                headers=headers,
                                                proxies=proxies,
                                                timeout=self._param.timeout)
                    if self._param.clean_html:
                        sections = HtmlParser()(None, response.content)
                        self.set_output("result", "\n".join(sections))
                    else:
                        self.set_output("result", response.text)

                if method == 'post':
                    if self._param.datatype.lower() == 'json':
                        response = requests.post(url=url,
                                                 json=args,
                                                 headers=headers,
                                                 proxies=proxies,
                                                 timeout=self._param.timeout)
                    else:
                        response = requests.post(url=url,
                                                 data=args,
                                                 headers=headers,
                                                 proxies=proxies,
                                                 timeout=self._param.timeout)
                    if self._param.clean_html:
                        self.set_output("result", "\n".join(sections))
                    else:
                        self.set_output("result", response.text)

                return self.output("result")
            except Exception as e:
                last_e = e
                logging.exception(f"Http request error: {e}")
                time.sleep(self._param.delay_after_error)

        if last_e:
            self.set_output("_ERROR", str(last_e))
            return f"Http request error: {last_e}"

        assert False, self.output()

    def thoughts(self) -> str:
        return "Waiting for the server respond..."
