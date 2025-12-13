#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
from typing import Any, Optional, Tuple

from deepdoc.parser.mineru_parser import MinerUParser


class Base:
    def __init__(self, key: str | dict, model_name: str, **kwargs):
        self.model_name = model_name

    def parse_pdf(self, filepath: str, binary=None, **kwargs) -> Tuple[Any, Any]:
        raise NotImplementedError("Please implement parse_pdf!")


class MinerUOcrModel(Base, MinerUParser):
    _FACTORY_NAME = "MinerU"

    def __init__(self, key: str | dict, model_name: str, **kwargs):
        Base.__init__(self, key, model_name, **kwargs)
        config = {}
        if key:
            try:
                config = json.loads(key)
            except Exception:
                config = {}
        config = config["api_key"]
        self.mineru_api = config.get("mineru_apiserver", os.environ.get("MINERU_APISERVER", ""))
        self.mineru_output_dir = config.get("mineru_output_dir", os.environ.get("MINERU_OUTPUT_DIR", ""))
        self.mineru_backend = config.get("mineru_backend", os.environ.get("MINERU_BACKEND", "pipeline"))
        self.mineru_server_url = config.get("mineru_server_url", os.environ.get("MINERU_SERVER_URL", ""))
        self.mineru_delete_output = bool(int(config.get("mineru_delete_output", os.environ.get("MINERU_DELETE_OUTPUT", 1))))
        self.mineru_executable = os.environ.get("MINERU_EXECUTABLE", "mineru")

        logging.info(f"Parsed MinerU config: {config}")

        MinerUParser.__init__(self, mineru_path=self.mineru_executable, mineru_api=self.mineru_api, mineru_server_url=self.mineru_server_url)

    def check_available(self, backend: Optional[str] = None, server_url: Optional[str] = None) -> Tuple[bool, str]:
        backend = backend or self.mineru_backend
        server_url = server_url or self.mineru_server_url
        return self.check_installation(backend=backend, server_url=server_url)

    def parse_pdf(self, filepath: str, binary=None, callback=None, parse_method: str = "raw",**kwargs):
        ok, reason = self.check_available()
        if not ok:
            raise RuntimeError(f"MinerU not found or server not accessible: {reason}. Please install it via: pip install -U 'mineru[core]'.")

        sections, tables = MinerUParser.parse_pdf(
            self,
            filepath=filepath,
            binary=binary,
            callback=callback,
            output_dir=self.mineru_output_dir,
            backend=self.mineru_backend,
            server_url=self.mineru_server_url,
            delete_output=self.mineru_delete_output,
            parse_method=parse_method,
            **kwargs
        )
        return sections, tables
