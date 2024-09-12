from typing import List

from .base import Base
from .session import Session


class Assistant(Base):
    def __init__(self, rag, res_dict):
        self.id = ""
        self.name = "assistant"
        self.avatar = "path/to/avatar"
        self.knowledgebases = ["kb1"]
        self.llm = Assistant.LLM(rag, {})
        self.prompt = Assistant.Prompt(rag, {})
        super().__init__(rag, res_dict)

    class LLM(Base):
        def __init__(self, rag, res_dict):
            self.model_name = "deepseek-chat"
            self.temperature = 0.1
            self.top_p = 0.3
            self.presence_penalty = 0.4
            self.frequency_penalty = 0.7
            self.max_tokens = 512
            super().__init__(rag, res_dict)

    class Prompt(Base):
        def __init__(self, rag, res_dict):
            self.similarity_threshold = 0.2
            self.keywords_similarity_weight = 0.7
            self.top_n = 8
            self.variables = [{"key": "knowledge", "optional": True}]
            self.rerank_model = None
            self.empty_response = None
            self.opener = "Hi! I'm your assistant, what can I do for you?"
            self.show_quote = True
            self.prompt = (
                "You are an intelligent assistant. Please summarize the content of the knowledge base to answer the question. "
                "Please list the data in the knowledge base and answer in detail. When all knowledge base content is irrelevant to the question, "
                "your answer must include the sentence 'The answer you are looking for is not found in the knowledge base!' "
                "Answers need to consider chat history.\nHere is the knowledge base:\n{knowledge}\nThe above is the knowledge base."
            )
            super().__init__(rag, res_dict)

    def save(self) -> bool:
        res = self.post('/assistant/save',
                        {"id": self.id, "name": self.name, "avatar": self.avatar, "knowledgebases": self.knowledgebases,
                         "llm": self.llm.to_json(), "prompt": self.prompt.to_json()
                         })
        res = res.json()
        if res.get("retmsg") == "success": return True
        raise Exception(res["retmsg"])

    def delete(self) -> bool:
        res = self.rm('/assistant/delete',
                      {"id": self.id})
        res = res.json()
        if res.get("retmsg") == "success": return True
        raise Exception(res["retmsg"])

    def create_session(self, name: str = "New session") -> Session:
        res = self.post("/session/save", {"name": name, "assistant_id": self.id})
        res = res.json()
        if res.get("retmsg") == "success":
            return Session(self.rag, res['data'])
        raise Exception(res["retmsg"])

    def list_session(self) -> List[Session]:
        res = self.get('/session/list', {"assistant_id": self.id})
        res = res.json()
        if res.get("retmsg") == "success":
            result_list = []
            for data in res["data"]:
                result_list.append(Session(self.rag, data))
            return result_list
        raise Exception(res["retmsg"])

    def get_session(self, id) -> Session:
        res = self.get("/session/get", {"id": id})
        res = res.json()
        if res.get("retmsg") == "success":
            return Session(self.rag, res["data"])
        raise Exception(res["retmsg"])

    def get_prologue(self):
        return self.prompt.opener
