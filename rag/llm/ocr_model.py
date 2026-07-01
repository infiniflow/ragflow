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
from typing import Any, Optional

from deepdoc.parser.mineru_parser import MinerUParser
from deepdoc.parser.opendataloader_parser import OpenDataLoaderParser
from deepdoc.parser.paddleocr_parser import PaddleOCRParser
from deepdoc.parser.somark_parser import SoMarkParser


class Base:
    def __init__(self, key: str | dict, model_name: str, **kwargs):
        self.model_name = model_name

    def parse_pdf(self, filepath: str, binary=None, **kwargs) -> tuple[Any, Any]:
        raise NotImplementedError("Please implement parse_pdf!")


class MinerUOcrModel(Base, MinerUParser):
    _FACTORY_NAME = "MinerU"

    def __init__(self, key: str | dict, model_name: str, **kwargs):
        Base.__init__(self, key, model_name, **kwargs)
        raw_config = {}
        if key:
            try:
                raw_config = json.loads(key)
            except Exception:
                raw_config = {}

        # nested {"api_key": {...}} from UI
        # flat {"MINERU_*": "..."} payload auto-provisioned from env vars
        config = raw_config.get("api_key", raw_config)
        if not isinstance(config, dict):
            config = {}

        def _resolve_config(key: str, env_key: str, default=""):
            # lower-case keys (UI), upper-case MINERU_* (env auto-provision), env vars
            return config.get(key, config.get(env_key, os.environ.get(env_key, default)))

        self.mineru_api = _resolve_config("mineru_apiserver", "MINERU_APISERVER", "")
        self.mineru_output_dir = _resolve_config("mineru_output_dir", "MINERU_OUTPUT_DIR", "")
        self.mineru_backend = _resolve_config("mineru_backend", "MINERU_BACKEND", "pipeline")
        self.mineru_server_url = _resolve_config("mineru_server_url", "MINERU_SERVER_URL", "")
        self.mineru_delete_output = bool(int(_resolve_config("mineru_delete_output", "MINERU_DELETE_OUTPUT", 1)))

        # Redact sensitive config keys before logging
        redacted_config = {}
        for k, v in config.items():
            if any(sensitive_word in k.lower() for sensitive_word in ("key", "password", "token", "secret")):
                redacted_config[k] = "[REDACTED]"
            else:
                redacted_config[k] = v
        logging.info(f"Parsed MinerU config (sensitive fields redacted): {redacted_config}")

        MinerUParser.__init__(self, mineru_api=self.mineru_api, mineru_server_url=self.mineru_server_url)

    def check_available(self, backend: Optional[str] = None, server_url: Optional[str] = None) -> tuple[bool, str]:
        backend = backend or self.mineru_backend
        server_url = server_url or self.mineru_server_url
        return self.check_installation(backend=backend, server_url=server_url)

    def parse_pdf(self, filepath: str, binary=None, callback=None, parse_method: str = "raw", **kwargs):
        ok, reason = self.check_available()
        if not ok:
            raise RuntimeError(f"MinerU server not accessible: {reason}")

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
            **kwargs,
        )
        return sections, tables


class PaddleOCROcrModel(Base, PaddleOCRParser):
    _FACTORY_NAME = "PaddleOCR"

    def __init__(self, key: str | dict, model_name: str, **kwargs):
        Base.__init__(self, key, model_name, **kwargs)
        raw_config = {}
        if key:
            try:
                raw_config = json.loads(key)
            except Exception:
                raw_config = {}

        # nested {"api_key": {...}} from UI
        # flat {"PADDLEOCR_*": "..."} payload auto-provisioned from env vars
        config = raw_config.get("api_key", raw_config)
        if not isinstance(config, dict):
            config = {}

        def _resolve_config(key: str, env_key: str, default=""):
            # lower-case keys (UI), upper-case PADDLEOCR_* (env auto-provision), env vars
            return config.get(key, config.get(env_key, os.environ.get(env_key, default)))

        self.paddleocr_base_url = _resolve_config("paddleocr_base_url", "PADDLEOCR_BASE_URL", "") or _resolve_config("paddleocr_api_url", "PADDLEOCR_API_URL", "")
        self.paddleocr_algorithm = _resolve_config("paddleocr_algorithm", "PADDLEOCR_ALGORITHM", "PaddleOCR-VL")
        self.paddleocr_access_token = _resolve_config("paddleocr_access_token", "PADDLEOCR_ACCESS_TOKEN", None)

        # Redact sensitive config keys before logging
        redacted_config = {}
        for k, v in config.items():
            if any(sensitive_word in k.lower() for sensitive_word in ("key", "password", "token", "secret")):
                redacted_config[k] = "[REDACTED]"
            else:
                redacted_config[k] = v
        logging.info(f"Parsed PaddleOCR config (sensitive fields redacted): {redacted_config}")

        PaddleOCRParser.__init__(
            self,
            base_url=self.paddleocr_base_url or None,
            access_token=self.paddleocr_access_token,
            algorithm=self.paddleocr_algorithm,
        )

    def check_available(self) -> tuple[bool, str]:
        return self.check_installation()

    def parse_pdf(self, filepath: str, binary=None, callback=None, parse_method: str = "raw", **kwargs):
        ok, reason = self.check_available()
        if not ok:
            raise RuntimeError(f"PaddleOCR server not accessible: {reason}")

        sections, tables = PaddleOCRParser.parse_pdf(self, filepath=filepath, binary=binary, callback=callback, parse_method=parse_method, **kwargs)
        return sections, tables

    def parse_image(self, filepath: str, binary=None, callback=None, **kwargs) -> str:
        ok, reason = self.check_available()
        if not ok:
            raise RuntimeError(f"PaddleOCR server not accessible: {reason}")

        logging.info(f"PaddleOCR parse_image start: {filepath}")
        result = PaddleOCRParser.parse_image(self, filepath=filepath, binary=binary, callback=callback, **kwargs)
        logging.info(f"PaddleOCR parse_image done: {filepath}, text length: {len(result)}")
        return result


class OpenDataLoaderOcrModel(Base, OpenDataLoaderParser):
    _FACTORY_NAME = "OpenDataLoader"

    def __init__(self, key: str | dict, model_name: str, **kwargs):
        Base.__init__(self, key, model_name, **kwargs)
        raw_config = {}
        if key:
            try:
                raw_config = json.loads(key)
            except Exception:
                raw_config = {}

        config = raw_config.get("api_key", raw_config)
        if not isinstance(config, dict):
            config = {}

        def _resolve_config(key: str, env_key: str, default=""):
            return config.get(key, config.get(env_key, os.environ.get(env_key, default)))

        redacted_config = {}
        for k, v in config.items():
            if any(s in k.lower() for s in ("key", "password", "token", "secret")):
                redacted_config[k] = "[REDACTED]"
            else:
                redacted_config[k] = v
        logging.info(f"Parsed OpenDataLoader config (sensitive fields redacted): {redacted_config}")

        OpenDataLoaderParser.__init__(self)
        self.api_url = _resolve_config("opendataloader_apiserver", "OPENDATALOADER_APISERVER", "").rstrip("/")
        self.api_key = _resolve_config("opendataloader_api_key", "OPENDATALOADER_API_KEY", "").strip()
        timeout_val = _resolve_config("opendataloader_timeout", "OPENDATALOADER_TIMEOUT", "600") or "600"
        try:
            self.timeout = int(timeout_val)
        except (TypeError, ValueError):
            self.timeout = 600

    def check_available(self) -> tuple[bool, str]:
        ok = self.check_installation()
        return ok, "" if ok else "OpenDataLoader service not reachable"

    def parse_pdf(self, filepath: str, binary=None, callback=None, parse_method: str = "raw", **kwargs):
        ok, reason = self.check_available()
        if not ok:
            raise RuntimeError(f"OpenDataLoader service not accessible: {reason}")

        sections, tables = OpenDataLoaderParser.parse_pdf(
            self,
            filepath=filepath,
            binary=binary,
            callback=callback,
            parse_method=parse_method,
            **kwargs,
        )
        return sections, tables


class SoMarkOcrModel(Base, SoMarkParser):
    _FACTORY_NAME = "SoMark"

    def __init__(self, key: str | dict, model_name: str, **kwargs):
        Base.__init__(self, key, model_name, **kwargs)
        raw_config: dict = {}
        if isinstance(key, dict):
            # API verify path passes the form dict directly; no JSON to parse.
            raw_config = key
        elif key:
            try:
                raw_config = json.loads(key)
            except Exception:
                raw_config = {}

        # nested {"api_key": {...}} from UI
        # flat {"SOMARK_*": "..."} payload auto-provisioned from env vars
        config = raw_config.get("api_key", raw_config)
        if not isinstance(config, dict):
            config = {}

        key_as_secret = key if isinstance(key, str) and key and not key.lstrip().startswith("{") else ""

        def _resolve(ui_key: str, env_key: str, default=""):
            return config.get(
                ui_key,
                config.get(
                    env_key,
                    kwargs.get(
                        ui_key,
                        kwargs.get(env_key, os.environ.get(env_key, default)),
                    ),
                ),
            )

        def _resolve_bool(ui_key: str, env_key: str, default: bool) -> bool:
            raw = _resolve(ui_key, env_key, int(default))
            if isinstance(raw, bool):
                return raw
            if isinstance(raw, (int, float)):
                return bool(raw)
            return str(raw).strip().lower() in {"1", "true", "yes", "on"}

        def _resolve_int(ui_key: str, env_key: str, default: int) -> int:
            raw = _resolve(ui_key, env_key, default)
            try:
                return int(raw)
            except (TypeError, ValueError):
                return default

        base_url = _resolve(
            "somark_base_url",
            "SOMARK_BASE_URL",
            kwargs.get("base_url", "https://somark.tech/api/v1"),
        )
        api_key = _resolve("somark_api_key", "SOMARK_API_KEY", key_as_secret)
        image_format = _resolve("somark_image_format", "SOMARK_IMAGE_FORMAT", "url")
        formula_format = _resolve("somark_formula_format", "SOMARK_FORMULA_FORMAT", "latex")
        table_format = _resolve("somark_table_format", "SOMARK_TABLE_FORMAT", "html")
        cs_format = _resolve("somark_cs_format", "SOMARK_CS_FORMAT", "image")
        enable_text_cross_page = _resolve_bool("somark_enable_text_cross_page", "SOMARK_ENABLE_TEXT_CROSS_PAGE", False)
        enable_table_cross_page = _resolve_bool("somark_enable_table_cross_page", "SOMARK_ENABLE_TABLE_CROSS_PAGE", False)
        enable_title_level_recognition = _resolve_bool("somark_enable_title_level_recognition", "SOMARK_ENABLE_TITLE_LEVEL_RECOGNITION", False)
        enable_inline_image = _resolve_bool("somark_enable_inline_image", "SOMARK_ENABLE_INLINE_IMAGE", True)
        enable_table_image = _resolve_bool("somark_enable_table_image", "SOMARK_ENABLE_TABLE_IMAGE", True)
        enable_image_understanding = _resolve_bool("somark_enable_image_understanding", "SOMARK_ENABLE_IMAGE_UNDERSTANDING", True)
        keep_header_footer = _resolve_bool("somark_keep_header_footer", "SOMARK_KEEP_HEADER_FOOTER", False)

        # Redact sensitive config keys before logging
        redacted_config = {}
        for k, v in config.items():
            if any(s in k.lower() for s in ("key", "password", "token", "secret")):
                redacted_config[k] = "[REDACTED]"
            else:
                redacted_config[k] = v
        logging.info(f"Parsed SoMark config (sensitive fields redacted): {redacted_config}")

        SoMarkParser.__init__(
            self,
            base_url=base_url,
            api_key=api_key,
            image_format=image_format,
            formula_format=formula_format,
            table_format=table_format,
            cs_format=cs_format,
            enable_text_cross_page=enable_text_cross_page,
            enable_table_cross_page=enable_table_cross_page,
            enable_title_level_recognition=enable_title_level_recognition,
            enable_inline_image=enable_inline_image,
            enable_table_image=enable_table_image,
            enable_image_understanding=enable_image_understanding,
            keep_header_footer=keep_header_footer,
        )

    def check_available(self) -> tuple[bool, str]:
        return self.check_installation()

    def parse_pdf(self, filepath: str, binary=None, callback=None, parse_method: str = "raw", **kwargs):
        ok, reason = self.check_available()
        if not ok:
            raise RuntimeError(f"SoMark service not accessible: {reason}")

        # parse_method selects the output tuple shape (see SoMarkParser._transfer_to_sections):
        # manual/pipeline -> typed 3-tuples for the rag/flow DAG; raw/other -> 2-tuples
        # for naive.py chunking. Thread it through like MinerU rather than dropping it.
        sections, tables = SoMarkParser.parse_pdf(
            self,
            filepath=filepath,
            binary=binary,
            callback=callback,
            parse_method=parse_method,
            **kwargs,
        )
        return sections, tables
