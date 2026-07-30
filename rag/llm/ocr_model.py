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
import io
import json
import logging
import os
import time
import zipfile
from pathlib import Path
from typing import Any, Optional

from deepdoc.parser.mineru_parser import MinerUContentType, MinerUParser
from deepdoc.parser.mistral_parser import MistralParser
from deepdoc.parser.opendataloader_parser import OpenDataLoaderParser
from deepdoc.parser.paddleocr_parser import PaddleOCRParser
from deepdoc.parser.pdf_parser import MAXIMUM_PAGE_NUMBER
from deepdoc.parser.somark_parser import SoMarkParser

import requests


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


class MinerUNetOcrModel(Base):
    _FACTORY_NAME = "MinerU.Net"

    def __init__(self, key: str | dict, model_name: str, **kwargs):
        Base.__init__(self, key, model_name, **kwargs)

        self.api_key = key or os.environ.get("MINERUNET_API_KEY", "")
        self.base_url = (kwargs.get("base_url") or "https://mineru.net").rstrip("/")

        logging.info("Initialized MinerU.Net OCR model, base_url=%s", self.base_url)

    def check_available(self) -> tuple[bool, str]:
        if not self.api_key:
            return False, "MinerU.Net API key is not configured."

        try:
            r = requests.get(
                f"{self.base_url}/api/v4/extract/task/connection-check",
                headers={"Authorization": f"Bearer {self.api_key}"},
                timeout=10,
            )
            if r.status_code in (401, 403):
                return False, f"authentication failed (HTTP {r.status_code}): {r.text[:200]}"
            # 404 (task not found) confirms the key is valid and the server is reachable.
            logging.info("[MinerU.Net] connection check passed, status=%d", r.status_code)
            return True, ""
        except Exception as exc:
            reason = f"MinerU.Net connection failed: {exc}"
            logging.warning(reason)
            return False, reason

    def parse_pdf(self, filepath: str, binary=None, callback=None, parse_method: str = "raw", **kwargs) -> tuple[Any, Any]:
        ok, reason = self.check_available()
        if not ok:
            raise RuntimeError(f"MinerU.Net not accessible: {reason}")

        headers = {"Authorization": f"Bearer {self.api_key}"}

        # Step 1: Submit file to MinerU.Net API
        if callback:
            callback(0.05, "[MinerU.Net] Submitting task...")

        task_id = self._submit_task(filepath, binary, headers, **kwargs)

        if callback:
            callback(0.1, f"[MinerU.Net] Task {task_id} submitted, waiting for processing...")

        # Step 2: Poll until done
        zip_url = self._poll_until_done(task_id, headers, callback)

        if callback:
            callback(0.75, "[MinerU.Net] Downloading result...")

        # Step 3: Download zip and extract content_list.json
        json_data = self._download_and_extract(zip_url, headers)

        if callback:
            callback(0.85, "[MinerU.Net] Parsing result...")

        # Step 4: Parse into sections/tables
        sections = []
        for item in json_data:
            item_type = item.get("type", "")
            if item_type == MinerUContentType.TEXT:
                section = item.get("text", "")
            elif item_type == MinerUContentType.TABLE:
                parts = [item.get("table_body", "")]
                parts.extend(item.get("table_caption", []))
                parts.extend(item.get("table_footnote", []))
                section = "\n".join(p for p in parts if p)
                if not section.strip():
                    section = "FAILED TO PARSE TABLE"
            elif item_type == MinerUContentType.IMAGE:
                section = "".join(item.get("image_caption", [])) + "\n" + "".join(item.get("image_footnote", []))
            elif item_type == MinerUContentType.EQUATION:
                section = item.get("text", "")
            elif item_type == MinerUContentType.CODE:
                section = item.get("code_body", "") + "\n".join(item.get("code_caption", []))
            elif item_type == MinerUContentType.LIST:
                section = "\n".join(item.get("list_items", []))
            elif item_type in (MinerUContentType.HEADER, MinerUContentType.FOOTER, MinerUContentType.PAGE_NUMBER, MinerUContentType.DISCARDED):
                continue
            else:
                logging.debug("[MinerU.Net] Skip unsupported type=%s", item_type)
                continue

            if not section:
                continue

            if parse_method in ("manual", "pipeline"):
                sections.append((section, item_type, ""))
            else:
                sections.append((section, ""))

        if callback:
            callback(1.0, "[MinerU.Net] Done.")

        return sections, []

    def _submit_task(self, filepath: str, binary, headers: dict, **kwargs) -> str:
        if binary is not None:
            if isinstance(binary, str) and (binary.startswith("http://") or binary.startswith("https://")):
                return self._submit_by_url(binary, kwargs.get("lang", ""), headers)
            return self._submit_by_upload(filepath, binary, headers, **kwargs)

        if isinstance(filepath, str) and (filepath.startswith("http://") or filepath.startswith("https://")):
            return self._submit_by_url(filepath, kwargs.get("lang", ""), headers)

        content = Path(filepath).read_bytes()
        return self._submit_by_upload(filepath, content, headers, **kwargs)

    def _submit_by_url(self, url: str, lang: str, headers: dict) -> str:
        req_body: dict = {"url": url, "model_version": self.model_name}
        if lang:
            req_body["lang"] = lang

        r = requests.post(
            f"{self.base_url}/api/v4/extract/task",
            json=req_body,
            headers=headers,
            timeout=30,
        )
        self._check_api_response(r, "submit task")
        return r.json()["data"]["task_id"]

    def _submit_by_upload(self, filepath: str, binary, headers: dict, **kwargs) -> str:
        if isinstance(binary, bytes):
            file_obj = io.BytesIO(binary)
        elif hasattr(binary, "read"):
            file_obj = io.BytesIO(binary.read())
        else:
            raise TypeError(f"_submit_by_upload expects bytes or a file-like object, got {type(binary).__name__}")

        filename = Path(filepath).name or "document.pdf"
        files = {"file": (filename, file_obj, "application/octet-stream")}
        data: dict = {"model_version": self.model_name}
        if kwargs.get("lang"):
            data["lang"] = kwargs["lang"]

        r = requests.post(
            f"{self.base_url}/api/v4/extract/task",
            data=data,
            files=files,
            headers=headers,
            timeout=30,
        )
        self._check_api_response(r, "upload file")
        return r.json()["data"]["task_id"]

    def _poll_until_done(self, task_id: str, headers: dict, callback=None) -> str:
        max_wait = 600
        interval = 2
        elapsed = 0.0

        while elapsed < max_wait:
            time.sleep(interval)
            elapsed += interval

            r = requests.get(
                f"{self.base_url}/api/v4/extract/task/{task_id}",
                headers=headers,
                timeout=15,
            )
            self._check_api_response(r, "poll task")

            data = r.json()["data"]
            state = data.get("state", "")

            if callback:
                progress = data.get("extract_progress", {})
                extracted = progress.get("extracted_pages", 0)
                total = progress.get("total_pages", 0)
                page_info = f" ({extracted}/{total} pages)" if total else ""
                callback(min(0.1 + 0.65 * (elapsed / max_wait), 0.74), f"[MinerU.Net] {state}{page_info}...")

            if state == "done":
                zip_url = data.get("full_zip_url", "")
                if not zip_url:
                    raise RuntimeError("MinerU.Net returned done state but no full_zip_url")
                return zip_url
            if state == "failed":
                raise RuntimeError(f"MinerU.Net task failed: {data.get('err_msg', 'unknown error')}")

            interval = min(interval * 1.5, 15)

        raise TimeoutError(f"MinerU.Net task {task_id} timed out after {max_wait}s")

    def _download_and_extract(self, zip_url: str, headers: dict) -> list[dict]:
        r = requests.get(zip_url, headers=headers, timeout=60)
        if r.status_code != 200:
            raise RuntimeError(f"Failed to download result zip (HTTP {r.status_code}): {r.text[:500]}")

        with zipfile.ZipFile(io.BytesIO(r.content)) as zf:
            for name in zf.namelist():
                if name.endswith("_content_list.json") or name.endswith("content_list.json"):
                    return json.loads(zf.read(name))

        names = "\n".join(zf.namelist())
        raise FileNotFoundError(f"No content_list.json found in result zip. Contents:\n{names}")

    @staticmethod
    def _check_api_response(r, context: str):
        if r.status_code not in (200, 201):
            raise RuntimeError(f"MinerU.Net {context} failed (HTTP {r.status_code}): {r.text[:500]}")
        resp = r.json()
        if resp.get("code") != 0:
            raise RuntimeError(f"MinerU.Net {context} failed (code {resp.get('code')}): {resp.get('msg', '')}")


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

        base_url = _resolve(
            "somark_base_url",
            "SOMARK_BASE_URL",
            kwargs.get("base_url", "https://somark.cn/api/v1"),
        )
        api_key = _resolve("api_key", "SOMARK_API_KEY", key_as_secret)
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

        self.base_url = base_url
        self.api_key = api_key
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


class MistralOcrModel(Base, MistralParser):
    _FACTORY_NAME = "Mistral OCR"

    def __init__(self, key: str | dict, model_name: str, **kwargs):
        Base.__init__(self, key, model_name, **kwargs)
        raw_config: dict = {}
        if isinstance(key, dict):
            raw_config = key
        elif key:
            try:
                raw_config = json.loads(key)
            except Exception:
                raw_config = {}

        # Only unwrap a nested {"api_key": {...}} config object; a flat config
        # whose "api_key" is a string must be preserved so the key is not lost.
        nested_config = raw_config.get("api_key") if isinstance(raw_config, dict) else None
        config = nested_config if isinstance(nested_config, dict) else raw_config
        if not isinstance(config, dict):
            config = {}

        key_as_secret = key if isinstance(key, str) and key and not key.lstrip().startswith("{") else ""

        def _resolve(ui_key: str, env_key: str, default=""):
            return config.get(ui_key, config.get(env_key, kwargs.get(ui_key, kwargs.get(env_key, os.environ.get(env_key, default)))))

        base_url = _resolve("mistral_ocr_base_url", "MISTRAL_OCR_BASE_URL", kwargs.get("base_url") or "https://api.mistral.ai/v1")
        api_key = _resolve("api_key", "MISTRAL_OCR_API_KEY", key_as_secret)
        table_format = _resolve("mistral_ocr_table_format", "MISTRAL_OCR_TABLE_FORMAT", "html")
        keep_hf = _resolve("mistral_ocr_keep_header_footer", "MISTRAL_OCR_KEEP_HEADER_FOOTER", 0)

        # Redact sensitive config keys before logging
        redacted_config = {}
        for k, v in config.items():
            if any(s in k.lower() for s in ("key", "password", "token", "secret")):
                redacted_config[k] = "[REDACTED]"
            else:
                redacted_config[k] = v
        logging.info(f"Parsed Mistral OCR config (sensitive fields redacted): {redacted_config}")

        MistralParser.__init__(
            self,
            base_url=base_url,
            api_key=api_key,
            model=model_name,
            table_format=table_format,
            keep_header_footer=str(keep_hf).strip().lower() in {"1", "true", "yes", "on"},
        )

    def check_available(self) -> tuple[bool, str]:
        return self.check_installation()

    def parse_pdf(self, filepath, binary=None, callback=None, parse_method: str = "raw", from_page: int = 0, to_page: int = MAXIMUM_PAGE_NUMBER, **kwargs):
        ok, reason = self.check_available()
        if not ok:
            raise RuntimeError(f"Mistral OCR not accessible: {reason}")
        return MistralParser.parse_pdf(
            self,
            filepath=filepath,
            binary=binary,
            callback=callback,
            parse_method=parse_method,
            from_page=from_page,
            to_page=to_page,
            **kwargs,
        )
