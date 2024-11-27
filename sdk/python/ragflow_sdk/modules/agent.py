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
    def create_session(id,rag) -> Session:
        res = requests.post(f"{rag.api_url}/agents/{id}/sessions",headers={"Authorization": f"Bearer {rag.user_key}"},json={})
        res = res.json()
        if res.get("code") == 0:
            return Session(rag,res.get("data"))
        raise Exception(res.get("message"))

