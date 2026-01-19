#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
#

from .base import Base

class ChunkUpdateError(Exception):
    def __init__(self, code=None, message=None, details=None):
        self.code = code
        self.message = message
        self.details = details
        super().__init__(message)

class Chunk(Base):
    def __init__(self, rag, res_dict):
        self.id = ""
        self.content = ""
        self.important_keywords = []
        self.questions = []
        self.create_time = ""
        self.create_timestamp = 0.0
        self.dataset_id = None
        self.document_name = ""
        self.documnet_keyword = ""
        self.document_id = ""
        self.available = True
        # Additional fields for retrieval results
        self.similarity = 0.0
        self.vector_similarity = 0.0
        self.term_similarity = 0.0
        self.positions = []
        self.doc_type = ""
        for k in list(res_dict.keys()):
            if k not in self.__dict__:
                res_dict.pop(k)
        super().__init__(rag, res_dict)

        #for backward compatibility
        if not self.document_name:
            self.document_name = self.documnet_keyword


    def update(self, update_message: dict):
        res = self.put(f"/datasets/{self.dataset_id}/documents/{self.document_id}/chunks/{self.id}", update_message)
        res = res.json()
        if res.get("code") != 0:
            raise ChunkUpdateError(
                code=res.get("code"),
                message=res.get("message"),
                details=res.get("details")
            )