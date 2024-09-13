from .base import Base


class Chunk(Base):
    def __init__(self, rag, res_dict):
        # 初始化类的属性
        self.id = ""
        self.content_with_weight = ""
        self.content_ltks = []
        self.content_sm_ltks = []
        self.important_kwd = []
        self.important_tks = []
        self.create_time = ""
        self.create_timestamp_flt = 0.0
        self.kb_id = None
        self.docnm_kwd = ""
        self.doc_id = ""
        self.q_vec = []
        self.status = "1"
        for k, v in res_dict.items():
            if hasattr(self, k):
                setattr(self, k, v)

        super().__init__(rag, res_dict)
    def delete(self) -> bool:
        """
        Delete the chunk in the document.
        """
        res = self.rm('/doc/chunk/rm',
                      {"doc_id": [self.id],""})
        res = res.json()
        if res.get("retmsg") == "success":
            return True
        raise Exception(res["retmsg"])