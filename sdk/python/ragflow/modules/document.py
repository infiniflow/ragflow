
from .base import Base



class Document(Base):
    def __init__(self, rag, res_dict):
        self.id = ""
        self.name = ""
        self.thumbnail = None
        self.kb_id = None
        self.parser_method = ""
        self.parser_config = {"pages": [[1, 1000000]]}
        self.source_type = "local"
        self.type = ""
        self.created_by = ""
        self.size = 0
        self.token_num = 0
        self.chunk_num = 0
        self.progress = 0.0
        self.progress_msg = ""
        self.process_begin_at = None
        self.process_duration = 0.0
        for k in list(res_dict.keys()):
            if k not in self.__dict__:
                res_dict.pop(k)
        super().__init__(rag, res_dict)

    def save(self) -> bool:
        """
        Save the document details to the server.
        """
        res = self.post('/doc/save',
                        {"id": self.id, "name": self.name, "thumbnail": self.thumbnail, "kb_id": self.kb_id,
                         "parser_id": self.parser_method, "parser_config": self.parser_config.to_json(),
                         "source_type": self.source_type, "type": self.type, "created_by": self.created_by,
                         "size": self.size, "token_num": self.token_num, "chunk_num": self.chunk_num,
                         "progress": self.progress, "progress_msg": self.progress_msg,
                         "process_begin_at": self.process_begin_at, "process_duation": self.process_duration
                         })
        res = res.json()
        if res.get("retmsg") == "success":
            return True
        raise Exception(res["retmsg"])

    def delete(self) -> bool:
        """
        Delete the document from the server.
        """
        res = self.rm('/doc/delete',
                      {"doc_id": self.id})
        res = res.json()
        if res.get("retmsg") == "success":
            return True
        raise Exception(res["retmsg"])

    def download(self) -> bytes:
        """
        Download the document content from the server using the Flask API.

        :return: The downloaded document content in bytes.
        """
        # Construct the URL for the API request using the document ID and knowledge base ID
        res = self.get(f"/doc/{self.kb_id}/documents/{self.id}",
                       {"headers": self.rag.authorization_header, "id": self.id, "name": self.name, "stream": True})

        # Check the response status code to ensure the request was successful
        if res.status_code == 200:
            # Return the document content as bytes
            return res.content
        else:
            # Handle the error and raise an exception
            raise Exception(
                f"Failed to download document. Server responded with: {res.status_code}, {res.text}"
            )
