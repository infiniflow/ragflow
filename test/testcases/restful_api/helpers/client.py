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

from dataclasses import dataclass
from typing import Any

import requests
from configs import HOST_ADDRESS, VERSION


@dataclass
class RestClient:
    token: str | None = None
    timeout: int = 30

    @property
    def api_root(self) -> str:
        return f"{HOST_ADDRESS}/api/{VERSION}"

    def _headers(self, headers: dict[str, str] | None = None) -> dict[str, str]:
        merged: dict[str, str] = {"Content-Type": "application/json"}
        if headers:
            merged.update(headers)
        if self.token and "Authorization" not in merged:
            merged["Authorization"] = f"Bearer {self.token}"
        return merged

    def request(
        self,
        method: str,
        path: str,
        *,
        headers: dict[str, str] | None = None,
        params: dict[str, Any] | None = None,
        json: dict[str, Any] | None = None,
        data: Any = None,
        files: Any = None,
        **request_kwargs: Any,
    ) -> requests.Response:
        req_headers = self._headers(headers)
        if files is not None:
            # requests sets multipart boundary automatically.
            req_headers.pop("Content-Type", None)

        timeout = request_kwargs.pop("timeout", self.timeout)
        normalized_path = f"/{path.lstrip('/')}" if path else "/"
        return requests.request(
            method=method,
            url=f"{self.api_root}{normalized_path}",
            headers=req_headers,
            params=params,
            json=json,
            data=data,
            files=files,
            timeout=timeout,
            **request_kwargs,
        )

    def get(self, path: str, **kwargs) -> requests.Response:
        return self.request("GET", path, **kwargs)

    def post(self, path: str, **kwargs) -> requests.Response:
        return self.request("POST", path, **kwargs)

    def delete(self, path: str, **kwargs) -> requests.Response:
        return self.request("DELETE", path, **kwargs)

    def put(self, path: str, **kwargs) -> requests.Response:
        return self.request("PUT", path, **kwargs)

    def patch(self, path: str, **kwargs) -> requests.Response:
        return self.request("PATCH", path, **kwargs)
