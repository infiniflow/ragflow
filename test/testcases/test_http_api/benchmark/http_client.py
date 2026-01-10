import json
from typing import Any, Dict, Optional, Tuple

import requests


class HttpClient:
    def __init__(
        self,
        base_url: str,
        api_version: str = "v1",
        api_key: Optional[str] = None,
        login_token: Optional[str] = None,
        connect_timeout: float = 5.0,
        read_timeout: float = 60.0,
        verify_ssl: bool = True,
    ) -> None:
        self.base_url = base_url.rstrip("/")
        self.api_version = api_version
        self.api_key = api_key
        self.login_token = login_token
        self.connect_timeout = connect_timeout
        self.read_timeout = read_timeout
        self.verify_ssl = verify_ssl

    def api_base(self) -> str:
        return f"{self.base_url}/api/{self.api_version}"

    def non_api_base(self) -> str:
        return f"{self.base_url}/{self.api_version}"

    def build_url(self, path: str, use_api_base: bool = True) -> str:
        base = self.api_base() if use_api_base else self.non_api_base()
        return f"{base}/{path.lstrip('/')}"

    def _headers(self, auth_kind: Optional[str], extra: Optional[Dict[str, str]]) -> Dict[str, str]:
        headers = {}
        if auth_kind == "api" and self.api_key:
            headers["Authorization"] = f"Bearer {self.api_key}"
        elif auth_kind == "login" and self.login_token:
            headers["Authorization"] = self.login_token
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
    ) -> requests.Response:
        url = self.build_url(path, use_api_base=use_api_base)
        merged_headers = self._headers(auth_kind, headers)
        timeout: Tuple[float, float] = (self.connect_timeout, self.read_timeout)
        return requests.request(
            method=method,
            url=url,
            headers=merged_headers,
            json=json_body,
            data=data,
            files=files,
            params=params,
            timeout=timeout,
            stream=stream,
            verify=self.verify_ssl,
        )

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
