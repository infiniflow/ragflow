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
from api.db.services.document_service import DocumentService
from rag.flow.base import ProcessBase, ProcessParamBase


class FileParam(ProcessParamBase):
    def __init__(self):
        super().__init__()

    def check(self):
        pass

    def get_input_form(self) -> dict[str, dict]:
        return {}


class File(ProcessBase):
    component_name = "File"

    async def _invoke(self, **kwargs):
        if self._canvas._doc_id:
            e, doc = DocumentService.get_by_id(self._canvas._doc_id)
            if not e:
                self.set_output("_ERROR", f"Document({self._canvas._doc_id}) not found!")
                return

            #b, n = File2DocumentService.get_storage_address(doc_id=self._canvas._doc_id)
            #self.set_output("blob", STORAGE_IMPL.get(b, n))
            self.set_output("name", doc.name)
        else:
            file = kwargs.get("file")[0]
            self.set_output("name", file["name"])
            self.set_output("file", file)
            #self.set_output("blob", FileService.get_blob(file["created_by"], file["id"]))

        self.callback(1, "File fetched.")
