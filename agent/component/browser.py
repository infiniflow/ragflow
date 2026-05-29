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
from urllib.error import HTTPError, URLError
from urllib.parse import unquote, urlparse
from urllib.request import Request, urlopen

from agent.component.base import ComponentBase
from agent.component.llm import LLMParam
from api.db import FileType
from api.db.joint_services.tenant_model_service import get_model_config_from_provider_instance, get_model_type_by_name
from api.db.services import duplicate_name
from api.db.services.file_service import FileService
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
        self.enable_default_extensions = False
        self.chromium_sandbox = False
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
        self.check_boolean(self.enable_default_extensions, "[Browser] Enable default extensions")
        self.check_boolean(self.chromium_sandbox, "[Browser] Chromium sandbox")
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

    @staticmethod
    def _resolve_upload_url_max_bytes() -> int:
        raw = str(os.getenv("RAGFLOW_BROWSER_UPLOAD_URL_MAX_BYTES", "") or "").strip()
        default_max_bytes = 100 * 1024 * 1024
        if not raw:
            return default_max_bytes
        try:
            parsed = int(raw)
            return parsed if parsed > 0 else default_max_bytes
        except (TypeError, ValueError):
            return default_max_bytes

    @staticmethod
    def _restore_env_var(key: str, value: str | None):
        if value is None:
            os.environ.pop(key, None)
            return
        os.environ[key] = value

    def _prepare_upload_url_file(self, url: str, upload_dir: str) -> dict[str, Any] | None:
        max_bytes = self._resolve_upload_url_max_bytes()
        local_path = ""
        local_name = ""
        total_size = 0
        try:
            req = Request(url, headers={"User-Agent": "RAGFlow-Browser-Node/1.0"})
            with urlopen(req, timeout=30) as response:
                local_name = self._extract_url_filename(url, response.headers)

                local_path = os.path.join(upload_dir, local_name)
                index = 1
                while os.path.exists(local_path):
                    stem, ext = os.path.splitext(local_name)
                    local_path = os.path.join(upload_dir, f"{stem}_{index}{ext}")
                    index += 1

                with open(local_path, "wb") as f:
                    while True:
                        chunk = response.read(1024 * 1024)
                        if not chunk:
                            break
                        total_size += len(chunk)
                        if total_size > max_bytes:
                            raise ValueError(f"upload url file exceeds max size limit: {max_bytes}")
                        f.write(chunk)
        except (HTTPError, URLError, OSError, TimeoutError, ValueError) as e:
            if local_path and os.path.exists(local_path):
                try:
                    os.remove(local_path)
                except OSError:
                    pass
            logging.warning("Browser failed to fetch upload url. url=%s, error=%s", url, e)
            return None

        if total_size <= 0:
            if local_path and os.path.exists(local_path):
                try:
                    os.remove(local_path)
                except OSError:
                    pass
            logging.warning("Browser upload url returned empty content: %s", url)
            return None

        return {
            "file_id": "",
            "name": local_name,
            "size": total_size,
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
            except (AttributeError, TypeError, ValueError):
                return {}
        result = {}
        for key in ("model", "model_name", "llm_name", "llm_factory", "api_key", "base_url", "api_base", "temperature"):
            val = getattr(cfg_obj, key, None)
            if val not in (None, ""):
                result[key] = val
        return result

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
        explicit_candidates = [
            os.getenv("BROWSER_USE_EXECUTABLE_PATH", "").strip(),
            os.getenv("BROWSER_USE_BROWSER_BINARY_PATH", "").strip(),
            os.getenv("BROWSER_USE_CHROME_BINARY_PATH", "").strip(),
        ]
        for explicit in explicit_candidates:
            if explicit and os.path.isfile(explicit) and os.access(explicit, os.X_OK):
                return explicit
        candidates = [
            "/opt/chrome/chrome",
            "/usr/local/bin/chrome",
            "/usr/local/bin/google-chrome",
            "/usr/bin/google-chrome",
            "/usr/bin/google-chrome-stable",
            "/usr/bin/chromium",
            "/usr/bin/chromium-browser",
        ]
        for path in candidates:
            if os.path.isfile(path) and os.access(path, os.X_OK):
                return path
        for cmd in ("chrome", "google-chrome", "google-chrome-stable", "chromium", "chromium-browser"):
            path = shutil.which(cmd)
            if path and os.path.isfile(path) and os.access(path, os.X_OK):
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

    def _resolve_openai_compatible_base_url(self, cfg: dict[str, Any]) -> str:
        explicit = str(cfg.get("base_url") or cfg.get("api_base") or "").strip()
        if explicit:
            return explicit

        provider = self._infer_provider_name(cfg)
        fallback = str(FACTORY_DEFAULT_BASE_URL.get(provider, "")).strip()
        return fallback if fallback else ""

    def _build_browser_llm(self):
        from browser_use.llm import ChatBrowserUse, ChatOpenAI

        chat_model_config = get_model_config_from_provider_instance(
            self._canvas.get_tenant_id(),
            get_model_type_by_name(self._param.llm_id),
            self._param.llm_id,
        )
        cfg = self._as_model_config_dict(chat_model_config)
        model_name = self._normalize_model_name(cfg.get("model_name") or cfg.get("model") or self._param.llm_id)
        if not model_name:
            raise ValueError(f"Invalid model config for Browser llm_id={self._param.llm_id}")
        base_url = self._resolve_openai_compatible_base_url(cfg)

        # ChatBrowserUse only supports bu-* models. For tenant models, use OpenAI-compatible adapter.
        if model_name.startswith("bu-") or model_name.startswith("browser-use/"):
            llm_kwargs = {
                "model": model_name,
                "api_key": cfg.get("api_key"),
                "base_url": base_url,
                "temperature": self._param.temperature,
                "max_retries": self._param.max_retries,
            }
            llm_kwargs = {k: v for k, v in llm_kwargs.items() if v not in (None, "")}
            return ChatBrowserUse(**llm_kwargs)

        llm_kwargs = {
            "model": model_name,
            "api_key": cfg.get("api_key"),
            "base_url": base_url,
            "temperature": self._param.temperature,
            "max_retries": self._param.max_retries,
        }
        llm_kwargs = {k: v for k, v in llm_kwargs.items() if v not in (None, "")}
        return ChatOpenAI(**llm_kwargs)

    async def _run_browser_use_async(
        self,
        task_text: str,
        download_dir: str,
        available_file_paths: list[str] | None = None,
        profile_dir: str | None = None,
    ):
        from browser_use import Agent as BrowserUseAgent, Browser as BrowserUseBrowser

        llm = self._build_browser_llm()
        # NOTE:
        # _invoke() uses asyncio.run(), which creates a fresh event loop per task run.
        # Reusing a Browser object created by a previous loop can deadlock/timestamp out
        # in browser-use watchdog handlers on subsequent runs.
        # We keep persistent user_data_dir for session continuity, but we do not keep
        # browser instances alive across runs.
        available_file_paths = available_file_paths or []
        agent_kwargs: dict[str, Any] = {
            "task": task_text,
            "llm": llm,
            "available_file_paths": available_file_paths,
        }
        browser_obj = None
        previous_disable_extensions = os.environ.get("BROWSER_USE_DISABLE_EXTENSIONS")
        previous_browser_binary_path = os.environ.get("BROWSER_USE_BROWSER_BINARY_PATH")

        try:
            enable_default_extensions = bool(self._param.enable_default_extensions)
            if not enable_default_extensions:
                os.environ["BROWSER_USE_DISABLE_EXTENSIONS"] = "1"
            else:
                os.environ.pop("BROWSER_USE_DISABLE_EXTENSIONS", None)

            executable_path = self._resolve_browser_executable()
            browser_kwargs = {
                "headless": self._param.headless,
                "downloads_path": download_dir,
                # Docker often runs as root without user namespaces; disable sandbox by default.
                "chromium_sandbox": bool(self._param.chromium_sandbox),
                # Disable runtime extension download by default for intranet/offline environments.
                # Enable only when explicitly required and extensions are pre-cached.
                "enable_default_extensions": enable_default_extensions,
            }
            if executable_path:
                browser_kwargs["executable_path"] = executable_path
                # Keep browser-use watchdog fallback in sync with our resolved path.
                os.environ["BROWSER_USE_BROWSER_BINARY_PATH"] = executable_path
            else:
                logging.warning(
                    "Browser no local browser executable found. "
                    "Set BROWSER_USE_EXECUTABLE_PATH or preinstall chromium in image to avoid runtime playwright install."
                )
            if profile_dir:
                browser_kwargs["user_data_dir"] = profile_dir
                # browser-use expects profile_directory to be a profile name
                # such as "Default" / "Profile 1", not an absolute path.
                browser_kwargs["profile_directory"] = "Default"

            browser_obj = BrowserUseBrowser(**browser_kwargs)
            agent_kwargs["browser"] = browser_obj
        except (OSError, RuntimeError, TypeError, ValueError) as e:
            logging.warning("Browser browser context customization skipped: %s", e)

        agent = BrowserUseAgent(**agent_kwargs)

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
        finally:
            if browser_obj:
                close_fn = getattr(browser_obj, "close", None)
                if close_fn:
                    try:
                        if inspect.iscoroutinefunction(close_fn):
                            await close_fn()
                        else:
                            await asyncio.to_thread(close_fn)
                    except Exception as close_err:
                        logging.warning("Browser failed to close browser object cleanly: %s", close_err)
            self._restore_env_var("BROWSER_USE_DISABLE_EXTENSIONS", previous_disable_extensions)
            self._restore_env_var("BROWSER_USE_BROWSER_BINARY_PATH", previous_browser_binary_path)

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
            try:
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
            except OSError as e:
                logging.warning("Browser failed to prepare upload file. file_id=%s, error=%s", file_id, e)
                continue
            except Exception as e:
                logging.warning("Browser failed to fetch upload blob. file_id=%s, error=%s", file_id, e)
                continue
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
        tenant_id = self._canvas.get_tenant_id()
        storage_put = settings.STORAGE_IMPL.put
        storage_rm = getattr(settings.STORAGE_IMPL, "rm", None)
        insert_file = FileService.insert

        for path in Path(download_dir).rglob("*"):
            if not path.is_file():
                continue
            try:
                if path.stat().st_size <= 0:
                    continue
                blob = path.read_bytes()
            except OSError as e:
                logging.warning("Browser failed to read downloaded file. path=%s, error=%s", path, e)
                continue
            if not blob:
                continue
            display_name = ""
            blob_stored = False
            try:
                display_name = duplicate_name(FileService.query, name=path.name, parent_id=parent_id)
                storage_put(parent_id, display_name, blob)
                blob_stored = True
                file_data = {
                    "id": get_uuid(),
                    "parent_id": parent_id,
                    "tenant_id": tenant_id,
                    "created_by": tenant_id,
                    "type": filename_type(display_name),
                    "name": display_name,
                    "location": display_name,
                    "size": len(blob),
                }
                inserted = insert_file(file_data)
                downloaded_files.append(
                    {
                        "file_id": inserted.id,
                        "name": inserted.name,
                        "size": inserted.size,
                        "parent_id": inserted.parent_id,
                    }
                )
            except Exception as e:
                if blob_stored and callable(storage_rm):
                    try:
                        storage_rm(parent_id, display_name)
                    except Exception as rollback_err:
                        logging.warning(
                            "Browser rollback stored download failed. path=%s, parent_id=%s, display_name=%s, error=%s",
                            path,
                            parent_id,
                            display_name,
                            rollback_err,
                        )
                logging.error(
                    "Browser failed to save download. path=%s, tenant_id=%s, parent_id=%s, display_name=%s, error=%s",
                    path,
                    tenant_id,
                    parent_id,
                    display_name,
                    e,
                )
                continue
        return downloaded_files

    @staticmethod
    def _extract_history_text(history: Any) -> str:
        if history is None:
            return ""

        def pick_final_result(value: Any) -> str:
            if value is None:
                return ""
            if isinstance(value, str):
                return value.strip()
            if isinstance(value, (int, float, bool)):
                return str(value)
            return ""

        # Only trust browser-use's explicit final_result API/property.
        final_result_fn = getattr(history, "final_result", None)
        if callable(final_result_fn):
            try:
                final_result_value = final_result_fn()
                return pick_final_result(final_result_value)
            except Exception:
                return ""
        return pick_final_result(final_result_fn)

    @timeout(int(os.environ.get("COMPONENT_EXEC_TIMEOUT", 20 * 60)))
    def _invoke(self, **kwargs):
        profile_dir = None
        persist_session = self._should_persist_session()
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
                if persist_session:
                    profile_dir = self._resolve_persistent_profile_dir()
                    os.makedirs(profile_dir, exist_ok=True)
                else:
                    try:
                        profile_dir = tempfile.mkdtemp(prefix="browser_use_profile_")
                    except OSError:
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
                return self.output()
        except Exception as e:
            logging.exception("Browser invoke failed")
            self.set_output("_ERROR", str(e))
            return self.output()
        finally:
            if profile_dir and not persist_session:
                shutil.rmtree(profile_dir, ignore_errors=True)

    def thoughts(self) -> str:
        return "Planning and executing browser actions..."
