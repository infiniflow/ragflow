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
import logging
import json
from copy import deepcopy
from functools import partial

import pandas as pd

from agent.component import component_class
from agent.component.base import ComponentBase


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
            "answer_0": {
                "obj": {
                    "component_name": "Answer",
                    "params": {}
                },
                "downstream": ["retrieval_0"],
                "upstream": ["begin", "generate_0"],
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
        "messages": [],
        "reference": [],
        "path": [["begin"]],
        "answer": []
    }
    """

    def __init__(self, dsl: str, tenant_id=None):
        self.path = []
        self.history = []
        self.messages = []
        self.answer = []
        self.components = {}
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
            "messages": [],
            "reference": [],
            "path": [],
            "answer": []
        }
        self._tenant_id = tenant_id
        self._embed_id = ""
        self.load()

    def load(self):
        self.components = self.dsl["components"]
        cpn_nms = set([])
        for k, cpn in self.components.items():
            cpn_nms.add(cpn["obj"]["component_name"])

        assert "Begin" in cpn_nms, "There have to be an 'Begin' component."
        assert "Answer" in cpn_nms, "There have to be an 'Answer' component."

        for k, cpn in self.components.items():
            cpn_nms.add(cpn["obj"]["component_name"])
            param = component_class(cpn["obj"]["component_name"] + "Param")()
            param.update(cpn["obj"]["params"])
            param.check()
            cpn["obj"] = component_class(cpn["obj"]["component_name"])(self, k, param)
            if cpn["obj"].component_name == "Categorize":
                for _, desc in param.category_description.items():
                    if desc["to"] not in cpn["downstream"]:
                        cpn["downstream"].append(desc["to"])

        self.path = self.dsl["path"]
        self.history = self.dsl["history"]
        self.messages = self.dsl["messages"]
        self.answer = self.dsl["answer"]
        self.reference = self.dsl["reference"]
        self._embed_id = self.dsl.get("embed_id", "")

    def __str__(self):
        self.dsl["path"] = self.path
        self.dsl["history"] = self.history
        self.dsl["messages"] = self.messages
        self.dsl["answer"] = self.answer
        self.dsl["reference"] = self.reference
        self.dsl["embed_id"] = self._embed_id
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

    def reset(self):
        self.path = []
        self.history = []
        self.messages = []
        self.answer = []
        self.reference = []
        for k, cpn in self.components.items():
            self.components[k]["obj"].reset()
        self._embed_id = ""

    def get_component_name(self, cid):
        for n in self.dsl["graph"]["nodes"]:
            if cid == n["id"]:
                return n["data"]["name"]
        return ""

    def run(self, **kwargs):
        if self.answer:
            cpn_id = self.answer[0]
            self.answer.pop(0)
            try:
                ans = self.components[cpn_id]["obj"].run(self.history, **kwargs)
            except Exception as e:
                ans = ComponentBase.be_output(str(e))
            self.path[-1].append(cpn_id)
            if kwargs.get("stream"):
                for an in ans():
                    yield an
            else:
                yield ans
            return

        if not self.path:
            self.components["begin"]["obj"].run(self.history, **kwargs)
            self.path.append(["begin"])

        self.path.append([])

        ran = -1
        waiting = []
        without_dependent_checking = []

        def prepare2run(cpns):
            nonlocal ran, ans
            for c in cpns:
                if self.path[-1] and c == self.path[-1][-1]:
                    continue
                cpn = self.components[c]["obj"]
                if cpn.component_name == "Answer":
                    self.answer.append(c)
                else:
                    logging.debug(f"Canvas.prepare2run: {c}")
                    if c not in without_dependent_checking:
                        cpids = cpn.get_dependent_components()
                        if any([cc not in self.path[-1] for cc in cpids]):
                            if c not in waiting:
                                waiting.append(c)
                            continue
                    yield "*'{}'* is running...🕞".format(self.get_component_name(c))

                    if cpn.component_name.lower() == "iteration":
                        st_cpn = cpn.get_start()
                        assert st_cpn, "Start component not found for Iteration."
                        if not st_cpn["obj"].end():
                            cpn = st_cpn["obj"]
                            c = cpn._id

                    try:
                        ans = cpn.run(self.history, **kwargs)
                    except Exception as e:
                        logging.exception(f"Canvas.run got exception: {e}")
                        self.path[-1].append(c)
                        ran += 1
                        raise e
                    self.path[-1].append(c)

            ran += 1

        downstream = self.components[self.path[-2][-1]]["downstream"]
        if not downstream and self.components[self.path[-2][-1]].get("parent_id"):
            cid = self.path[-2][-1]
            pid = self.components[cid]["parent_id"]
            o, _ = self.components[cid]["obj"].output(allow_partial=False)
            oo, _ = self.components[pid]["obj"].output(allow_partial=False)
            self.components[pid]["obj"].set(pd.concat([oo, o], ignore_index=True))
            downstream = [pid]

        for m in prepare2run(downstream):
            yield {"content": m, "running_status": True}

        while 0 <= ran < len(self.path[-1]):
            logging.debug(f"Canvas.run: {ran} {self.path}")
            cpn_id = self.path[-1][ran]
            cpn = self.get_component(cpn_id)
            if not any([cpn["downstream"], cpn.get("parent_id"), waiting]):
                break

            loop = self._find_loop()
            if loop:
                raise OverflowError(f"Too much loops: {loop}")

            if cpn["obj"].component_name.lower() in ["switch", "categorize", "relevant"]:
                switch_out = cpn["obj"].output()[1].iloc[0, 0]
                assert switch_out in self.components, \
                    "{}'s output: {} not valid.".format(cpn_id, switch_out)
                for m in prepare2run([switch_out]):
                    yield {"content": m, "running_status": True}
                continue

            downstream = cpn["downstream"]
            if not downstream and cpn.get("parent_id"):
                pid = cpn["parent_id"]
                _, o = cpn["obj"].output(allow_partial=False)
                _, oo = self.components[pid]["obj"].output(allow_partial=False)
                self.components[pid]["obj"].set_output(pd.concat([oo.dropna(axis=1), o.dropna(axis=1)], ignore_index=True))
                downstream = [pid]

            for m in prepare2run(downstream):
                yield {"content": m, "running_status": True}

            if ran >= len(self.path[-1]) and waiting:
                without_dependent_checking = waiting
                waiting = []
                for m in prepare2run(without_dependent_checking):
                    yield {"content": m, "running_status": True}
                without_dependent_checking = []
                ran -= 1

        if self.answer:
            cpn_id = self.answer[0]
            self.answer.pop(0)
            ans = self.components[cpn_id]["obj"].run(self.history, **kwargs)
            self.path[-1].append(cpn_id)
            if kwargs.get("stream"):
                assert isinstance(ans, partial)
                for an in ans():
                    yield an
            else:
                yield ans

        else:
            raise Exception("The dialog flow has no way to interact with you. Please add an 'Interact' component to the end of the flow.")

    def get_component(self, cpn_id):
        return self.components[cpn_id]

    def get_tenant_id(self):
        return self._tenant_id

    def get_history(self, window_size):
        convs = []
        for role, obj in self.history[window_size * -1:]:
            if isinstance(obj, list) and obj and all([isinstance(o, dict) for o in obj]):
                convs.append({"role": role, "content": '\n'.join([str(s.get("content", "")) for s in obj])})
            else:
                convs.append({"role": role, "content": str(obj)})
        return convs

    def add_user_input(self, question):
        self.history.append(("user", question))

    def set_embedding_model(self, embed_id):
        self._embed_id = embed_id

    def get_embedding_model(self):
        return self._embed_id

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
        for k, v in kwargs.items():
            for q in self.components["begin"]["obj"]._param.query:
                if k != q["key"]:
                    continue
                q["value"] = v

    def get_preset_param(self):
        return self.components["begin"]["obj"]._param.query

    def get_component_input_elements(self, cpnnm):
        return self.components[cpnnm]["obj"].get_input_elements()