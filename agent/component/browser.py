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

import asyncio
import hashlib
import inspect
import json
import logging
import os
import re
import shutil
import tempfile
from abc import ABC
from pathlib import Path
from typing import Any
from urllib.parse import unquote, urlparse
from urllib.request import Request, urlopen

from agent.component.base import ComponentBase
from agent.component.llm import LLMParam
from api.db import FileType
from api.db.joint_services.tenant_model_service import get_model_config_by_type_and_name
from api.db.services import duplicate_name
from api.db.services.file_service import FileService
from api.db.services.tenant_llm_service import TenantLLMService
from api.utils.file_utils import filename_type
from common import settings
from common.connection_utils import timeout
from common.misc_utils import get_uuid
from rag.llm import FACTORY_DEFAULT_BASE_URL


class BrowserParam(LLMParam):
    """
    Parameters for Browser node.
    """

    def __init__(self):
        super().__init__()
        self.prompts = "{sys.query}"
        self.max_steps = 30
        self.headless = True
        # Reuse browser profile across runs of the same agent node by default.
        self.persist_session = True
        self.upload_sources = []
        self.outputs = {
            "content": {"type": "string", "value": ""},
            "downloaded_files": {"type": "Array<Object>", "value": []},
        }

    def check(self):
        self.check_empty(self.llm_id, "[Browser] LLM")
        self.check_positive_integer(self.max_steps, "[Browser] Max steps")
        self.check_boolean(self.headless, "[Browser] Headless")
        self.check_boolean(self.persist_session, "[Browser] Persist session")
        self.check_empty(self.prompts, "[Browser] Prompts")
        return True

    def get_input_form(self) -> dict[str, dict]:
        return {
            "prompts": {"type": "text", "name": "Prompts"},
            "upload_sources": {"type": "line", "name": "Upload sources"},
        }


class Browser(ComponentBase, ABC):
    component_name = "Browser"

    def get_input_elements(self) -> dict[str, dict]:
        text_parts = [
            str(self._param.prompts or ""),
            json.dumps(self._param.upload_sources, ensure_ascii=False),
        ]
        return self.get_input_elements_from_text("\n".join(text_parts))

    def _iter_strings(self, value: Any):
        if value is None:
            return
        if isinstance(value, str):
            yield value
            return
        if isinstance(value, dict):
            for item in value.values():
                yield from self._iter_strings(item)
            return
        if isinstance(value, (list, tuple, set)):
            for item in value:
                yield from self._iter_strings(item)

    def _resolve_param_value(self, value: Any) -> Any:
        if isinstance(value, str):
            direct_ref = value.strip()
            if direct_ref.startswith("{") and direct_ref.endswith("}") and self._canvas.is_reff(direct_ref):
                return self._canvas.get_variable_value(direct_ref)
            return value
        return value

    def _extract_ids(self, value: Any) -> list[str]:
        ids: list[str] = []
        value = self._resolve_param_value(value)

        def collect(item: Any):
            if item is None:
                return
            if isinstance(item, str):
                token = item.strip()
                if not token:
                    return
                if token.startswith("{") and token.endswith("}") and self._canvas.is_reff(token):
                    collect(self._canvas.get_variable_value(token))
                    return
                if token.startswith("[") and token.endswith("]"):
                    try:
                        parsed = json.loads(token)
                        collect(parsed)
                        return
                    except Exception:
                        pass
                if self._is_http_url(token):
                    ids.append(token)
                    return
                if "," in token:
                    for part in token.split(","):
                        collect(part)
                    return
                ids.append(token)
                return
            if isinstance(item, dict):
                for k in ("file_id", "id", "url", "value"):
                    if k in item:
                        collect(item[k])
                        return
                for v in item.values():
                    collect(v)
                return
            if isinstance(item, (list, tuple, set)):
                for v in item:
                    collect(v)
                return
            token = str(item).strip()
            if token:
                ids.append(token)

        collect(value)
        deduped: list[str] = []
        visited = set()
        for item in ids:
            if item in visited:
                continue
            visited.add(item)
            deduped.append(item)
        return deduped

    @staticmethod
    def _is_http_url(value: str) -> bool:
        token = str(value or "").strip()
        if not token:
            return False
        parsed = urlparse(token)
        return parsed.scheme in {"http", "https"} and bool(parsed.netloc)

    @staticmethod
    def _extract_url_filename(url: str, headers: Any) -> str:
        content_disposition = str(getattr(headers, "get", lambda *_args, **_kwargs: "")("Content-Disposition", "") or "")
        if content_disposition:
            # Prefer RFC 5987 encoded filename*=UTF-8''... when present.
            m = re.search(r"filename\*\s*=\s*(?:UTF-8''|utf-8'')?([^;]+)", content_disposition)
            if m:
                name = unquote(m.group(1).strip().strip('"'))
                if name:
                    return os.path.basename(name)
            m = re.search(r'filename\s*=\s*"([^"]+)"', content_disposition)
            if m:
                name = m.group(1).strip()
                if name:
                    return os.path.basename(name)
            m = re.search(r"filename\s*=\s*([^;]+)", content_disposition)
            if m:
                name = m.group(1).strip().strip('"')
                if name:
                    return os.path.basename(name)

        parsed = urlparse(url)
        raw_name = os.path.basename(parsed.path or "")
        name = unquote(raw_name).strip()
        if name:
            return name
        return f"url_file_{get_uuid()[:8]}.bin"

    def _prepare_upload_url_file(self, url: str, upload_dir: str) -> dict[str, Any] | None:
        try:
            req = Request(url, headers={"User-Agent": "RAGFlow-Browser-Node/1.0"})
            with urlopen(req, timeout=30) as response:
                blob = response.read()
                if not blob:
                    logging.warning("Browser upload url returned empty content: %s", url)
                    return None
                local_name = self._extract_url_filename(url, response.headers)
        except Exception as e:
            logging.warning("Browser failed to fetch upload url. url=%s, error=%s", url, e)
            return None

        local_path = os.path.join(upload_dir, local_name)
        index = 1
        while os.path.exists(local_path):
            stem, ext = os.path.splitext(local_name)
            local_path = os.path.join(upload_dir, f"{stem}_{index}{ext}")
            index += 1
        with open(local_path, "wb") as f:
            f.write(blob)
        return {
            "file_id": "",
            "name": local_name,
            "size": len(blob),
            "local_path": local_path,
            "source_url": url,
        }

    def _resolve_text(self, raw_text: Any) -> str:
        text = str(self._resolve_param_value(raw_text) or "")
        vars_map = self.get_input_elements_from_text(text)
        kv = {}
        for key, meta in vars_map.items():
            val = meta.get("value", "")
            if isinstance(val, str):
                kv[key] = val
            else:
                kv[key] = json.dumps(val, ensure_ascii=False)
        return self.string_format(text, kv)

    @staticmethod
    def _as_model_config_dict(cfg_obj: Any) -> dict[str, Any]:
        if cfg_obj is None:
            return {}
        if isinstance(cfg_obj, dict):
            return cfg_obj
        if hasattr(cfg_obj, "to_dict") and callable(cfg_obj.to_dict):
            try:
                result = cfg_obj.to_dict()
                return result if isinstance(result, dict) else {}
            except Exception:
                return {}
        result = {}
        for key in ("model", "model_name", "llm_name", "llm_factory", "api_key", "base_url", "api_base", "temperature"):
            val = getattr(cfg_obj, key, None)
            if val not in (None, ""):
                result[key] = val
        return result

    @staticmethod
    def _build_with_signature(cls: Any, kwargs: dict[str, Any]):
        clean_kwargs = {k: v for k, v in kwargs.items() if v not in (None, "")}
        try:
            sig = inspect.signature(cls)
            accepted = {k: v for k, v in clean_kwargs.items() if k in sig.parameters}
            if accepted:
                return cls(**accepted)
            # Some constructors expose a generic `**kwargs` signature; in that case,
            # parameter-name filtering above would drop all fields and lose config.
            if clean_kwargs:
                return cls(**clean_kwargs)
            return cls()
        except Exception:
            return cls(**clean_kwargs) if clean_kwargs else cls()

    @staticmethod
    def _env_truthy(name: str, default: bool = False) -> bool:
        val = os.getenv(name)
        if val is None:
            return default
        return val.strip().lower() in {"1", "true", "yes", "on"}

    @staticmethod
    def _env_float(name: str, default: float) -> float:
        val = os.getenv(name)
        if val is None:
            return default
        try:
            return float(val)
        except Exception:
            return default

    @staticmethod
    def _error_chain(exc: Exception) -> str:
        parts = []
        cur = exc
        depth = 0
        while cur is not None and depth < 6:
            parts.append(f"{type(cur).__name__}: {cur}")
            cur = cur.__cause__ or cur.__context__
            depth += 1
        return " <- ".join(parts)

    @staticmethod
    def _resolve_browser_executable() -> str:
        explicit = os.getenv("BROWSER_USE_EXECUTABLE_PATH", "").strip()
        if explicit and os.path.isfile(explicit):
            return explicit
        candidates = [
            "/usr/local/bin/chrome",
            "/usr/bin/google-chrome",
            "/usr/bin/google-chrome-stable",
            "/usr/bin/chromium",
            "/usr/bin/chromium-browser",
        ]
        for path in candidates:
            if os.path.isfile(path):
                return path
        return ""

    @staticmethod
    def _normalize_model_name(model: Any) -> str:
        name = str(model or "").strip()
        if not name:
            return ""
        if name.startswith("bu-") or name.startswith("browser-use/"):
            return name
        if "@" in name:
            # RAGFlow model aliases may include provider suffix, e.g. qwen3.5-flash@Tongyi-Qianwen.
            # browser-use OpenAI-compatible adapters need the pure model name.
            name = name.split("@", 1)[0].strip()
        return name

    @staticmethod
    def _safe_path_segment(value: Any) -> str:
        token = str(value or "").strip()
        if not token:
            return "unknown"
        token = re.sub(r"[^A-Za-z0-9._-]+", "_", token)
        return token.strip("._-") or "unknown"

    def _resolve_persistent_profile_dir(self) -> str:
        root = os.path.join(tempfile.gettempdir(), "ragflow_browser_use_profiles")
        tenant = self._safe_path_segment(self._canvas.get_tenant_id())
        raw_canvas_id = getattr(self._canvas, "_id", "")
        if not raw_canvas_id:
            graph_text = json.dumps(
                self._canvas.dsl.get("graph", {}),
                sort_keys=True,
                ensure_ascii=False,
            )
            raw_canvas_id = (
                f"dsl_{hashlib.sha1(graph_text.encode('utf-8')).hexdigest()[:12]}"
            )
        canvas_id = self._safe_path_segment(raw_canvas_id)
        node_id = self._safe_path_segment(self._id)
        return os.path.join(root, tenant, canvas_id, node_id)

    def _should_persist_session(self) -> bool:
        return bool(self._param.persist_session)

    def _infer_provider_name(self, cfg: dict[str, Any]) -> str:
        provider = str(cfg.get("llm_factory") or "").strip()
        if provider:
            return provider
        llm_id = str(self._param.llm_id or "")
        if "@" in llm_id:
            return llm_id.split("@", 1)[1].strip()
        return ""

    def _resolve_openai_compatible_base_url(self, cfg: dict[str, Any], model_name: str) -> str:
        explicit = str(cfg.get("base_url") or cfg.get("api_base") or "").strip()
        if explicit:
            return explicit

        provider = self._infer_provider_name(cfg)
        fallback = str(FACTORY_DEFAULT_BASE_URL.get(provider, "")).strip()
        if fallback:
            logging.info(
                "Browser filled empty base_url with provider default. provider=%s, model=%s, base_url=%s",
                provider,
                model_name,
                fallback,
            )
            return fallback
        return ""

    def _build_browser_llm(self):
        from browser_use.llm import ChatBrowserUse, ChatOpenAI

        chat_model_config = get_model_config_by_type_and_name(
            self._canvas.get_tenant_id(),
            TenantLLMService.llm_id2llm_type(self._param.llm_id),
            self._param.llm_id,
        )
        cfg = self._as_model_config_dict(chat_model_config)
        model_name = self._normalize_model_name(cfg.get("model_name") or cfg.get("model") or self._param.llm_id)
        if not model_name:
            raise ValueError(f"Invalid model config for Browser llm_id={self._param.llm_id}")
        base_url = self._resolve_openai_compatible_base_url(cfg, model_name)
        llm_timeout = self._env_float("RAGFLOW_BROWSER_USE_LLM_TIMEOUT", 120.0)
        logging.info(
            "Browser building LLM adapter. llm_id=%s, model=%s, base_url=%s, timeout=%s, max_retries=%s",
            self._param.llm_id,
            model_name,
            base_url,
            llm_timeout,
            self._param.max_retries,
        )

        # ChatBrowserUse only supports bu-* models. For tenant models, use OpenAI-compatible adapter.
        if model_name.startswith("bu-") or model_name.startswith("browser-use/"):
            llm_kwargs = {
                "model": model_name,
                "api_key": cfg.get("api_key"),
                "base_url": base_url,
                "temperature": self._param.temperature,
                "timeout": llm_timeout,
                "max_retries": self._param.max_retries,
            }
            return self._build_with_signature(ChatBrowserUse, llm_kwargs)

        llm_kwargs = {
            "model": model_name,
            "api_key": cfg.get("api_key"),
            "base_url": base_url,
            "temperature": self._param.temperature,
            "timeout": llm_timeout,
            "max_retries": self._param.max_retries,
        }
        return self._build_with_signature(ChatOpenAI, llm_kwargs)

    async def _run_browser_use_async(
        self,
        task_text: str,
        download_dir: str,
        available_file_paths: list[str] | None = None,
        profile_dir: str | None = None,
    ):
        from browser_use import Agent as BrowserUseAgent

        llm = self._build_browser_llm()
        agent_kwargs: dict[str, Any] = {"task": task_text, "llm": llm}
        browser_obj = None
        available_file_paths = available_file_paths or []
        logging.info(
            "Browser available_file_paths prepared. count=%s, paths=%s",
            len(available_file_paths),
            available_file_paths,
        )

        try:
            import browser_use as browser_use_pkg

            browser_cls = getattr(browser_use_pkg, "Browser", None)
            if browser_cls:
                enable_default_extensions = self._env_truthy("RAGFLOW_BROWSER_USE_ENABLE_DEFAULT_EXTENSIONS", False)
                executable_path = self._resolve_browser_executable()
                browser_kwargs = {
                    "headless": self._param.headless,
                    "downloads_path": download_dir,
                    "downloads_dir": download_dir,
                    "save_downloads_path": download_dir,
                    "executable_path": executable_path,
                    # Docker often runs as root without user namespaces; disable sandbox by default.
                    "chromium_sandbox": self._env_truthy("RAGFLOW_BROWSER_USE_CHROMIUM_SANDBOX", False),
                    # Disable runtime extension download by default for intranet/offline environments.
                    # Enable only when explicitly required and extensions are pre-cached.
                    "enable_default_extensions": enable_default_extensions,
                }
                if profile_dir:
                    browser_kwargs["user_data_dir"] = profile_dir
                    browser_kwargs["profile_directory"] = profile_dir
                if not executable_path:
                    logging.warning(
                        "Browser no local browser executable found. "
                        "Set BROWSER_USE_EXECUTABLE_PATH or preinstall chromium in image to avoid runtime playwright install."
                    )
                browser_obj = self._build_with_signature(browser_cls, browser_kwargs)

                sig = inspect.signature(BrowserUseAgent)
                if "browser" in sig.parameters:
                    agent_kwargs["browser"] = browser_obj
                elif "browser_context" in sig.parameters:
                    agent_kwargs["browser_context"] = browser_obj
                if "available_file_paths" in sig.parameters:
                    agent_kwargs["available_file_paths"] = available_file_paths
                    logging.info(
                        "Browser injecting available_file_paths into Agent kwargs. count=%s",
                        len(available_file_paths),
                    )
                elif available_file_paths:
                    logging.warning(
                        "Browser Agent signature has no available_file_paths parameter. paths=%s",
                        available_file_paths,
                    )
        except Exception as e:
            logging.warning("Browser browser context customization skipped: %s", e)

        agent = self._build_with_signature(BrowserUseAgent, agent_kwargs)

        history = None
        run_fn = getattr(agent, "run", None)
        if run_fn is None:
            raise RuntimeError("browser-use Agent does not provide run().")

        run_kwargs = {"max_steps": self._param.max_steps}
        try:
            if inspect.iscoroutinefunction(run_fn):
                history = await run_fn(**run_kwargs)
            else:
                history = await asyncio.to_thread(run_fn, **run_kwargs)
        except Exception as e:
            logging.error("Browser agent.run failed. error_chain=%s", self._error_chain(e))
            logging.exception("Browser agent.run traceback")
            raise

        if browser_obj:
            close_fn = getattr(browser_obj, "close", None)
            if close_fn:
                try:
                    if inspect.iscoroutinefunction(close_fn):
                        await close_fn()
                    else:
                        await asyncio.to_thread(close_fn)
                except Exception:
                    pass

        return history

    def _prepare_upload_files(self, upload_dir: str) -> list[dict[str, Any]]:
        upload_refs = self._extract_ids(self._param.upload_sources)
        prepared = []
        for file_ref in upload_refs:
            if self._is_http_url(file_ref):
                prepared_url_file = self._prepare_upload_url_file(file_ref, upload_dir)
                if prepared_url_file:
                    prepared.append(prepared_url_file)
                continue

            file_id = file_ref
            exists, file = FileService.get_by_id(file_id)
            if not exists:
                logging.warning("Browser upload file_id not found: %s", file_id)
                continue
            blob = settings.STORAGE_IMPL.get(file.parent_id, file.location)
            if not blob:
                logging.warning("Browser upload blob not found: %s", file_id)
                continue
            local_name = os.path.basename(file.location) if file.location else (file.name or f"{file_id}.bin")
            local_path = os.path.join(upload_dir, local_name)
            index = 1
            while os.path.exists(local_path):
                stem, ext = os.path.splitext(local_name)
                local_path = os.path.join(upload_dir, f"{stem}_{index}{ext}")
                index += 1
            with open(local_path, "wb") as f:
                f.write(blob)
            prepared.append(
                {
                    "file_id": file.id,
                    "name": file.name,
                    "size": file.size,
                    "local_path": local_path,
                }
            )
        return prepared

    def _save_downloads(self, download_dir: str, parent_id: str) -> list[dict[str, Any]]:
        downloaded_files: list[dict[str, Any]] = []
        exists, folder = FileService.get_by_id(parent_id)
        if not exists or folder.type != FileType.FOLDER.value:
            raise ValueError(f"RAGFlow target folder does not exist or is not a folder: {parent_id}")

        for path in Path(download_dir).rglob("*"):
            if not path.is_file():
                continue
            blob = path.read_bytes()
            if not blob:
                continue
            display_name = duplicate_name(FileService.query, name=path.name, parent_id=parent_id)
            settings.STORAGE_IMPL.put(parent_id, display_name, blob)
            file_data = {
                "id": get_uuid(),
                "parent_id": parent_id,
                "tenant_id": self._canvas.get_tenant_id(),
                "created_by": self._canvas.get_tenant_id(),
                "type": filename_type(display_name),
                "name": display_name,
                "location": display_name,
                "size": len(blob),
            }
            inserted = FileService.insert(file_data)
            downloaded_files.append(
                {
                    "file_id": inserted.id,
                    "name": inserted.name,
                    "size": inserted.size,
                    "parent_id": inserted.parent_id,
                }
            )
        return downloaded_files

    @staticmethod
    def _extract_history_text(history: Any) -> str:
        if history is None:
            return ""
        if isinstance(history, str):
            return history
        if isinstance(history, dict):
            for key in ("final_result", "result", "answer", "content", "message"):
                if key in history and history[key]:
                    return str(history[key])
            return json.dumps(history, ensure_ascii=False)
        if isinstance(history, list):
            if not history:
                return ""
            return json.dumps(history[-1], ensure_ascii=False)
        return str(history)

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 20 * 60)))
    def _invoke(self, **kwargs):
        try:
            user_prompt = self._resolve_text(kwargs.get("prompts", self._param.prompts))
            with tempfile.TemporaryDirectory(prefix="browser_use_upload_") as upload_dir, tempfile.TemporaryDirectory(
                prefix="browser_use_download_"
            ) as download_dir:
                uploaded_files = self._prepare_upload_files(upload_dir)

                upload_lines = [
                    f"- file_id={item['file_id']}, name={item['name']}, local_path={item['local_path']}"
                    for item in uploaded_files
                ]
                task_text = user_prompt
                if upload_lines:
                    task_text += (
                        "\n\nYou can upload files from these local paths when operating web pages:\n"
                        + "\n".join(upload_lines)
                    )

                upload_local_paths = [item.get("local_path", "") for item in uploaded_files if item.get("local_path")]
                profile_dir = None
                if self._should_persist_session():
                    profile_dir = self._resolve_persistent_profile_dir()
                    os.makedirs(profile_dir, exist_ok=True)
                    logging.info(
                        "Browser using persistent profile dir: %s", profile_dir
                    )
                else:
                    try:
                        profile_dir = tempfile.mkdtemp(prefix="browser_use_profile_")
                    except Exception:
                        profile_dir = None
                history = asyncio.run(
                    self._run_browser_use_async(
                        task_text, download_dir, upload_local_paths, profile_dir
                    )
                )
                target_dir_id = FileService.get_root_folder(self._canvas.get_tenant_id())["id"]
                downloaded_files = self._save_downloads(download_dir, target_dir_id)

                self.set_output("content", self._extract_history_text(history))
                self.set_output("downloaded_files", downloaded_files)
                if profile_dir and not self._should_persist_session():
                    shutil.rmtree(profile_dir, ignore_errors=True)
                return self.output()
        except Exception as e:
            logging.exception("Browser invoke failed")
            self.set_output("_ERROR", str(e))
            return self.output()

    def thoughts(self) -> str:
        return "Planning and executing browser actions..."
