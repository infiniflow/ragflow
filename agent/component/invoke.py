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

from agent.component.base import ComponentBase, ComponentParamBase
from common.connection_utils import timeout
from deepdoc.parser import HtmlParser


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
        self.request_body = ""  # Request body as JSON string

    def check(self):
        self.check_valid_value(self.method.lower(), "Type of content from the crawler", ["get", "post", "put"])
        self.check_empty(self.url, "End point URL")
        self.check_positive_integer(self.timeout, "Timeout time in second")
        self.check_boolean(self.clean_html, "Clean HTML")
        self.check_valid_value(self.datatype.lower(), "Data post type", ["json", "formdata"])  # Check for valid datapost value

    def get_input_form(self) -> dict[str, dict]:
        return getattr(self, "inputs", {})


class Invoke(ComponentBase, ABC):
    component_name = "Invoke"

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 3)))
    def _invoke(self, **kwargs):
        if self.check_if_canceled("Invoke processing"):
            return

        args = {}
        for para in self._param.variables:
            if para.get("value"):
                args[para["key"]] = para["value"]
            else:
                args[para["key"]] = self._canvas.get_variable_value(para["ref"])

        def replace_variable(match):
            var_name = match.group(1)
            try:
                value = self._canvas.get_variable_value(var_name)
                return str(value or "")
            except Exception:
                return ""

        url = (kwargs.get("url") or self._param.url).strip()

        # {base_url} or {component_id@variable_name}
        url = re.sub(r"\{([a-zA-Z_][a-zA-Z0-9_.@-]*)\}", replace_variable, url)

        if url.find("http") != 0:
            url = "http://" + url

        method = (kwargs.get("method") or self._param.method).lower()
        headers = {}
        headers_input = kwargs.get("headers") or self._param.headers
        if headers_input:
            if isinstance(headers_input, dict):
                headers = headers_input
            elif isinstance(headers_input, str):
                headers = json.loads(headers_input)
        proxies = None
        if re.sub(r"https?:?/?/?", "", self._param.proxy):
            proxies = {"http": self._param.proxy, "https": self._param.proxy}

        # Process request body if provided (from kwargs when used as tool, or from param)
        request_body_data = None
        request_body_input = kwargs.get("request_body") or self._param.request_body
        
        if request_body_input:
            # If it's already a dict (from agent tool call), use it directly
            if isinstance(request_body_input, dict):
                request_body_data = request_body_input
            else:
                # Otherwise, treat it as a JSON string
                request_body_str = str(request_body_input).strip()
                if request_body_str:
                    # Replace variables in request body
                    request_body_str = re.sub(r"\{([a-zA-Z_][a-zA-Z0-9_.@-]*)\}", replace_variable, request_body_str)
                    try:
                        request_body_data = json.loads(request_body_str)
                    except json.JSONDecodeError as e:
                        logging.error(f"Invalid JSON in request body: {e}")
                        self.set_output("_ERROR", f"Invalid JSON in request body: {e}")
                        return f"Invalid JSON in request body: {e}"

        last_e = ""
        for _ in range(self._param.max_retries + 1):
            if self.check_if_canceled("Invoke processing"):
                return

            try:
                if method == "get":
                    response = requests.get(url=url, params=args, headers=headers, proxies=proxies, timeout=self._param.timeout)
                    if self._param.clean_html:
                        sections = HtmlParser()(None, response.content)
                        self.set_output("result", "\n".join(sections))
                    else:
                        self.set_output("result", response.text)

                if method == "put":
                    # Use request_body if provided, otherwise fall back to args
                    body_data = request_body_data if request_body_data is not None else args
                    if self._param.datatype.lower() == "json":
                        if request_body_data is not None:
                            response = requests.put(url=url, json=body_data, headers=headers, proxies=proxies, timeout=self._param.timeout)
                        else:
                            response = requests.put(url=url, json=body_data, headers=headers, proxies=proxies, timeout=self._param.timeout)
                    else:
                        if request_body_data is not None:
                            # For formdata with request_body, convert to string if it's a dict
                            if isinstance(body_data, dict):
                                response = requests.put(url=url, data=body_data, headers=headers, proxies=proxies, timeout=self._param.timeout)
                            else:
                                response = requests.put(url=url, data=str(body_data), headers=headers, proxies=proxies, timeout=self._param.timeout)
                        else:
                            response = requests.put(url=url, data=body_data, headers=headers, proxies=proxies, timeout=self._param.timeout)
                    if self._param.clean_html:
                        sections = HtmlParser()(None, response.content)
                        self.set_output("result", "\n".join(sections))
                    else:
                        self.set_output("result", response.text)

                if method == "post":
                    # Use request_body if provided, otherwise fall back to args
                    body_data = request_body_data if request_body_data is not None else args
                    if self._param.datatype.lower() == "json":
                        if request_body_data is not None:
                            response = requests.post(url=url, json=body_data, headers=headers, proxies=proxies, timeout=self._param.timeout)
                        else:
                            response = requests.post(url=url, json=body_data, headers=headers, proxies=proxies, timeout=self._param.timeout)
                    else:
                        if request_body_data is not None:
                            # For formdata with request_body, convert to string if it's a dict
                            if isinstance(body_data, dict):
                                response = requests.post(url=url, data=body_data, headers=headers, proxies=proxies, timeout=self._param.timeout)
                            else:
                                response = requests.post(url=url, data=str(body_data), headers=headers, proxies=proxies, timeout=self._param.timeout)
                        else:
                            response = requests.post(url=url, data=body_data, headers=headers, proxies=proxies, timeout=self._param.timeout)
                    if self._param.clean_html:
                        sections = HtmlParser()(None, response.content)
                        self.set_output("result", "\n".join(sections))
                    else:
                        self.set_output("result", response.text)

                return self.output("result")
            except Exception as e:
                if self.check_if_canceled("Invoke processing"):
                    return

                last_e = e
                logging.exception(f"Http request error: {e}")
                time.sleep(self._param.delay_after_error)

        if last_e:
            self.set_output("_ERROR", str(last_e))
            return f"Http request error: {last_e}"

        assert False, self.output()

    def thoughts(self) -> str:
        return "Waiting for the server respond..."
