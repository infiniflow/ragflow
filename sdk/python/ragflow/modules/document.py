from time import sleep, time

import requests

from api.settings import RetCode
from .base import Base
from datetime import datetime
from time import sleep
from typing import Tuple, Iterator
import requests


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
        # 拼接API请求的URL，使用文档ID和数据集ID
        res=self.get(f"/doc/{self.kb_id}/documents/{self.id}",{"headers":self.rag.authorization_header,"id": self.id,"name": self.name,"stream":True})
        # api_url = f"{self.rag.api_url}/{self.kb_id}/documents/{self.id}"
        #
        # # 发送GET请求以下载文档
        # response = requests.get(api_url, headers=self.rag.authorization_header, stream=True)

        # 检查响应状态码并确保请求成功
        if res.status_code == 200:
            # 将文档内容以字节形式返回
            return res.content
        else:
            # 处理错误并抛出异常
            raise Exception(
                f"Failed to download document. Server responded with: {res.status_code}, {res.text}")

    def async_parse(self) -> None:
        """
        Start asynchronous document parsing by sending a request to the server.
        """
        # API路径：启动文档解析
        url = f"/{self.kb_id}/documents/{self.id}/status"

        # 发送POST请求以启动文档解析
        res = self.rag.post(url)

        # 检查返回结果
        res_data = res.json()
        if res_data.get("code") != RetCode.SUCCESS:
            raise Exception(
                f"Failed to start parsing document '{self.id}'. Server responded with: {res_data['message']}")

        print(f"Document '{self.id}' parsing started successfully.")

    def join(self, interval=15, timeout=3600) -> iter:
        """
        Wait for the document parsing process to complete, checking the status at regular intervals.

        :param interval: Time interval in seconds to check the parsing status.
        :param timeout: Maximum time in seconds to wait for the parsing to complete.
        :return: An iterator that yields the progress percentage and message at each check.
        """
        # API路径：获取文档解析状态
        url = f"/{self.kb_id}/documents/{self.id}/status"

        start_time = time()

        while time() - start_time < timeout:
            # 发送GET请求获取文档的解析状态
            res = self.rag.get(url)
            res_data = res.json()

            if res_data["code"] != RetCode.SUCCESS:
                raise Exception(f"Failed to retrieve document status: {res_data['message']}")

            progress = res_data["data"]["progress"]
            status = res_data["data"]["status"]

            # 返回当前进度和状态消息
            yield progress, f"Status: {status}, Progress: {progress}%"

            # 如果解析已完成，则退出循环
            if status != "RUNNING":
                break

            # 等待下一个状态检查
            sleep(interval)

        if time() - start_time >= timeout:
            raise TimeoutError(f"Timeout reached while waiting for document '{self.id}' parsing to complete.")

    def parse(self, interval=15, timeout=3600) -> iter:
        """
        Start document parsing and wait for it to complete.

        :param interval: Time interval in seconds to check the parsing status.
        :param timeout: Maximum time in seconds to wait for the parsing to complete.
        :return: An iterator that yields the progress percentage and message at each check.
        """
        # 启动异步解析
        self.async_parse()

        # 等待解析完成
        yield from self.join(interval=interval, timeout=timeout)

    def cancel(self) -> None:
        """
        Cancel the parsing of the document.
        """
        # API路径：取消文档解析
        url = f"/{self.kb_id}/documents/{self.id}/status"

        # 发送DELETE请求以取消文档解析
        res = self.rag.delete(url)

        res_data = res.json()
        if res_data.get("code") != RetCode.SUCCESS:
            raise Exception(
                f"Failed to cancel parsing document '{self.id}'. Server responded with: {res_data['message']}")

        print(f"Document '{self.id}' parsing cancelled successfully.")
