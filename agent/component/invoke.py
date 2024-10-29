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
from abc import ABC
import requests
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

    def check(self):
        self.check_valid_value(self.method.lower(), "Type of content from the crawler", ['get', 'post', 'put'])
        self.check_empty(self.url, "End point URL")
        self.check_positive_integer(self.timeout, "Timeout time in second")
        self.check_boolean(self.clean_html, "Clean HTML")


class Invoke(ComponentBase, ABC):
    component_name = "Invoke"

    def _run(self, history, **kwargs):
        args = {}
        for para in self._param.variables:
            if para.get("component_id"):
                cpn = self._canvas.get_component(para["component_id"])["obj"]
                _, out = cpn.output(allow_partial=False)
                args[para["key"]] = "\n".join(out["content"])
            else:
                args[para["key"]] = "\n".join(para["value"])

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

        if method == 'get':
            response = requests.get(url=url,
                                    params=args,
                                    headers=headers,
                                    proxies=proxies,
                                    timeout=self._param.timeout)
            if self._param.clean_html:
                sections = HtmlParser()(None, response.content)
                return Invoke.be_output("\n".join(sections))

            return Invoke.be_output(response.text)

        if method == 'put':
            response = requests.put(url=url,
                                    data=args,
                                    headers=headers,
                                    proxies=proxies,
                                    timeout=self._param.timeout)
            if self._param.clean_html:
                sections = HtmlParser()(None, response.content)
                return Invoke.be_output("\n".join(sections))
            return Invoke.be_output(response.text)

        if method == 'post':
            response = requests.post(url=url,
                                    json=args,
                                    headers=headers,
                                    proxies=proxies,
                                    timeout=self._param.timeout)
            if self._param.clean_html:
                sections = HtmlParser()(None, response.content)
                return Invoke.be_output("\n".join(sections))
            return Invoke.be_output(response.text)
