#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
import base64
import contextvars
import datetime
import inspect
import json
import logging
import re
import time
from concurrent.futures import ThreadPoolExecutor
from copy import deepcopy
from functools import partial
from typing import Any, Tuple, Union

from agent.component import component_class
from agent.component.base import ComponentBase
from agent.dsl_migration import normalize_chunker_dsl
from api.db.joint_services.tenant_model_service import get_tenant_default_model_by_type
from api.db.services.file_service import FileService
from api.db.services.llm_service import LLMBundle
from api.db.services.task_service import has_canceled
from common.constants import LLMType
from common.llm_request_context import set_llm_request_context, reset_llm_request_context
from common.exceptions import TaskCanceledException
from common.misc_utils import get_uuid, hash_str2int
from common.token_utils import token_usage_sink, langfuse_run_attrs
from rag.prompts.generator import chunks_format
from rag.utils.redis_conn import REDIS_CONN
from rag.utils.tts_cache import synthesize_with_cache

_logger = logging.getLogger(__name__)


class Graph:
    """
    dsl = {
        "components": {
            "begin": {
                "obj":{
                    "component_name": "Begin",
                    "params": {},
                },
                "downstream": ["answer_0"],
                "upstream": [],
            },
            "retrieval_0": {
                "obj": {
                    "component_name": "Retrieval",
                    "params": {}
                },
                "downstream": ["generate_0"],
                "upstream": ["answer_0"],
            },
            "generate_0": {
                "obj": {
                    "component_name": "Generate",
                    "params": {}
                },
                "downstream": ["answer_0"],
                "upstream": ["retrieval_0"],
            }
        },
        "history": [],
        "path": ["begin"],
        "retrieval": {"chunks": [], "doc_aggs": []},
        "globals": {
            "sys.query": "",
            "sys.user_id": tenant_id,
            "sys.conversation_turns": 0,
            "sys.files": []
        }
    }
    """

    def __init__(self, dsl: str, tenant_id=None, task_id=None, custom_header=None):
        self.path = []
        self.components = {}
        self.error = ""
        # Accept legacy DSL on read, but keep the in-memory canvas in the latest schema.
        self.dsl = normalize_chunker_dsl(json.loads(dsl))
        self._tenant_id = tenant_id
        self.task_id = task_id if task_id else get_uuid()
        self.custom_header = custom_header
        self._thread_pool = ThreadPoolExecutor(max_workers=5)
        self.load()

    def load(self):
        self.components = self.dsl["components"]
        cpn_nms = set([])
        for k, cpn in self.components.items():
            cpn_nms.add(cpn["obj"]["component_name"])
            param = component_class(cpn["obj"]["component_name"] + "Param")()
            cpn["obj"]["params"]["custom_header"] = self.custom_header
            param.update(cpn["obj"]["params"])
            try:
                param.check()
            except Exception as e:
                raise ValueError(self.get_component_name(k) + f": {e}")

            cpn["obj"] = component_class(cpn["obj"]["component_name"])(self, k, param)

        self.path = self.dsl["path"]

    def __str__(self):
        self.dsl["path"] = self.path
        self.dsl["task_id"] = self.task_id
        dsl = {"components": {}}
        for k in self.dsl.keys():
            if k in ["components"]:
                continue
            try:
                dsl[k] = deepcopy(self.dsl[k])
            except Exception as e:
                logging.warning("Graph.__str__: deepcopy failed for dsl key '%s' (type=%s): %s. Using shallow reference.", k, type(self.dsl[k]).__name__, e)
                dsl[k] = self.dsl[k]

        for k, cpn in self.components.items():
            if k not in dsl["components"]:
                dsl["components"][k] = {}
            for c in cpn.keys():
                if c == "obj":
                    dsl["components"][k][c] = json.loads(str(cpn["obj"]))
                    continue
                try:
                    dsl["components"][k][c] = deepcopy(cpn[c])
                except Exception as e:
                    logging.warning("Graph.__str__: deepcopy failed for component '%s' key '%s' (type=%s): %s. Using shallow reference.", k, c, type(cpn[c]).__name__, e)
                    dsl["components"][k][c] = cpn[c]

        def _serialize_default(obj):
            if callable(obj):
                return None
            logging.warning("Graph.__str__: JSON fallback via str() for type=%s", type(obj).__name__)
            return str(obj)

        return json.dumps(dsl, ensure_ascii=False, default=_serialize_default)

    def reset(self):
        self.path = []
        for k, cpn in self.components.items():
            self.components[k]["obj"].reset()
        try:
            REDIS_CONN.delete(f"{self.task_id}-logs")
            REDIS_CONN.delete(f"{self.task_id}-cancel")
        except Exception as e:
            logging.exception(e)

    def close(self):
        from common.mcp_tool_call_conn import MCPToolCallSession

        seen = set()
        for cpn in self.components.values():
            obj = cpn.get("obj")
            if obj and hasattr(obj, "tools"):
                for tool in obj.tools.values():
                    if isinstance(tool, MCPToolCallSession) and id(tool) not in seen:
                        seen.add(id(tool))
                        try:
                            tool.close_sync(timeout=3)
                        except Exception:
                            pass

    def get_component_name(self, cid):
        for n in self.dsl.get("graph", {}).get("nodes", []):
            if cid == n["id"]:
                return n["data"]["name"]
        return ""

    def run(self, **kwargs):
        raise NotImplementedError()

    def get_component(self, cpn_id) -> Union[None, dict[str, Any]]:
        return self.components.get(cpn_id)

    def get_component_obj(self, cpn_id) -> ComponentBase:
        return self.components.get(cpn_id)["obj"]

    def get_component_type(self, cpn_id) -> str:
        return self.components.get(cpn_id)["obj"].component_name

    def get_component_input_form(self, cpn_id) -> dict:
        return self.components.get(cpn_id)["obj"].get_input_form()

    def get_tenant_id(self):
        return self._tenant_id

    def get_value_with_variable(self, value: str) -> Any:
        pat = re.compile(r"\{* *\{([a-zA-Z:0-9]+@[A-Za-z0-9_.-]+|sys\.[A-Za-z0-9_.]+|env\.[A-Za-z0-9_.]+)\} *\}*")
        out_parts = []
        last = 0

        for m in pat.finditer(value):
            out_parts.append(value[last : m.start()])
            key = m.group(1)
            v = self.get_variable_value(key)
            if v is None:
                rep = ""
            elif isinstance(v, partial):
                buf = []
                for chunk in v():
                    buf.append(chunk)
                rep = "".join(buf)
            elif isinstance(v, str):
                rep = v
            else:
                rep = json.dumps(v, ensure_ascii=False)

            out_parts.append(rep)
            last = m.end()

        out_parts.append(value[last:])
        return "".join(out_parts)

    def get_variable_value(self, exp: str) -> Any:
        exp = exp.strip("{").strip("}").strip(" ").strip("{").strip("}")
        if exp.find("@") < 0:
            return self.globals[exp]
        # Split from the left with maxsplit=1 so the trailing var_nm can
        # legitimately contain '@' characters (defensive: although the
        # upstream regex in `get_value_with_variable` constrains `var_nm`
        # to `[A-Za-z0-9_.-]+`, direct callers of this method may pass
        # any string and should not raise `ValueError: too many values
        # to unpack`). `cpn_id` is system-generated and never contains '@'.
        cpn_id, var_nm = exp.split("@", 1)
        cpn = self.get_component(cpn_id)
        if not cpn:
            raise Exception(f"Can't find variable: '{cpn_id}@{var_nm}'")
        parts = var_nm.split(".", 1)
        root_key = parts[0]
        rest = parts[1] if len(parts) > 1 else ""
        root_val = cpn["obj"].output(root_key)

        if not rest:
            return root_val
        return self.get_variable_param_value(root_val, rest)

    def get_variable_param_value(self, obj: Any, path: str) -> Any:
        cur = obj
        if not path:
            return cur
        for key in path.split("."):
            if cur is None:
                return None

            if isinstance(cur, str):
                try:
                    cur = json.loads(cur)
                except Exception:
                    return None

            if isinstance(cur, dict):
                cur = cur.get(key)
                continue

            if isinstance(cur, (list, tuple)):
                try:
                    idx = int(key)
                    cur = cur[idx]
                except Exception:
                    return None
                continue

            cur = getattr(cur, key, None)
        return cur

    def set_variable_value(self, exp: str, value):
        exp = exp.strip("{").strip("}").strip(" ").strip("{").strip("}")
        if exp.find("@") < 0:
            self.globals[exp] = value
            return
        # See `get_variable_value` above for rationale on `maxsplit=1`.
        # Without it, a var_nm containing '@' would raise
        # `ValueError: too many values to unpack` instead of being preserved.
        cpn_id, var_nm = exp.split("@", 1)
        cpn = self.get_component(cpn_id)
        if not cpn:
            raise Exception(f"Can't find variable: '{cpn_id}@{var_nm}'")
        parts = var_nm.split(".", 1)
        root_key = parts[0]
        rest = parts[1] if len(parts) > 1 else ""
        if not rest:
            cpn["obj"].set_output(root_key, value)
            return
        root_val = cpn["obj"].output(root_key)
        if not root_val:
            root_val = {}
        cpn["obj"].set_output(root_key, self.set_variable_param_value(root_val, rest, value))

    def set_variable_param_value(self, obj: Any, path: str, value) -> Any:
        cur = obj
        keys = path.split(".")
        if not path:
            return value
        for key in keys[:-1]:
            if key not in cur or not isinstance(cur[key], dict):
                cur[key] = {}
            cur = cur[key]
        cur[keys[-1]] = value
        return obj

    def is_canceled(self) -> bool:
        return has_canceled(self.task_id)

    def cancel_task(self) -> bool:
        try:
            REDIS_CONN.set(f"{self.task_id}-cancel", "x")
        except Exception as e:
            logging.exception(e)
            return False
        return True


class Canvas(Graph):
    def __init__(self, dsl: str, tenant_id=None, task_id=None, canvas_id=None, custom_header=None):
        self.globals = {
            "sys.query": "",
            "sys.user_id": tenant_id,
            "sys.conversation_turns": 0,
            "sys.files": [],
            "sys.history": [],
            "sys.date": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S"),
        }
        self.variables = {}
        # Aggregated provider token usage (prompt/completion/total) across every LLM
        # call in a single run — query rewriting, cross-language translation, tool
        # reasoning and the final answer. Populated via the token_usage_sink context
        # variable that each LLMBundle chat call writes to. Reset at run() start.
        self._run_token_usage: dict = {"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0, "calls": 0}
        super().__init__(dsl, tenant_id, task_id, custom_header=custom_header)
        self._id = canvas_id

    def load(self):
        super().load()
        self.history = self.dsl["history"]
        if "globals" in self.dsl:
            self.globals = self.dsl["globals"]
            if "sys.history" not in self.globals:
                self.globals["sys.history"] = []
            if "sys.date" not in self.globals:
                self.globals["sys.date"] = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
        else:
            self.globals = {
                "sys.query": "",
                "sys.user_id": "",
                "sys.conversation_turns": 0,
                "sys.files": [],
                "sys.history": [],
                "sys.date": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S"),
            }
        if "variables" in self.dsl:
            self.variables = self.dsl["variables"]
        else:
            self.variables = {}

        self.retrieval = self.dsl["retrieval"]
        self.memory = self.dsl.get("memory", [])

    def __str__(self):
        self.dsl["history"] = self.history
        self.dsl["retrieval"] = self.retrieval
        self.dsl["memory"] = self.memory
        return super().__str__()

    def clear_history(self):
        self.history = []
        if isinstance(self.globals.get("sys.history"), list):
            self.globals["sys.history"] = []

    def reset(self, mem=False):
        super().reset()
        if not mem:
            self.history = []
            self.retrieval = []
            self.memory = []
        print(self.variables)
        for k in self.globals.keys():
            if k.startswith("sys."):
                if isinstance(self.globals[k], str):
                    self.globals[k] = ""
                elif isinstance(self.globals[k], int):
                    self.globals[k] = 0
                elif isinstance(self.globals[k], float):
                    self.globals[k] = 0
                elif isinstance(self.globals[k], list):
                    self.globals[k] = []
                elif isinstance(self.globals[k], dict):
                    self.globals[k] = {}
                else:
                    self.globals[k] = None
            if k.startswith("env."):
                key = k[4:]
                if key in self.variables:
                    variable = self.variables[key]
                    value = variable.get("value")
                    if value is not None:
                        self.globals[k] = value
                    else:
                        var_type = variable.get("type", "")
                        if var_type == "number":
                            self.globals[k] = 0
                        elif var_type == "boolean":
                            self.globals[k] = False
                        elif var_type == "object":
                            self.globals[k] = {}
                        elif var_type.startswith("array"):
                            self.globals[k] = []
                        else:  # "string" or unknown
                            self.globals[k] = ""
                else:
                    self.globals[k] = ""

    async def run(self, **kwargs):
        # Install a fresh per-run token usage sink and Langfuse correlation context,
        # and guarantee both are torn down when the run ends (even on early return or
        # exception) so later LLM calls in the same task never inherit a previous
        # run's sink or session/user attributes.
        self._run_token_usage = {"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0, "calls": 0}
        _lf_attrs = {}
        _user_id = kwargs.get("user_id")
        if _user_id:
            _lf_attrs["user_id"] = str(_user_id)[:200]
        _session_id = kwargs.get("session_id") or self._id
        if _session_id:
            _lf_attrs["session_id"] = str(_session_id)[:200]
        sink_token = token_usage_sink.set(self._run_token_usage)
        attrs_token = langfuse_run_attrs.set(_lf_attrs)
        # Forward the originating session/user to upstream LLM providers (as the
        # OpenAI `user` field) for the duration of this run, and reset afterwards so
        # the value never leaks to later calls in the same task. Reuse the same
        # session/user already derived above so both integrations stay consistent.
        _req_ctx_token = set_llm_request_context(
            session_id=_session_id,
            user_id=_user_id,
        )
        try:
            async for ev in self._run_impl(**kwargs):
                yield ev
        finally:
            # reset() can raise if the generator is closed from a different context
            # (e.g. client disconnect); fall back to clearing the values in that case.
            try:
                token_usage_sink.reset(sink_token)
            except ValueError:
                logging.debug("Failed to reset token usage ContextVar", exc_info=True)
                token_usage_sink.set(None)
            try:
                langfuse_run_attrs.reset(attrs_token)
            except ValueError:
                logging.debug("Failed to reset Langfuse run attributes ContextVar", exc_info=True)
                langfuse_run_attrs.set(None)
            reset_llm_request_context(_req_ctx_token)

    async def _run_impl(self, **kwargs):
        self.globals["sys.date"] = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
        st = time.perf_counter()
        self._loop = asyncio.get_running_loop()
        self.message_id = get_uuid()
        created_at = int(time.time())
        self.add_user_input(kwargs.get("query"))
        path_set = set(self.path)
        for k, cpn in self.components.items():
            if k in path_set:
                self.components[k]["obj"].reset(True)

        if kwargs.get("webhook_payload"):
            for k, cpn in self.components.items():
                if self.components[k]["obj"].component_name.lower() == "begin" and self.components[k]["obj"]._param.mode == "Webhook":
                    payload = kwargs.get("webhook_payload", {})
                    if "input" in payload:
                        self.components[k]["obj"].set_input_value("request", payload["input"])
                    for kk, vv in payload.items():
                        if kk == "input":
                            continue
                        self.components[k]["obj"].set_output(kk, vv)

        layout_recognize = None
        for cpn in self.components.values():
            if cpn["obj"].component_name.lower() == "begin":
                layout_recognize = getattr(cpn["obj"]._param, "layout_recognize", None)
                break

        for k in kwargs.keys():
            if k in ["query", "user_id", "files", "chat_template_kwargs"] and kwargs[k]:
                if k == "files":
                    self.globals[f"sys.{k}"] = await self.get_files_async(kwargs[k], layout_recognize)
                else:
                    self.globals[f"sys.{k}"] = kwargs[k]
        if not self.globals["sys.conversation_turns"]:
            self.globals["sys.conversation_turns"] = 0
        self.globals["sys.conversation_turns"] += 1
        is_resume = bool(self.path) and self.path[0].lower().find("userfillup") >= 0

        def decorate(event, dt):
            nonlocal created_at
            return {
                "event": event,
                # "conversation_id": "f3cc152b-24b0-4258-a1a1-7d5e9fc8a115",
                "message_id": self.message_id,
                "created_at": created_at,
                "task_id": self.task_id,
                "data": dt,
            }

        if not is_resume:
            self.path.append("begin")
            self.retrieval.append({"chunks": [], "doc_aggs": []})
        if self.is_canceled():
            msg = f"Task {self.task_id} has been canceled before starting."
            logging.info(msg)
            raise TaskCanceledException(msg)

        if not is_resume:
            yield decorate("workflow_started", {"inputs": kwargs.get("inputs")})
            _logger.debug(
                "[Canvas] Workflow started. Path: %s, Inputs: %s",
                [self.get_component_name(c) for c in self.path],
                json.dumps(kwargs.get("inputs", {}), ensure_ascii=False, default=str)[:500],
            )
        self.retrieval.append({"chunks": {}, "doc_aggs": {}})

        async def _run_batch(f, t):
            if self.is_canceled():
                msg = f"Task {self.task_id} has been canceled during batch execution."
                logging.info(msg)
                raise TaskCanceledException(msg)

            loop = asyncio.get_running_loop()
            tasks = []
            max_concurrency = getattr(self._thread_pool, "_max_workers", 5)
            sem = asyncio.Semaphore(max_concurrency)

            async def _invoke_one(cpn_obj, sync_fn, call_kwargs, use_async: bool):
                async with sem:
                    if use_async:
                        await cpn_obj.invoke_async(**(call_kwargs or {}))
                        return
                    # run_in_executor does not carry context variables into the worker
                    # thread; copy the current context so the LLM request context (the
                    # `user` forwarding), token usage sink, and Langfuse attributes set
                    # by run() remain visible to sync components.
                    bound_call = partial(sync_fn, **(call_kwargs or {}))
                    call_ctx = contextvars.copy_context()
                    await loop.run_in_executor(self._thread_pool, partial(call_ctx.run, bound_call))

            i = f
            while i < t:
                cpn = self.get_component_obj(self.path[i])
                task_fn = None
                call_kwargs = None

                if cpn.component_name.lower() in ["begin", "userfillup"]:
                    call_kwargs = {"inputs": kwargs.get("inputs", {})}
                    task_fn = cpn.invoke
                    i += 1
                else:
                    for _, ele in cpn.get_input_elements().items():
                        if isinstance(ele, dict) and ele.get("_cpn_id") and ele.get("_cpn_id") not in self.path[:i] and self.path[0].lower().find("userfillup") < 0:
                            self.path.pop(i)
                            t -= 1
                            break
                    else:
                        call_kwargs = cpn.get_input()
                        task_fn = cpn.invoke
                        i += 1

                if task_fn is None:
                    continue

                _logger.debug(
                    "[Canvas] Invoking component '%s' (%s) with inputs: %s",
                    self.get_component_name(self.path[i - 1]),
                    cpn.component_name,
                    json.dumps(call_kwargs, ensure_ascii=False, default=str)[:500],
                )

                fn_invoke_async = getattr(cpn, "_invoke_async", None)
                use_async = (fn_invoke_async and asyncio.iscoroutinefunction(fn_invoke_async)) or asyncio.iscoroutinefunction(getattr(cpn, "_invoke", None))
                tasks.append(asyncio.create_task(_invoke_one(cpn, task_fn, call_kwargs, use_async)))

            if tasks:
                await asyncio.gather(*tasks)

        def _node_finished(cpn_obj):
            outputs = cpn_obj.output()
            _logger.debug(
                "[Canvas] Component '%s' (%s) finished. Outputs: %s, Error: %s",
                self.get_component_name(cpn_obj._id),
                self.get_component_type(cpn_obj._id),
                json.dumps(outputs, ensure_ascii=False, default=str)[:500],
                cpn_obj.error(),
            )
            return decorate(
                "node_finished",
                {
                    "inputs": cpn_obj.get_input_values(),
                    "outputs": outputs,
                    "component_id": cpn_obj._id,
                    "component_name": self.get_component_name(cpn_obj._id),
                    "component_type": self.get_component_type(cpn_obj._id),
                    "error": cpn_obj.error(),
                    "elapsed_time": time.perf_counter() - cpn_obj.output("_created_time"),
                    "created_at": cpn_obj.output("_created_time"),
                },
            )

        self.error = ""
        idx = 0 if is_resume else len(self.path) - 1
        partials = []
        tts_mdl = None
        while idx < len(self.path):
            to = len(self.path)
            for i in range(idx, to):
                yield decorate(
                    "node_started",
                    {
                        "inputs": None,
                        "created_at": int(time.time()),
                        "component_id": self.path[i],
                        "component_name": self.get_component_name(self.path[i]),
                        "component_type": self.get_component_type(self.path[i]),
                        "thoughts": self.get_component_thoughts(self.path[i]),
                    },
                )
            await _run_batch(idx, to)
            to = len(self.path)
            # post-processing of components invocation
            for i in range(idx, to):
                cpn = self.get_component(self.path[i])
                cpn_obj = self.get_component_obj(self.path[i])
                if cpn_obj.component_name.lower() == "message":
                    if cpn_obj.get_param("auto_play"):
                        tts_model_config = get_tenant_default_model_by_type(self._tenant_id, LLMType.TTS)
                        tts_mdl = LLMBundle(self._tenant_id, tts_model_config)
                    if isinstance(cpn_obj.output("content"), partial):
                        _m = ""
                        buff_m = ""
                        stream = cpn_obj.output("content")()

                        async def _process_stream(m):
                            nonlocal buff_m, _m, tts_mdl
                            if not m:
                                return
                            if m == "<think>":
                                return decorate("message", {"content": "", "start_to_think": True})

                            elif m == "</think>":
                                return decorate("message", {"content": "", "end_to_think": True})

                            buff_m += m
                            _m += m

                            if len(buff_m) > 16:
                                ev = decorate("message", {"content": m, "audio_binary": self.tts(tts_mdl, buff_m)})
                                buff_m = ""
                                return ev

                            return decorate("message", {"content": m})

                        if inspect.isasyncgen(stream):
                            async for m in stream:
                                ev = await _process_stream(m)
                                if ev:
                                    yield ev
                        else:
                            for m in stream:
                                ev = await _process_stream(m)
                                if ev:
                                    yield ev
                        if buff_m:
                            yield decorate("message", {"content": "", "audio_binary": self.tts(tts_mdl, buff_m)})
                            buff_m = ""
                        cpn_obj.set_output("content", _m)
                    else:
                        yield decorate("message", {"content": cpn_obj.output("content")})

                    message_end = self._build_message_end(cpn_obj)
                    yield decorate("message_end", message_end)

                    while partials:
                        _cpn_obj = self.get_component_obj(partials[0])
                        if isinstance(_cpn_obj.output("content"), partial):
                            break
                        yield _node_finished(_cpn_obj)
                        partials.pop(0)

                other_branch = False
                if cpn_obj.error():
                    ex = cpn_obj.exception_handler()
                    if ex and ex["goto"]:
                        self.path.extend(ex["goto"])
                        other_branch = True
                    elif ex and ex["default_value"]:
                        yield decorate("message", {"content": ex["default_value"]})
                        yield decorate("message_end", {})
                    else:
                        self.error = cpn_obj.error()

                if cpn_obj.component_name.lower() not in ("iteration", "loop"):
                    if isinstance(cpn_obj.output("content"), partial):
                        if self.error:
                            cpn_obj.set_output("content", None)
                            yield _node_finished(cpn_obj)
                        else:
                            partials.append(self.path[i])
                    else:
                        yield _node_finished(cpn_obj)

                def _append_path(cpn_id):
                    nonlocal other_branch
                    if other_branch:
                        return
                    if self.path[-1] == cpn_id:
                        return
                    self.path.append(cpn_id)

                def _extend_path(cpn_ids):
                    nonlocal other_branch
                    if other_branch:
                        return
                    for cpn_id in cpn_ids:
                        _append_path(cpn_id)

                if cpn_obj.component_name.lower() in ("iterationitem", "loopitem") and cpn_obj.end():
                    iter = cpn_obj.get_parent()
                    yield _node_finished(iter)
                    _extend_path(self.get_component(cpn["parent_id"])["downstream"])
                elif cpn_obj.component_name.lower() in ["categorize", "switch"]:
                    _extend_path(cpn_obj.output("_next"))
                elif cpn_obj.component_name.lower() in ("iteration", "loop"):
                    _append_path(cpn_obj.get_start())
                elif cpn_obj.component_name.lower() == "exitloop" and cpn_obj.get_parent().component_name.lower() == "loop":
                    _extend_path(self.get_component(cpn["parent_id"])["downstream"])
                elif not cpn["downstream"] and cpn_obj.get_parent():
                    _append_path(cpn_obj.get_parent().get_start())
                else:
                    _extend_path(cpn["downstream"])

            if self.error:
                logging.error(f"Runtime Error: {self.error}")
                break
            idx = to

            if any([self.components.get(c) is not None and self.get_component_obj(c).component_name.lower() == "userfillup" for c in self.path[idx:]]):
                path = [c for c in self.path[idx:] if self.components.get(c) is not None and self.get_component(c)["obj"].component_name.lower() == "userfillup"]
                path.extend([c for c in self.path[idx:] if self.components.get(c) is not None and self.get_component(c)["obj"].component_name.lower() != "userfillup"])
                another_inputs = {}
                tips = ""
                for c in path:
                    o = self.get_component_obj(c)
                    if o.component_name.lower() == "userfillup":
                        o.invoke()
                        another_inputs.update({k: v for k, v in o.get_input_elements().items() if not self._is_input_field_satisfied(v)})
                        if o.get_param("enable_tips"):
                            tips = o.output("tips")
                if not another_inputs:
                    continue
                self.path = path
                yield decorate("user_inputs", {"inputs": another_inputs, "tips": tips})
                return
        self.path = self.path[:idx]
        if not self.error:
            yield decorate(
                "workflow_finished",
                {
                    "inputs": kwargs.get("inputs"),
                    "outputs": self.get_component_obj(self.path[-1]).output(),
                    "elapsed_time": time.perf_counter() - st,
                    "created_at": st,
                    # Run-level total of all LLM calls — emitted once here.
                    "usage": self._run_usage_payload(),
                },
            )
            self.history.append(("assistant", self.get_component_obj(self.path[-1]).output()))
            self.globals["sys.history"].append(f"{self.history[-1][0]}: {self.history[-1][1]}")
        elif "Task has been canceled" in self.error:
            yield decorate(
                "workflow_finished",
                {
                    "inputs": kwargs.get("inputs"),
                    "outputs": "Task has been canceled",
                    "elapsed_time": time.perf_counter() - st,
                    "created_at": st,
                    "usage": self._run_usage_payload(),
                },
            )

    def is_reff(self, exp: str) -> bool:
        exp = exp.strip("{").strip("}")
        if exp.find("@") < 0:
            return exp in self.globals
        arr = exp.split("@")
        if len(arr) != 2:
            return False
        if self.get_component(arr[0]) is None:
            return False
        return True

    def tts(self, tts_mdl, text):
        def clean_tts_text(text: str) -> str:
            if not text:
                return ""

            text = text.encode("utf-8", "ignore").decode("utf-8", "ignore")

            text = re.sub(r"[\x00-\x08\x0B-\x0C\x0E-\x1F\x7F]", "", text)

            emoji_pattern = re.compile(
                "[\U0001f600-\U0001f64f\U0001f300-\U0001f5ff\U0001f680-\U0001f6ff\U0001f1e0-\U0001f1ff\U00002700-\U000027bf\U0001f900-\U0001f9ff\U0001fa70-\U0001faff\U0001fad0-\U0001faff]+",
                flags=re.UNICODE,
            )
            text = emoji_pattern.sub("", text)

            text = re.sub(r"\s+", " ", text).strip()

            MAX_LEN = 500
            if len(text) > MAX_LEN:
                text = text[:MAX_LEN]

            return text

        if not tts_mdl or not text:
            return None
        text = clean_tts_text(text)
        if not text:
            return None
        return synthesize_with_cache(tts_mdl, text)

    def get_history(self, window_size):
        convs = []
        if window_size <= 0:
            return convs
        for role, obj in self.history[window_size * -2 :]:
            if isinstance(obj, dict):
                convs.append({"role": role, "content": obj.get("content", "")})
            else:
                convs.append({"role": role, "content": str(obj)})
        return convs

    def add_user_input(self, question):
        self.history.append(("user", question))
        rendered = json.dumps(question, ensure_ascii=False) if isinstance(question, dict) else question
        self.globals["sys.history"].append(f"{self.history[-1][0]}: {rendered}")

    @staticmethod
    def _is_input_field_satisfied(field: Any) -> bool:
        if not isinstance(field, dict):
            return field is not None

        value = field.get("value")
        field_type = str(field.get("type", "")).lower()
        if field_type.find("file") >= 0:
            if field.get("optional") and value is None:
                return True
            return value not in (None, [], "")

        if value is None:
            return False

        return True

    def get_prologue(self):
        return self.components["begin"]["obj"]._param.prologue

    def get_mode(self):
        return self.components["begin"]["obj"]._param.mode

    def get_sys_query(self):
        return self.globals.get("sys.query", "")

    def set_global_param(self, **kwargs):
        self.globals.update(kwargs)

    def get_preset_param(self):
        return self.components["begin"]["obj"]._param.inputs

    def get_component_input_elements(self, cpnnm):
        return self.components[cpnnm]["obj"].get_input_elements()

    async def get_files_async(self, files: Union[None, list[dict]], layout_recognize: str = None) -> list[str]:
        if not files:
            return []

        def image_to_base64(file):
            return "data:{};base64,{}".format(file["mime_type"], base64.b64encode(FileService.get_blob(file["created_by"], file["id"])).decode("utf-8"))

        def parse_file(file):
            blob = FileService.get_blob(file["created_by"], file["id"])
            return FileService.parse(file["name"], blob, True, file["created_by"], layout_recognize)

        loop = asyncio.get_running_loop()
        tasks = []
        for file in files:
            if file["mime_type"].find("image") >= 0:
                tasks.append(loop.run_in_executor(self._thread_pool, image_to_base64, file))
                continue
            tasks.append(loop.run_in_executor(self._thread_pool, parse_file, file))
        return await asyncio.gather(*tasks)

    def get_files(self, files: Union[None, list[dict]], layout_recognize: str = None) -> list[str]:
        """
        Synchronous wrapper for get_files_async, used by sync component invoke paths.
        """
        loop = getattr(self, "_loop", None)
        if loop and loop.is_running():
            return asyncio.run_coroutine_threadsafe(self.get_files_async(files, layout_recognize), loop).result()

        return asyncio.run(self.get_files_async(files, layout_recognize))

    def tool_use_callback(self, agent_id: str, func_name: str, params: dict, result: Any, elapsed_time=None):
        agent_ids = agent_id.split("-->")
        agent_name = self.get_component_name(agent_ids[0])
        path = agent_name if len(agent_ids) < 2 else agent_name + "-->" + "-->".join(agent_ids[1:])
        try:
            bin = REDIS_CONN.get(f"{self.task_id}-{self.message_id}-logs")
            if bin:
                obj = json.loads(bin.encode("utf-8"))
                if obj[-1]["component_id"] == agent_ids[0]:
                    obj[-1]["trace"].append({"path": path, "tool_name": func_name, "arguments": params, "result": result, "elapsed_time": elapsed_time})
                else:
                    obj.append({"component_id": agent_ids[0], "trace": [{"path": path, "tool_name": func_name, "arguments": params, "result": result, "elapsed_time": elapsed_time}]})
            else:
                obj = [{"component_id": agent_ids[0], "trace": [{"path": path, "tool_name": func_name, "arguments": params, "result": result, "elapsed_time": elapsed_time}]}]
            REDIS_CONN.set_obj(f"{self.task_id}-{self.message_id}-logs", obj, 60 * 10)
        except Exception as e:
            logging.exception(e)

    def add_reference(self, chunks: list[object], doc_infos: list[object]):
        if not self.retrieval:
            self.retrieval = [{"chunks": {}, "doc_aggs": {}}]

        r = self.retrieval[-1]
        for ck in chunks_format({"chunks": chunks}):
            cid = hash_str2int(ck["id"], 500)
            # cid = uuid.uuid5(uuid.NAMESPACE_DNS, ck["id"])
            if cid not in r:
                r["chunks"][cid] = ck

        for doc in doc_infos:
            if doc["doc_name"] not in r:
                r["doc_aggs"][doc["doc_name"]] = doc

    def get_reference(self):
        if not self.retrieval:
            return {"chunks": {}, "doc_aggs": {}}
        return self.retrieval[-1]

    def _has_reference(self) -> bool:
        ref = self.get_reference()
        if not isinstance(ref, dict):
            return False
        return bool(ref.get("chunks") or ref.get("doc_aggs"))

    def _build_message_end(self, cpn_obj) -> dict:
        message_end = {}
        if cpn_obj.get_param("status"):
            message_end["status"] = cpn_obj.get_param("status")
        if isinstance(cpn_obj.output("attachment"), dict):
            message_end["attachment"] = cpn_obj.output("attachment")
        if self._has_reference():
            message_end["reference"] = self.get_reference()
        # NOTE: aggregated run token usage is intentionally NOT attached here.
        # _build_message_end runs once per Message component, so a multi-Message graph
        # would emit cumulative usage repeatedly and double count. The run total is
        # emitted exactly once on the terminal workflow_finished event instead.
        return message_end

    def _run_usage_payload(self) -> dict:
        usage = getattr(self, "_run_token_usage", None) or {}
        return {
            "prompt_tokens": usage.get("prompt_tokens", 0),
            "completion_tokens": usage.get("completion_tokens", 0),
            "total_tokens": usage.get("total_tokens", 0),
            "calls": usage.get("calls", 0),
        }

    def add_memory(self, user: str, assist: str, summ: str):
        self.memory.append((user, assist, summ))

    def get_memory(self) -> list[Tuple]:
        return self.memory

    def get_component_thoughts(self, cpn_id) -> str:
        return self.components.get(cpn_id)["obj"].thoughts()
