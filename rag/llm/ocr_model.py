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
import logging
import os
from io import BytesIO
from os import PathLike
from typing import Callable, Optional

from deepdoc.parser.mineru_parser import MinerUParser


class Base:
    def __init__(self, key: str | dict, model_name: str, **kwargs):
        self.model_name = model_name

    def parse_pdf(
        self,
        filepath: str | PathLike[str],
        binary: BytesIO | bytes,
        callback: Optional[Callable] = None,
        *,
        output_dir: Optional[str] = None,
        backend: str = "pipeline",
        lang: Optional[str] = None,
        method: str = "auto",
        server_url: Optional[str] = None,
        delete_output: bool = True,
        parse_method: str = "raw",
        formula_enable: bool = True,
        table_enable: bool = True,
    ) -> tuple:
        raise NotImplementedError("Please implement parse_pdf!")


class MinerUOcrModel(Base, MinerUParser):
    _FACTORY_NAME = "MinerU"

    def __init__(self, key: str | dict, model_name: str, **kwargs):
        Base.__init__(self, key, model_name, **kwargs)
        
        # Use environment variables directly - no database config needed
        self.mineru_api = os.environ.get("MINERU_APISERVER", "")
        self.mineru_output_dir = os.environ.get("MINERU_OUTPUT_DIR", "")
        self.mineru_backend = os.environ.get("MINERU_BACKEND", "pipeline")
        self.mineru_server_url = os.environ.get("MINERU_SERVER_URL", "")
        self.mineru_delete_output = os.environ.get("MINERU_DELETE_OUTPUT", "1") == "1"
        self.mineru_executable = os.environ.get("MINERU_EXECUTABLE", "mineru")

        logging.info(f"MinerU config from env: api={self.mineru_api}, backend={self.mineru_backend}, server_url={self.mineru_server_url}")

        MinerUParser.__init__(self, mineru_path=self.mineru_executable, mineru_api=self.mineru_api, mineru_server_url=self.mineru_server_url)

    def check_available(self, backend: Optional[str] = None, server_url: Optional[str] = None) -> tuple[bool, str]:
        backend = backend or self.mineru_backend
        server_url = server_url or self.mineru_server_url
        return self.check_installation(backend=backend, server_url=server_url)

    def parse_pdf(
        self,
        filepath: str | PathLike[str],
        binary: BytesIO | bytes,
        callback: Optional[Callable] = None,
        *,
        output_dir: Optional[str] = None,
        backend: str = "pipeline",
        lang: Optional[str] = None,
        method: str = "auto",
        server_url: Optional[str] = None,
        delete_output: bool = True,
        parse_method: str = "raw",
        formula_enable: bool = True,
        table_enable: bool = True,
    ) -> tuple:
        ok, reason = self.check_available()
        if not ok:
            raise RuntimeError(f"MinerU not found or server not accessible: {reason}. Please install it via: pip install -U 'mineru[core]'.")

        sections, tables = MinerUParser.parse_pdf(
            self,
            filepath=filepath,
            binary=binary,  # type: ignore[arg-type]
            callback=callback,
            output_dir=output_dir or self.mineru_output_dir,
            backend=backend or self.mineru_backend,
            lang=lang,
            method=method,
            server_url=server_url or self.mineru_server_url,
            delete_output=delete_output if delete_output is not None else self.mineru_delete_output,
            parse_method=parse_method,
            formula_enable=formula_enable,
            table_enable=table_enable,
        )
        return sections, tables
