#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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

import time
import json
import typing
from typing import Any, Dict, Optional

import requests
# from requests.sessions import HTTPAdapter


class HttpClient:
    def __init__(
            self,
            host: str = "127.0.0.1",
            port: int = 9381,
            api_version: str = "v1",
            api_key: Optional[str] = None,
            connect_timeout: float = 5.0,
            read_timeout: float = 60.0,
            verify_ssl: bool = False,
    ) -> None:
        self.host = host
        self.port = port
        self.api_version = api_version
        self.api_key = api_key
        self.login_token: str | None = None
        self.connect_timeout = connect_timeout
        self.read_timeout = read_timeout
        self.verify_ssl = verify_ssl

    def api_base(self) -> str:
        return f"{self.host}:{self.port}/api/{self.api_version}"

    def non_api_base(self) -> str:
        return f"{self.host}:{self.port}/{self.api_version}"

    def build_url(self, path: str, use_api_base: bool = True) -> str:
        base = self.api_base() if use_api_base else self.non_api_base()
        if self.verify_ssl:
            return f"https://{base}/{path.lstrip('/')}"
        else:
            return f"http://{base}/{path.lstrip('/')}"

    def _headers(self, auth_kind: Optional[str], extra: Optional[Dict[str, str]]) -> Dict[str, str]:
        headers = {}
        if auth_kind == "api" and self.api_key:
            headers["Authorization"] = f"Bearer {self.api_key}"
        elif auth_kind == "web" and self.login_token:
            headers["Authorization"] = self.login_token
        elif auth_kind == "admin" and self.login_token:
            headers["Authorization"] = self.login_token
        else:
            pass
        if extra:
            headers.update(extra)
        return headers

    def request(
            self,
            method: str,
            path: str,
            *,
            use_api_base: bool = True,
            auth_kind: Optional[str] = "api",
            headers: Optional[Dict[str, str]] = None,
            json_body: Optional[Dict[str, Any]] = None,
            data: Any = None,
            files: Any = None,
            params: Optional[Dict[str, Any]] = None,
            stream: bool = False,
            iterations: int = 1,
    ) -> requests.Response | dict:
        url = self.build_url(path, use_api_base=use_api_base)
        merged_headers = self._headers(auth_kind, headers)
        # timeout: Tuple[float, float] = (self.connect_timeout, self.read_timeout)
        session = requests.Session()
        # adapter = HTTPAdapter(pool_connections=100, pool_maxsize=100)
        # session.mount("http://", adapter)
        http_function = typing.Any
        match method:
            case "GET":
                http_function = session.get
            case "POST":
                http_function = session.post
            case "PUT":
                http_function = session.put
            case "DELETE":
                http_function = session.delete
            case "PATCH":
                http_function = session.patch
            case _:
                raise ValueError(f"Invalid HTTP method: {method}")

        if iterations > 1:
            response_list = []
            total_duration = 0.0
            for _ in range(iterations):
                start_time = time.perf_counter()
                response = http_function(url, headers=merged_headers, json=json_body, data=data, stream=stream)
                # response = session.get(url, headers=merged_headers, json=json_body, data=data, stream=stream)
                # response = requests.request(
                #     method=method,
                #     url=url,
                #     headers=merged_headers,
                #     json=json_body,
                #     data=data,
                #     files=files,
                #     params=params,
                #     stream=stream,
                #     verify=self.verify_ssl,
                # )
                end_time = time.perf_counter()
                total_duration += end_time - start_time
                response_list.append(response)
            return {"duration": total_duration, "response_list": response_list}
        else:
            return http_function(url, headers=merged_headers, json=json_body, data=data, stream=stream)
            # return session.get(url, headers=merged_headers, json=json_body, data=data, stream=stream)
            # return requests.request(
            #     method=method,
            #     url=url,
            #     headers=merged_headers,
            #     json=json_body,
            #     data=data,
            #     files=files,
            #     params=params,
            #     stream=stream,
            #     verify=self.verify_ssl,
            # )

    def request_json(
            self,
            method: str,
            path: str,
            *,
            use_api_base: bool = True,
            auth_kind: Optional[str] = "api",
            headers: Optional[Dict[str, str]] = None,
            json_body: Optional[Dict[str, Any]] = None,
            data: Any = None,
            files: Any = None,
            params: Optional[Dict[str, Any]] = None,
            stream: bool = False,
    ) -> Dict[str, Any]:
        response = self.request(
            method,
            path,
            use_api_base=use_api_base,
            auth_kind=auth_kind,
            headers=headers,
            json_body=json_body,
            data=data,
            files=files,
            params=params,
            stream=stream,
        )
        try:
            return response.json()
        except Exception as exc:
            raise ValueError(f"Non-JSON response from {path}: {exc}") from exc

    @staticmethod
    def parse_json_bytes(raw: bytes) -> Dict[str, Any]:
        try:
            return json.loads(raw.decode("utf-8"))
        except Exception as exc:
            raise ValueError(f"Invalid JSON payload: {exc}") from exc
