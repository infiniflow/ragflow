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

from typing import Any, Optional, TYPE_CHECKING

if TYPE_CHECKING:
    from requests import Response
    from requests.sessions import _Files, _Params
    from ..ragflow import RAGFlow

class Base:
    __slots__ = 'rag',

    rag: "RAGFlow"

    def __init__(self, rag: "RAGFlow", res_dict: dict[str, Any]) -> None:
        self.rag = rag
        self._update_from_dict(rag, res_dict)

    def _update_from_dict(self, rag: "RAGFlow", res_dict: dict[str, Any]) -> None:
        for k, v in res_dict.items():
            if isinstance(v, dict):
                setattr(self, k, Base(rag, v))
            else:
                setattr(self, k, v)

    def to_json(self) -> dict[str, Any]:
        pr: dict[str, Any] = {}
        for name in dir(self):
            value = getattr(self, name)
            if not name.startswith("__") and not callable(value) and name != "rag":
                if isinstance(value, Base):
                    pr[name] = value.to_json()
                else:
                    pr[name] = value
        return pr

    def post(self, path: str, json: Any=None, stream: bool=False, files: Optional["_Files"]=None) -> "Response":
        res = self.rag.post(path, json, stream=stream, files=files)
        return res

    def get(self, path: str, params: Optional["_Params"]=None) -> "Response":
        res = self.rag.get(path, params)
        return res

    def rm(self, path: str, json: Any) -> "Response":
        res = self.rag.delete(path, json)
        return res

    def put(self, path: str, json: Any) -> "Response":
        res = self.rag.put(path, json)
        return res

    def __str__(self) -> str:
        return str(self.to_json())
