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

from typing import Any, NotRequired, Optional, TYPE_CHECKING, TypedDict
from .base import Base

if TYPE_CHECKING:
    from ..ragflow import RAGFlow

__all__ = 'Chunk',

class UpdateMessage(TypedDict):
    content: NotRequired[str]
    important_keywords: NotRequired[list[str]]
    available: NotRequired[bool]

class ChunkUpdateError(Exception):
    __slots__ = (
        'code',
        'message',
        'details',
    )

    code: Optional[int]
    message: Optional[str]
    details: Optional[str]

    def __init__(self, code: Optional[int]=None, message: Optional[str]=None, details: Optional[str]=None):
        self.code = code
        self.message = message
        self.details = details
        super().__init__(message)

class Chunk(Base):
    __slots__ = (
        'id',
        'content',
        'important_keywords',
        'questions',
        'create_time',
        'create_timestamp',
        'dataset_id',
        'document_name',
        'document_id',
        'available',
        'similarity',
        'vector_similarity',
        'term_similarity',
        'positions',
        'doc_type',
    )

    id: str
    content: str
    important_keywords: list[str]
    questions: list[str]
    create_time: str
    create_timestamp: float
    dataset_id: Optional[str]
    document_name: str
    document_id: str
    available: bool
    similarity: float
    vector_similarity: float
    term_similarity: float
    positions: list[str]
    doc_type: str

    def __init__(self, rag: "RAGFlow", res_dict: dict[str, Any]) -> None:
        self.id = ""
        self.content = ""
        self.important_keywords = []
        self.questions = []
        self.create_time = ""
        self.create_timestamp = 0.0
        self.dataset_id = None
        self.document_name = ""
        self.document_id = ""
        self.available = True
        # Additional fields for retrieval results
        self.similarity = 0.0
        self.vector_similarity = 0.0
        self.term_similarity = 0.0
        self.positions = []
        self.doc_type = ""
        for k in list(res_dict.keys()):
            if not hasattr(self, k):
                res_dict.pop(k)
        super().__init__(rag, res_dict)

    def update(self, update_message: UpdateMessage) -> None:
        res = self.put(f"/datasets/{self.dataset_id}/documents/{self.document_id}/chunks/{self.id}", update_message)
        res = res.json()
        if res.get("code") != 0:
            raise ChunkUpdateError(
                code=res.get("code"),
                message=res.get("message"),
                details=res.get("details")
            )
