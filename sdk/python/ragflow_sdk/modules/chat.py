
from .base import Base
from .session import Session


class Chat(Base):
    def __init__(self, rag, res_dict):
        self.id = ""
        self.name = "assistant"
        self.avatar = "path/to/avatar"
        self.dataset_ids = ["kb1"]
        self.llm = Chat.LLM(rag, {})
        self.prompt = Chat.Prompt(rag, {})
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
            self.top_k = 1024
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

    def update(self, update_message: dict):
        res = self.put(f'/chats/{self.id}',
                        update_message)
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res["message"])


    def create_session(self, name: str = "New session") -> Session:
        res = self.post(f"/chats/{self.id}/sessions", {"name": name})
        res = res.json()
        if res.get("code") == 0:
            return Session(self.rag, res['data'])
        raise Exception(res["message"])

    def list_sessions(self,page: int = 1, page_size: int = 30, orderby: str = "create_time", desc: bool = True,
                      id: str = None, name: str = None) -> list[Session]:
        res = self.get(f'/chats/{self.id}/sessions',{"page": page, "page_size": page_size, "orderby": orderby, "desc": desc, "id": id, "name": name} )
        res = res.json()
        if res.get("code") == 0:
            result_list = []
            for data in res["data"]:
                result_list.append(Session(self.rag, data))
            return result_list
        raise Exception(res["message"])

    def delete_sessions(self,ids: list[str] | None = None):
        res = self.rm(f"/chats/{self.id}/sessions", {"ids": ids})
        res = res.json()
        if res.get("code") != 0:
            raise Exception(res.get("message"))
