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
import json
import time
from copy import deepcopy
from functools import partial
from typing import Any, Union

import trio
from agent.component import component_class
from api.utils import get_uuid


class Canvas:
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
        "retrival": {"chunks": [], "doc_aggs": []},
        "globals": {
            "sys.query": "",
            "sys.user_id": tenant_id,
            "sys.convsation_turns": 0,
            "sys.files": []
        }
    }
    """

    def __init__(self, dsl: str, tenant_id=None, task_id=None):
        self.path = []
        self.history = []
        self.components = {}
        self.globals = {
            "sys.query": "",
            "sys.user_id": tenant_id,
            "sys.convsation_turns": 0,
            "sys.files": []
        }
        self.dsl = json.loads(dsl) if dsl else {
            "components": {
                "begin": {
                    "obj": {
                        "component_name": "Begin",
                        "params": {
                            "prologue": "Hi there!"
                        }
                    },
                    "downstream": [],
                    "upstream": [],
                    "parent_id": ""
                }
            },
            "history": [],
            "path": [],
            "retrival": {"chunks": [], "doc_aggs": []},
            "globals": {
                "sys.query": "",
                "sys.user_id": "",
                "sys.convsation_turns": 0,
                "sys.files": []
            }
        }
        self._tenant_id = tenant_id
        self.task_id = task_id if task_id else get_uuid()
        self.load()

    def load(self):
        self.components = self.dsl["components"]
        cpn_nms = set([])
        for k, cpn in self.components.items():
            cpn_nms.add(cpn["obj"]["component_name"])

        assert "Begin" in cpn_nms, "There have to be an 'Begin' component."

        for k, cpn in self.components.items():
            cpn_nms.add(cpn["obj"]["component_name"])
            param = component_class(cpn["obj"]["component_name"] + "Param")()
            param.update(cpn["obj"]["params"])
            try:
                param.check()
            except Exception as e:
                raise ValueError(self.get_component_name(k) + f": {e}")
            cpn["obj"] = component_class(cpn["obj"]["component_name"])(self, k, param)
            if cpn["obj"].component_name == "Categorize":
                for _, desc in param.category_description.items():
                    if desc["to"] not in cpn["downstream"]:
                        cpn["downstream"].append(desc["to"])

        self.path = self.dsl["path"]
        self.history = self.dsl["history"]
        self.globals = self.dsl["globals"]
        self.retrival = self.dsl["retrival"]

    def __str__(self):
        self.dsl["path"] = self.path
        self.dsl["history"] = self.history
        self.dsl["globals"] = self.globals
        self.dsl["task_id"] = self.task_id
        self.dsl["retrival"] = self.retrival
        dsl = {
            "components": {}
        }
        for k in self.dsl.keys():
            if k in ["components"]:
                continue
            dsl[k] = deepcopy(self.dsl[k])

        for k, cpn in self.components.items():
            if k not in dsl["components"]:
                dsl["components"][k] = {}
            for c in cpn.keys():
                if c == "obj":
                    dsl["components"][k][c] = json.loads(str(cpn["obj"]))
                    continue
                dsl["components"][k][c] = deepcopy(cpn[c])
        return json.dumps(dsl, ensure_ascii=False)

    def reset(self, mem=False):
        self.path = []
        self.retrival = {"chunks": [], "doc_aggs": []}
        if not mem:
            self.history = []
        for k, cpn in self.components.items():
            self.components[k]["obj"].reset()

    def get_component_name(self, cid):
        for n in self.dsl.get("graph", {}).get("nodes", []):
            if cid == n["id"]:
                return n["data"]["name"]
        return ""

    def run(self, **kwargs):
        st = time.perf_counter()
        message_id = get_uuid()
        created_at = int(time.time())
        self.add_user_input(kwargs.get("query"))

        def decorate(event, dt):
            nonlocal message_id, created_at
            return {
                "event": event,
                #"conversation_id": "f3cc152b-24b0-4258-a1a1-7d5e9fc8a115",
                "message_id": message_id,
                "created_at": created_at,
                "task_id": self.task_id,
                "data": dt
            }

        if not self.path:
            self.path.append("begin")
            self.globals = {
                "sys.query": kwargs.get("query"),
                "sys.user_id": kwargs.get("user_id"),
                "sys.convsation_turns": 1,
                "sys.files": kwargs.get("files", [])
            }
            inputs = self.get_component_obj("begin").get_input()
            inputs.update(self.globals)
            yield decorate("workflow_started", {"inputs": inputs})

        async def _run_batch(f, t):
            async with trio.open_nursery() as nursery:
                for i in range(f, t):
                    cpn = self.get_component_obj(self.path[i])
                    for var, o in cpn.get_input_elements().items():
                        v = cpn.get_param(var)
                        if not v:
                            continue
                        if self.is_reff(v):
                            cpn.set_input_value(var, self.get_variable_value(v))
                        else:
                            cpn.set_input_value(var, v)
                    nursery.start_soon(lambda: cpn.invoke(**cpn.get_input()))

        error = ""
        idx = len(self.path) - 1
        st_idx = idx
        while idx < len(self.path):
            to = len(self.path)
            for i in range(idx, to):
                yield decorate("node_started", {"inputs": None, "created_at": int(time.time()), "component_id": self.path[i]})
            trio.run(_run_batch, idx, to)

            for i in range(idx, to):
                cpn = self.get_component(self.path[i])
                error = cpn["obj"].error() if not error else error
                if not error and cpn["obj"].component_name.lower() == "message":
                    if isinstance(cpn["obj"].output("content"), partial):
                        for m in cpn["obj"].output("content")():
                            yield decorate("message", {"content": m})
                    else:
                        yield decorate("message", {"content": cpn["obj"].output("content")})
                    yield decorate("message_end", {})

                yield decorate("node_finished",
                               {
                                   "inputs": cpn["obj"].get_input(),
                                   "outputs": cpn["obj"].output(),
                                   "component_id": self.path[i],
                                   "error": cpn["obj"].error(),
                                   "elapsed_time": cpn["obj"].output("_elapsed_time"),
                                   "created_at": int(time.time()),
                                })
                if cpn["obj"].component_name.lower() in ["categorize", "switch"]:
                    self.path.extend(cpn["obj"].output("_next"))
                else:
                    for c in cpn["downstream"]:
                        if c in self.path:
                            continue
                        self.path.append(c)
            if error:
                break
            idx = to

        self.path = self.path[:idx]
        if not error:
            yield decorate("workflow_finished",
                       {
                           "inputs": self.get_component_obj(self.path[st_idx]).get_input(),
                           "outputs": self.get_component_obj(self.path[-1]).output(),
                           "elapsed_time": time.perf_counter() - st,
                           "created_at": int(time.time()),
                       })
            self.history.append(("assistant", self.get_component_obj(self.path[-1]).output()))

    def get_component(self, cpn_id) -> Union[None, dict[str, Any]]:
        return self.components.get(cpn_id)

    def get_component_obj(self, cpn_id) -> object:
        return self.components.get(cpn_id)["obj"]

    def is_reff(self, exp):
        exp = exp.strip("{").strip("}")
        if exp.find("@") < 0:
            return exp in self.globals
        arr = exp.split("@")
        if len(arr) != 2:
            return False
        if self.get_component(arr[0]) is None:
            return False
        return True

    def get_variable_value(self, nm: str) -> Any:
        if nm.find("@") < 0:
            return self.globals[nm]
        cpn_id, var_nm = nm.split("@")
        cpn = self.get_component(cpn_id)
        if not cpn:
            raise Exception(f"Can't find variable: '{var_nm}'")
        return cpn["obj"].output(var_nm)

    def get_tenant_id(self):
        return self._tenant_id

    def get_history(self, window_size):
        convs = []
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

