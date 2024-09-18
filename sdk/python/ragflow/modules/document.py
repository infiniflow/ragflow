import time

from .base import Base
from .chunk import Chunk


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
        self.run = "0"
        self.status = "1"
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
        res = self.get(f"/doc/{self.id}",
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

    def async_parse(self):
        """
        Initiate document parsing asynchronously without waiting for completion.
        """
        try:
            # Construct request data including document ID and run status (assuming 1 means to run)
            data = {"doc_ids": [self.id], "run": 1}

            # Send a POST request to the specified parsing status endpoint to start parsing
            res = self.post(f'/doc/run', data)

            # Check the server response status code
            if res.status_code != 200:
                raise Exception(f"Failed to start async parsing: {res.text}")

            print("Async parsing started successfully.")

        except Exception as e:
            # Catch and handle exceptions
            print(f"Error occurred during async parsing: {str(e)}")
            raise

    import time

    def join(self, interval=5, timeout=3600):
        """
        Wait for the asynchronous parsing to complete and yield parsing progress periodically.

        :param interval: The time interval (in seconds) for progress reports.
        :param timeout: The timeout (in seconds) for the parsing operation.
        :return: An iterator yielding parsing progress and messages.
        """
        start_time = time.time()
        while time.time() - start_time < timeout:
            # Check the parsing status
            res = self.get(f'/doc/{self.id}/status', {"doc_ids": [self.id]})
            res_data = res.json()
            data = res_data.get("data", [])

            # Retrieve progress and status message
            progress = data.get("progress", 0)
            progress_msg = data.get("status", "")

            yield progress, progress_msg  # Yield progress and message

            if progress == 100:  # Parsing completed
                break

            time.sleep(interval)

    def cancel(self):
        """
        Cancel the parsing task for the document.
        """
        try:
            # Construct request data, including document ID and action to cancel (assuming 2 means cancel)
            data = {"doc_ids": [self.id], "run": 2}

            # Send a POST request to the specified parsing status endpoint to cancel parsing
            res = self.post(f'/doc/run', data)

            # Check the server response status code
            if res.status_code != 200:
                print("Failed to cancel parsing. Server response:", res.text)
            else:
                print("Parsing cancelled successfully.")

        except Exception as e:
            print(f"Error occurred during async parsing cancellation: {str(e)}")
            raise

    def list_chunks(self, page=1, offset=0, limit=12,size=30, keywords="", available_int=None):
        """
        List all chunks associated with this document by calling the external API.

        Args:
            page (int): The page number to retrieve (default 1).
            size (int): The number of chunks per page (default 30).
            keywords (str): Keywords for searching specific chunks (default "").
            available_int (int): Filter for available chunks (optional).

        Returns:
            list: A list of chunks returned from the API.
        """
        data = {
            "doc_id": self.id,
            "page": page,
            "size": size,
            "keywords": keywords,
            "offset":offset,
            "limit":limit
        }

        if available_int is not None:
            data["available_int"] = available_int

        res = self.post(f'/doc/chunk/list', data)
        if res.status_code == 200:
            res_data = res.json()
            if res_data.get("retmsg") == "success":
                chunks = res_data["data"]["chunks"]
                self.chunks = chunks  # Store the chunks in the document instance
                return chunks
            else:
                raise Exception(f"Error fetching chunks: {res_data.get('retmsg')}")
        else:
            raise Exception(f"API request failed with status code {res.status_code}")

    def add_chunk(self, content: str):
        res = self.post('/doc/chunk/create', {"doc_id": self.id, "content_with_weight":content})

        # 假设返回的 response 包含 chunk 的信息
        if res.status_code == 200:
            chunk_data = res.json()
            return Chunk(self.rag,chunk_data)  # 假设有一个 Chunk 类来处理 chunk 对象
        else:
            raise Exception(f"Failed to add chunk: {res.status_code} {res.text}")
