from .base import Base
from .session import Session,Message
import requests
import json


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
    def create_session(id,rag) -> Session:
        res = requests.post(f"{rag.api_url}/agents/{id}/sessions",headers={"Authorization": f"Bearer {rag.user_key}"},json={})
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

    @staticmethod
    def ask(agent_id,rag,stream=True,**kwargs):
        url = f"{rag.api_url}/agents/{agent_id}/completions"
        headers = {"Authorization": f"Bearer {rag.user_key}"}
        res = requests.post(url=url, headers=headers, json=kwargs,stream=stream)
        for line in res.iter_lines():
            line = line.decode("utf-8")
            if line.startswith("{"):
                json_data = json.loads(line)
                raise Exception(json_data["message"])
            if line.startswith("data:"):
                json_data = json.loads(line[5:])
                if json_data["data"] is not True:
                    if json_data["data"].get("running_status"):
                        continue
                    answer = json_data["data"]["answer"]
                    reference = json_data["data"]["reference"]
                    temp_dict = {
                        "content": answer,
                        "role": "assistant"
                    }
                    if "chunks" in reference:
                        chunks = reference["chunks"]
                        temp_dict["reference"] = chunks
                    message = Message(rag, temp_dict)
                    yield message
