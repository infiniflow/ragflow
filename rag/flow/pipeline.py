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
import base64
import datetime
import json
import logging
import time
from concurrent.futures import ThreadPoolExecutor
from copy import deepcopy
from functools import partial
from typing import Any, Union, Tuple

from agent.canvas import Graph
from agent.component import component_class
from agent.component.base import ComponentBase
from api.db.services.file_service import FileService
from api.utils import get_uuid, hash_str2int
from rag.prompts.prompts import chunks_format
from rag.utils.redis_conn import REDIS_CONN


class Pipeline(Graph):

    def __init__(self, dsl: str, tenant_id=None, kb_id=None, doc_id=None, task_id=None, flow_id=None):
        super().__init__(dsl, tenant_id, task_id)
        self._kb_id = kb_id
        self._doc_id = doc_id
        self._flow_id = flow_id

    def callback(self, component_name: str, progress: float, message: str) -> None:
        log_key = f"{self._flow_id}-{self.task_id}-logs"
        try:
            bin = REDIS_CONN.get(log_key)
            if bin:
                obj = json.loads(bin.encode("utf-8"))
                if obj[-1]["component_name"] == component_name:
                    obj[-1]["trace"].append({"progress": progress, "message": message, "datetime": datetime.datetime.now().strftime("%H:%M:%S")})
                else:
                    obj.append({
                    "component_name": component_name,
                    "trace": [{"progress": progress, "message": message, "datetime": datetime.datetime.now().strftime("%H:%M:%S")}]
                })
            else:
                obj = [{
                    "component_name": component_name,
                    "trace": [{"progress": progress, "message": message, "datetime": datetime.datetime.now().strftime("%H:%M:%S")}]
                }]
            REDIS_CONN.set_obj(log_key, obj, 60*10)
        except Exception as e:
            logging.exception(e)

    def run(self, **kwargs):
        st = time.perf_counter()
        if not self.path:
            self.path.append("begin")

        self.error = ""
        idx = len(self.path) - 1
        if idx == 0:
            cpn = self.get_component_obj(self.path[0])
            cpn.invoke(**kwargs)
            idx += 1

        while idx < len(self.path):
            last_cpn = self.get_component_obj(self.path[idx-1])
            cpn_obj = self.get_component_obj(self.path[idx])
            cpn.invoke(**last_cpn.output())
            if cpn_obj.error():
                self.error = cpn_obj.error()
                break
            idx += 1

    def get_component(self, cpn_id) -> Union[None, dict[str, Any]]:
        return self.components.get(cpn_id)

    def get_component_obj(self, cpn_id) -> ComponentBase:
        return self.components.get(cpn_id)["obj"]

    def get_component_type(self, cpn_id) -> str:
        return self.components.get(cpn_id)["obj"].component_name

    def get_component_input_form(self, cpn_id) -> dict:
        return self.components.get(cpn_id)["obj"].get_input_form()

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

    def get_variable_value(self, exp: str) -> Any:
        exp = exp.strip("{").strip("}").strip(" ").strip("{").strip("}")
        if exp.find("@") < 0:
            return self.globals[exp]
        cpn_id, var_nm = exp.split("@")
        cpn = self.get_component(cpn_id)
        if not cpn:
            raise Exception(f"Can't find variable: '{cpn_id}@{var_nm}'")
        return cpn["obj"].output(var_nm)

    def get_tenant_id(self):
        return self._tenant_id

    def get_history(self, window_size):
        convs = []
        if window_size <= 0:
            return convs
        for role, obj in self.history[window_size * -1:]:
            if isinstance(obj, dict):
                convs.append({"role": role, "content": obj.get("content", "")})
            else:
                convs.append({"role": role, "content": str(obj)})
        return convs

    def add_user_input(self, question):
        self.history.append(("user", question))

    def _find_loop(self, max_loops=6):
        path = self.path[-1][::-1]
        if len(path) < 2:
            return False

        for i in range(len(path)):
            if path[i].lower().find("answer") == 0 or path[i].lower().find("iterationitem") == 0:
                path = path[:i]
                break

        if len(path) < 2:
            return False

        for loc in range(2, len(path) // 2):
            pat = ",".join(path[0:loc])
            path_str = ",".join(path)
            if len(pat) >= len(path_str):
                return False
            loop = max_loops
            while path_str.find(pat) == 0 and loop >= 0:
                loop -= 1
                if len(pat)+1 >= len(path_str):
                    return False
                path_str = path_str[len(pat)+1:]
            if loop < 0:
                pat = " => ".join([p.split(":")[0] for p in path[0:loc]])
                return pat + " => " + pat

        return False

    def get_prologue(self):
        return self.components["begin"]["obj"]._param.prologue

    def set_global_param(self, **kwargs):
        self.globals.update(kwargs)

    def get_preset_param(self):
        return self.components["begin"]["obj"]._param.inputs

    def get_component_input_elements(self, cpnnm):
        return self.components[cpnnm]["obj"].get_input_elements()

    def get_files(self, files: Union[None, list[dict]]) -> list[str]:
        if not files:
            return  []
        def image_to_base64(file):
            return "data:{};base64,{}".format(file["mime_type"],
                                        base64.b64encode(FileService.get_blob(file["created_by"], file["id"])).decode("utf-8"))
        exe = ThreadPoolExecutor(max_workers=5)
        threads = []
        for file in files:
            if file["mime_type"].find("image") >=0:
                threads.append(exe.submit(image_to_base64, file))
                continue
            threads.append(exe.submit(FileService.parse, file["name"], FileService.get_blob(file["created_by"], file["id"]), True, file["created_by"]))
        return [th.result() for th in threads]

    def tool_use_callback(self, agent_id: str, func_name: str, params: dict, result: Any, elapsed_time=None):
        agent_ids = agent_id.split("-->")
        agent_name = self.get_component_name(agent_ids[0])
        path = agent_name if len(agent_ids) < 2 else agent_name+"-->"+"-->".join(agent_ids[1:])
        try:
            bin = REDIS_CONN.get(f"{self.task_id}-{self.message_id}-logs")
            if bin:
                obj = json.loads(bin.encode("utf-8"))
                if obj[-1]["component_id"] == agent_ids[0]:
                    obj[-1]["trace"].append({"path": path, "tool_name": func_name, "arguments": params, "result": result, "elapsed_time": elapsed_time})
                else:
                    obj.append({
                    "component_id": agent_ids[0],
                    "trace": [{"path": path, "tool_name": func_name, "arguments": params, "result": result, "elapsed_time": elapsed_time}]
                })
            else:
                obj = [{
                    "component_id": agent_ids[0],
                    "trace": [{"path": path, "tool_name": func_name, "arguments": params, "result": result, "elapsed_time": elapsed_time}]
                }]
            REDIS_CONN.set_obj(f"{self.task_id}-{self.message_id}-logs", obj, 60*10)
        except Exception as e:
            logging.exception(e)

    def add_refernce(self, chunks: list[object], doc_infos: list[object]):
        if not self.retrieval:
            self.retrieval = [{"chunks": {}, "doc_aggs": {}}]

        r = self.retrieval[-1]
        for ck in chunks_format({"chunks": chunks}):
            cid = hash_str2int(ck["id"], 100)
            if cid not in r:
                r["chunks"][cid] = ck

        for doc in doc_infos:
            if doc["doc_name"] not in r:
                r["doc_aggs"][doc["doc_name"]] = doc

    def get_reference(self):
        if not self.retrieval:
            return {"chunks": {}, "doc_aggs": {}}
        return self.retrieval[-1]

    def add_memory(self, user:str, assist:str, summ: str):
        self.memory.append((user, assist, summ))

    def get_memory(self) -> list[Tuple]:
        return self.memory

    def get_component_thoughts(self, cpn_id) -> str:
        return self.components.get(cpn_id)["obj"].thoughts()

