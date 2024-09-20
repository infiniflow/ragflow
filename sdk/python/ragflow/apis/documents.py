from typing import List

import requests

from .base_api import BaseApi


class Document(BaseApi):

    def __init__(self, user_key, api_url, authorization_header):
        """
        api_url: http://<host_address>/api/v1
        """
        self.user_key = user_key
        self.api_url = api_url
        self.authorization_header = authorization_header

    def upload(self, kb_id: str, file_paths: List[str]):
        """
        Upload documents file a Dataset(Knowledgebase).

        :param kb_id: The dataset ID.
        :param file_paths: One or more file paths.

        """
        files = []
        for file_path in file_paths:
            with open(file_path, 'rb') as file:
                file_data = file.read()
                files.append(('file', (file_path, file_data, 'application/octet-stream')))

        data = {'kb_id': kb_id}
        res = requests.post(self.api_url + "/documents/upload", data=data, files=files,
                            headers=self.authorization_header)
        res = res.json()
        if "retmsg" in res and res["retmsg"] == "success":
            return res
        raise Exception(res)

    def change_parser(self, doc_id: str, parser_id: str, parser_config: dict):
        """
        Change document file parsing method.

        :param doc_id: The document ID.
        :param parser_id: The parsing method.
        :param parser_config: The parsing method configuration.

        """
        res = super().post(
            "/documents/change_parser",
            {
                "doc_id": doc_id,
                "parser_id": parser_id,
                "parser_config": parser_config,
            }
        )
        res = res.json()
        if "retmsg" in res and res["retmsg"] == "success":
            return res
        raise Exception(res)

    def run_parsing(self, doc_ids: list):
        """
        Run parsing documents file.

        :param doc_ids: The set of Document IDs.

        """
        res = super().post("/documents/run",
                           {"doc_ids": doc_ids})
        res = res.json()
        if "retmsg" in res and res["retmsg"] == "success":
            return res
        raise Exception(res)
