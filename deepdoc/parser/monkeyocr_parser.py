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
from __future__ import annotations

import logging
from typing import Callable, Optional

from deepdoc.parser.mineru_parser import MinerUParser


class MonkeyOCRParser(MinerUParser):
    """MonkeyOCRv2 PDF parser using a dedicated MinerU-compatible HTTP adapter.

    Zip/content-list consumption is delegated to :class:`MinerUParser` so this
    backend stays aligned with MinerU output handling while keeping its own
    configuration surface (``MONKEYOCR_*`` env keys and provider registration).
    """

    def __init__(self, monkeyocr_api: str = "", monkeyocr_server_url: str = ""):
        super().__init__(
            mineru_api=(monkeyocr_api or "").rstrip("/"),
            mineru_server_url=(monkeyocr_server_url or "").rstrip("/"),
        )
        self.logger = logging.getLogger(self.__class__.__name__)

    def check_installation(self, backend: str = "vlm-transformers", server_url: Optional[str] = None) -> tuple[bool, str]:
        # Availability is the HTTP adapter; ``backend`` is kept for MinerUParser API compatibility.
        _ = backend
        if not self.mineru_api:
            reason = "[MonkeyOCR] MONKEYOCR_APISERVER not configured."
            self.logger.warning(reason)
            return False, reason

        api_openapi = f"{self.mineru_api}/openapi.json"
        try:
            api_ok = self._is_http_endpoint_valid(api_openapi)
            self.logger.info("[MonkeyOCR] API openapi.json reachable=%s url=%s", api_ok, api_openapi)
            if not api_ok:
                return False, f"[MonkeyOCR] API not accessible: {api_openapi}"
        except Exception as exc:
            reason = f"[MonkeyOCR] API check failed: {exc}"
            self.logger.warning(reason)
            return False, reason

        resolved_server = server_url or self.mineru_server_url
        if resolved_server:
            try:
                server_ok = self._is_http_endpoint_valid(resolved_server)
                self.logger.info("[MonkeyOCR] optional vLLM server reachable=%s url=%s", server_ok, resolved_server)
            except Exception as exc:
                self.logger.warning("[MonkeyOCR] optional vLLM server probe failed: %s: %s", resolved_server, exc)

        return True, ""

    def parse_pdf(
        self,
        filepath,
        binary,
        callback: Optional[Callable] = None,
        *,
        output_dir: Optional[str] = None,
        backend: str = "vlm-transformers",
        server_url: Optional[str] = None,
        delete_output: bool = True,
        parse_method: str = "raw",
        page_from: int = 0,
        page_to=None,
        **kwargs,
    ) -> tuple:
        if callback:
            callback(0.1, "[MonkeyOCR] Parsing PDF via MonkeyOCR adapter...")
        return super().parse_pdf(
            filepath,
            binary,
            callback=callback,
            output_dir=output_dir,
            backend=backend,
            server_url=server_url or self.mineru_server_url,
            delete_output=delete_output,
            parse_method=parse_method,
            page_from=page_from,
            page_to=page_to,
            **kwargs,
        )
