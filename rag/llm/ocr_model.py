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


class Base:
    def __init__(self, key: str | dict, model_name: str, **kwargs):
        self.model_name = model_name

    def parse_pdf(self, filepath: str, binary=None, **kwargs) -> tuple[Any, Any]:
        raise NotImplementedError("Please implement parse_pdf!")


from deepdoc.parser.deepseek_ocr2_parser import DeepSeekOcr2Parser, DeepSeekOcr2Backend


def _safe_bool(value, default=True):
    """Safely convert config value to bool."""
    if isinstance(value, bool):
        return value
    if isinstance(value, (int, float)):
        return bool(value)
    if isinstance(value, str):
        v = value.strip().lower()
        if v in ("1", "true", "yes", "on"):
            return True
        if v in ("0", "false", "no", "off"):
            return False
        if v == "":
            return default
    return default


class DeepSeekOcr2Model(Base):
    """DeepSeek-OCR2 model using Visual Causal Flow for document understanding."""

    _FACTORY_NAME = "DeepSeek-OCR2"

    def __init__(self, key: str | dict, model_name: str, **kwargs):
        Base.__init__(self, key, model_name, **kwargs)

        # Parse configuration
        raw_config = {}
        if key:
            try:
                raw_config = json.loads(key) if isinstance(key, str) else key
            except Exception:
                raw_config = {}

        config = raw_config.get("api_key", raw_config)
        if not isinstance(config, dict):
            config = {}

        def _resolve_config(k: str, env_key: str, default=""):
            return config.get(k, config.get(env_key, os.environ.get(env_key, default)))

        # Configuration
        self.backend = _resolve_config("backend", "DEEPSEEK_OCR2_BACKEND", "local")
        self.api_url = _resolve_config("api_url", "DEEPSEEK_OCR2_API_URL", "")
        self.api_key_value = _resolve_config("api_key", "DEEPSEEK_OCR2_API_KEY", "")
        self.model_path = _resolve_config("model_path", "DEEPSEEK_OCR2_MODEL_PATH", "")
        self.device = _resolve_config("device", "DEEPSEEK_OCR2_DEVICE", "cuda")
        self.use_flash_attn = _safe_bool(_resolve_config("use_flash_attn", "DEEPSEEK_OCR2_USE_FLASH_ATTN", "1"), default=True)

        # Initialize parser
        self.parser = DeepSeekOcr2Parser(
            model_path=self.model_path or None,
            device=self.device,
            use_flash_attn=self.use_flash_attn,
            backend=self.backend,
            api_url=self.api_url,
            api_key=self.api_key_value,
        )

        logging.info(f"[DeepSeek-OCR2] Initialized with backend={self.backend}")

    def check_available(self) -> tuple[bool, str]:
        return self.parser.check_available()

    def parse_pdf(self, filepath: str, binary=None, callback=None, **kwargs):
        ok, reason = self.check_available()
        if not ok:
            raise RuntimeError(f"DeepSeek-OCR2 not available: {reason}")
        return self.parser.parse_pdf(filepath=filepath, binary=binary, callback=callback, **kwargs)


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
            if any(
                sensitive_word in k.lower()
                for sensitive_word in ("key", "password", "token", "secret")
            ):
                redacted_config[k] = "[REDACTED]"
            else:
                redacted_config[k] = v
        logging.info(
            f"Parsed MinerU config (sensitive fields redacted): {redacted_config}"
        )

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
            **kwargs
        )
        return sections, tables
