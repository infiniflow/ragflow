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
import logging
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

    def _run(self, history, **kwargs):
        args = {}
        for para in self._param.variables:
            logging.info(f"Setting argument {para}")
            if para.get("component_id") and para.get("key"):
                if '@' in para["component_id"]:
                    component = para["component_id"].split('@')[0]
                    field = para["component_id"].split('@')[1]
                    cpn = self._canvas.get_component(component)["obj"]
                    for param in cpn._param.query:
                        if param["key"] == field:
                            if "value" in param:
                                args[para["key"]] = param["value"]
                else:
                    component = self._canvas.get_component(para["component_id"])
                    if component is not None:
                        cpn = component["obj"]
                        if cpn.component_name.lower() == "answer":
                            args[para["key"]] = self._canvas.get_history(1)[0]["content"]
                            continue
                        _, out = cpn.output(allow_partial=False)
                        if not out.empty:
                            args[para["key"]] = "\n".join(out["content"])
            elif para.get("component_id") == ("allparambegin"): 
                    begin_obj = self._canvas.get_component("begin")
                    if begin_obj is not None:
                        cpn = begin_obj["obj"]
                        for ele in cpn._param.query:
                            logging.info(f"Setting argument {ele}")
                            args[ele["key"]] = ele["value"]
            elif para.get("key"):
                args[para["key"]] = para["value"]
        logging.info(f"Invoke: args: {args}")
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
                return Invoke.be_output("\n".join(sections))
            return Invoke.be_output(response.text)

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
                sections = HtmlParser()(None, response.content)
                return Invoke.be_output("\n".join(sections))
            return Invoke.be_output(response.text)
