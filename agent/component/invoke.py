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
from functools import partial

import requests

from agent.component.base import ComponentBase, ComponentParamBase
from common.connection_utils import timeout
from deepdoc.parser import HtmlParser


class InvokeParam(ComponentParamBase):
    """
    Define the Invoke component parameters.
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
        self.datatype = "json"

    def check(self):
        self.check_valid_value(self.method.lower(), "Type of content from the crawler", ["get", "post", "put"])
        self.check_empty(self.url, "End point URL")
        self.check_positive_integer(self.timeout, "Timeout time in second")
        self.check_boolean(self.clean_html, "Clean HTML")
        self.check_valid_value(self.datatype.lower(), "Data post type", ["json", "formdata"])  # Check for valid datapost value


class Invoke(ComponentBase, ABC):
    component_name = "Invoke"
    header_variable_ref_patt = r"\{([a-zA-Z_][a-zA-Z0-9_.@-]*)\}"

    @staticmethod
    def _coerce_json_arg_if_possible(key, value):
        raw_value = value
        if isinstance(value, str):
            try:
                value = json.loads(value)
                logging.debug(
                    "Invoke JSON arg coercion succeeded. key=%s parsed_type=%s",
                    key,
                    type(value).__name__,
                )
            except json.JSONDecodeError as exc:
                logging.info(
                    "Invoke JSON arg coercion skipped; value is not valid JSON. key=%s raw=%r error=%s",
                    key,
                    raw_value,
                    exc,
                )
                return raw_value

        try:
            json.dumps(value, allow_nan=False)
        except (TypeError, ValueError) as exc:
            logging.warning(
                "Invoke JSON arg is not JSON-serializable. key=%s value_type=%s value=%r error=%s",
                key,
                type(value).__name__,
                value,
                exc,
            )
            raise ValueError(f"Invoke JSON argument '{key}' is not JSON-serializable.") from exc

        return value

    def get_input_form(self) -> dict[str, dict]:
        res = {}
        for item in self._param.variables or []:
            if not isinstance(item, dict):
                continue
            ref = (item.get("ref") or "").strip()
            if not ref or ref in res:
                continue

            elements = self.get_input_elements_from_text("{" + ref + "}")
            element = elements.get(ref, {})
            res[ref] = {
                "type": "line",
                "name": element.get("name") or item.get("key") or ref,
            }
        return res

    def _resolve_variable_value(self, variable_name: str, kwargs: dict | None = None):
        kwargs = kwargs or {}
        value = kwargs.get(variable_name, self._canvas.get_variable_value(variable_name))
        if isinstance(value, partial):
            value = "".join(value())
            self.set_input_value(variable_name, value)
        return "" if value is None else value

    def _render_template(self, content: str, pattern: str, kwargs: dict | None = None, *, flags: int = 0) -> str:
        content = content or ""
        if not content:
            return content

        def replace_variable(match_obj):
            return str(self._resolve_variable_value(match_obj.group(1), kwargs))

        return re.sub(pattern, replace_variable, content, flags=flags)

    def _resolve_template_text(self, content: str, kwargs: dict | None = None) -> str:
        return self._render_template(content, self.variable_ref_patt, kwargs, flags=re.DOTALL)

    def _resolve_header_text(self, content: str, kwargs: dict | None = None) -> str:
        # Headers support plain {token} placeholders, so they cannot reuse the canvas variable regex.
        return self._render_template(content, self.header_variable_ref_patt, kwargs)

    def _resolve_arg_value(self, para: dict, kwargs: dict) -> object:
        ref = (para.get("ref") or "").strip()
        if ref and (ref in kwargs or self._canvas.get_variable_value(ref) is not None):
            return self._resolve_variable_value(ref, kwargs)

        if para.get("value") is not None:
            value = para["value"]
            if isinstance(value, str):
                return self._resolve_template_text(value, kwargs)
            return value

        if ref:
            return self._resolve_variable_value(ref, kwargs)

        return ""

    def _is_json_mode(self) -> bool:
        return self._param.datatype.lower() == "json"

    def _build_request_args(self, kwargs: dict) -> dict:
        args = {}
        for para in self._param.variables:
            key = para["key"]
            value = self._resolve_arg_value(para, kwargs)
            if self._is_json_mode():
                # JSON mode accepts stringified JSON so complex payloads can be passed through variables.
                value = self._coerce_json_arg_if_possible(key, value)
            args[key] = value

            if para.get("ref"):
                self.set_input_value(para["ref"], value)
        return args

    def _build_url(self, kwargs: dict) -> str:
        url = self._resolve_template_text(self._param.url.strip(), kwargs)
        if not url.startswith(("http://", "https://")):
            url = "http://" + url
        return url

    def _build_headers(self, kwargs: dict) -> dict:
        if not self._param.headers:
            return {}

        headers = json.loads(self._param.headers)
        if not isinstance(headers, dict):
            raise ValueError("Invoke headers must be a JSON object.")

        return {
            key: self._resolve_header_text(value, kwargs) if isinstance(value, str) else value
            for key, value in headers.items()
        }

    def _build_proxies(self) -> dict | None:
        if not re.sub(r"https?:?/?/?", "", self._param.proxy):
            return None
        return {"http": self._param.proxy, "https": self._param.proxy}

    def _send_request(self, url: str, args: dict, headers: dict, proxies: dict | None):
        method = self._param.method.lower()
        request = getattr(requests, method)
        request_kwargs = {
            "url": url,
            "headers": headers,
            "proxies": proxies,
            "timeout": self._param.timeout,
        }

        # GET sends query params; POST/PUT send either JSON or form data based on datatype.
        if method == "get":
            request_kwargs["params"] = args
            return request(**request_kwargs)

        body_key = "json" if self._is_json_mode() else "data"
        request_kwargs[body_key] = args
        return request(**request_kwargs)

    def _format_response(self, response) -> str:
        if not self._param.clean_html:
            return response.text

        # HtmlParser keeps the Invoke output text-focused when the endpoint returns HTML.
        sections = HtmlParser()(None, response.content)
        return "\n".join(sections)
    
    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 3)))
    def _invoke(self, **kwargs):
        if self.check_if_canceled("Invoke processing"):
            return

        args = self._build_request_args(kwargs)
        url = self._build_url(kwargs)
        headers = self._build_headers(kwargs)
        proxies = self._build_proxies()

        last_error = None
        for _ in range(self._param.max_retries + 1):
            if self.check_if_canceled("Invoke processing"):
                return

            try:
                response = self._send_request(url, args, headers, proxies)
                result = self._format_response(response)
                self.set_output("result", result)
                return result
            except Exception as e:
                if self.check_if_canceled("Invoke processing"):
                    return

                last_error = e
                logging.exception(f"Http request error: {e}")
                time.sleep(self._param.delay_after_error)

        if last_error:
            self.set_output("_ERROR", str(last_error))
            return f"Http request error: {last_error}"

    def thoughts(self) -> str:
        return "Waiting for the server respond..."
