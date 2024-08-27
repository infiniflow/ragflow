#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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

import requests

from .modules.dataset import DataSet


class RAGFlow:
    def __init__(self, user_key, base_url, version='v1'):
        """
        api_url: http://<host_address>/api/v1
        """
        self.user_key = user_key
        self.api_url = f"{base_url}/api/{version}"
        self.authorization_header = {"Authorization": "{} {}".format("Bearer",self.user_key)}

    def post(self, path, param):
        res = requests.post(url=self.api_url + path, json=param, headers=self.authorization_header)
        return res

    def get(self, path, params=''):
        res = requests.get(self.api_url + path, params=params, headers=self.authorization_header)
        return res

    def create_dataset(self, name:str,avatar:str="",description:str="",language:str="English",permission:str="me",
                       document_count:int=0,chunk_count:int=0,parser_method:str="naive",
                       parser_config:DataSet.ParserConfig=None):
        if parser_config is None:
            parser_config = DataSet.ParserConfig(self, {"chunk_token_count":128,"layout_recognize": True, "delimiter":"\n!?。；！？","task_page_size":12})
        parser_config=parser_config.to_json()
        res=self.post("/dataset/save",{"name":name,"avatar":avatar,"description":description,"language":language,"permission":permission,
                               "doc_num": document_count,"chunk_num":chunk_count,"parser_id":parser_method,
                               "parser_config":parser_config
                               }
                     )
        res = res.json()
        if not res.get("retmsg"):
            return DataSet(self, res["data"])
        raise Exception(res["retmsg"])


