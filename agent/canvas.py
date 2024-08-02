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
import importlib
import json
import traceback
from abc import ABC
from copy import deepcopy
from functools import partial

import pandas as pd

from agent.component import component_class
from agent.component.base import ComponentBase
from agent.settings import flow_logger, DEBUG


class Canvas(ABC):
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
                    "upstream": []
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
            if k in ["components"]:continue
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

    def run(self, **kwargs):
        ans = ""
        if self.answer:
            cpn_id = self.answer[0]
            self.answer.pop(0)
            try:
                ans = self.components[cpn_id]["obj"].run(self.history, **kwargs)
            except Exception as e:
                ans = ComponentBase.be_output(str(e))
            self.path[-1].append(cpn_id)
            if kwargs.get("stream"):
                assert isinstance(ans, partial)
                return ans
            self.history.append(("assistant", ans.to_dict("records")))
            return ans

        if not self.path:
            self.components["begin"]["obj"].run(self.history, **kwargs)
            self.path.append(["begin"])

        self.path.append([])
        ran = -1

        def prepare2run(cpns):
            nonlocal ran, ans
            for c in cpns:
                if self.path[-1] and c == self.path[-1][-1]: continue
                cpn = self.components[c]["obj"]
                if cpn.component_name == "Answer":
                    self.answer.append(c)
                else:
                    if DEBUG: print("RUN: ", c)
                    if cpn.component_name == "Generate":
                        cpids = cpn.get_dependent_components()
                        if any([c not in self.path[-1] for c in cpids]):
                            continue
                    ans = cpn.run(self.history, **kwargs)
                    self.path[-1].append(c)
            ran += 1

        prepare2run(self.components[self.path[-2][-1]]["downstream"])
        while 0 <= ran < len(self.path[-1]):
            if DEBUG: print(ran, self.path)
            cpn_id = self.path[-1][ran]
            cpn = self.get_component(cpn_id)
            if not cpn["downstream"]: break

            loop = self._find_loop()
            if loop: raise OverflowError(f"Too much loops: {loop}")

            if cpn["obj"].component_name.lower() in ["switch", "categorize", "relevant"]:
                switch_out = cpn["obj"].output()[1].iloc[0, 0]
                assert switch_out in self.components, \
                    "{}'s output: {} not valid.".format(cpn_id, switch_out)
                try:
                    prepare2run([switch_out])
                except Exception as e:
                    for p in [c for p in self.path for c in p][::-1]:
                        if p.lower().find("answer") >= 0:
                            self.get_component(p)["obj"].set_exception(e)
                            prepare2run([p])
                            break
                    traceback.print_exc()
                    break
                continue

            try:
                prepare2run(cpn["downstream"])
            except Exception as e:
                for p in [c for p in self.path for c in p][::-1]:
                    if p.lower().find("answer") >= 0:
                        self.get_component(p)["obj"].set_exception(e)
                        prepare2run([p])
                        break
                traceback.print_exc()
                break

        if self.answer:
            cpn_id = self.answer[0]
            self.answer.pop(0)
            ans = self.components[cpn_id]["obj"].run(self.history, **kwargs)
            self.path[-1].append(cpn_id)
            if kwargs.get("stream"):
                assert isinstance(ans, partial)
                return ans

            self.history.append(("assistant", ans.to_dict("records")))

        return ans

    def get_component(self, cpn_id):
        return self.components[cpn_id]

    def get_tenant_id(self):
        return self._tenant_id

    def get_history(self, window_size):
        convs = []
        for role, obj in self.history[window_size * -2:]:
            convs.append({"role": role, "content": (obj if role == "user" else
                                                    '\n'.join(pd.DataFrame(obj)['content']))})
        return convs

    def add_user_input(self, question):
        self.history.append(("user", question))

    def set_embedding_model(self, embed_id):
        self._embed_id = embed_id

    def get_embedding_model(self):
        return self._embed_id

    def _find_loop(self, max_loops=2):
        path = self.path[-1][::-1]
        if len(path) < 2: return False

        for i in range(len(path)):
            if path[i].lower().find("answer") >= 0:
                path = path[:i]
                break

        if len(path) < 2: return False

        for l in range(2, len(path) // 2):
            pat = ",".join(path[0:l])
            path_str = ",".join(path)
            if len(pat) >= len(path_str): return False
            loop = max_loops
            while path_str.find(pat) == 0 and loop >= 0:
                loop -= 1
                if len(pat)+1 >= len(path_str):
                    return False
                path_str = path_str[len(pat)+1:]
            if loop < 0:
                pat = " => ".join([p.split(":")[0] for p in path[0:l]])
                return pat + " => " + pat

        return False
