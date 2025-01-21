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

from .base import Base
from .session import Session
import requests


class Agent(Base):
    def __init__(self,rag,res_dict):
        self.id  = None
        self.avatar = None
        self.canvas_type = None
        self.description = None
        self.dsl = None
        super().__init__(rag, res_dict)

    class Dsl(Base):
        def __init__(self,rag,res_dict):
            self.answer = []
            self.components = {
                "begin": {
                    "downstream": ["Answer:China"],
                    "obj": {
                        "component_name": "Begin",
                        "params": {}
                    },
                    "upstream": []
                }
            }
            self.graph = {
                "edges": [],
                "nodes": [
                    {
                        "data": {
                            "label": "Begin",
                            "name": "begin"
                        },
                        "id": "begin",
                        "position": {
                            "x": 50,
                            "y": 200
                        },
                        "sourcePosition": "left",
                        "targetPosition": "right",
                        "type": "beginNode"
                    }
                ]
            }
            self.history =  []
            self.messages =  []
            self.path =  []
            self.reference = []
            super().__init__(rag,res_dict)

    @staticmethod
    def create_session(id,rag,**kwargs) -> Session:
        res = requests.post(f"{rag.api_url}/agents/{id}/sessions",headers={"Authorization": f"Bearer {rag.user_key}"},json=kwargs)
        res = res.json()
        if res.get("code") == 0:
            return Session(rag,res.get("data"))
        raise Exception(res.get("message"))

    @staticmethod
    def list_sessions(agent_id,rag,page: int = 1, page_size: int = 30, orderby: str = "create_time", desc: bool = True,
                      id: str = None) -> list[Session]:
        url = f"{rag.api_url}/agents/{agent_id}/sessions"
        headers = {"Authorization": f"Bearer {rag.user_key}"}
        params = {"page": page, "page_size": page_size, "orderby": orderby, "desc": desc, "id": id}
        res = requests.get(url=url,headers=headers,params=params)
        res = res.json()
        if res.get("code") == 0:
            result_list = []
            for data in res.get("data"):
                temp_agent = Session(rag,data)
                result_list.append(temp_agent)
            return result_list
        raise Exception(res.get("message"))
