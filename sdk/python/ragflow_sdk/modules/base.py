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


class Base:
    def __init__(self, rag, res_dict):
        self.rag = rag
        self._update_from_dict(rag, res_dict)

    def _update_from_dict(self, rag, res_dict):
        for k, v in res_dict.items():
            if isinstance(v, dict):
                self.__dict__[k] = Base(rag, v)
            else:
                self.__dict__[k] = v

    def to_json(self):
        pr = {}
        for name in dir(self):
            value = getattr(self, name)
            if not name.startswith("__") and not callable(value) and name != "rag":
                if isinstance(value, Base):
                    pr[name] = value.to_json()
                else:
                    pr[name] = value
        return pr

    def post(self, path, json=None, stream=False, files=None):
        res = self.rag.post(path, json, stream=stream, files=files)
        return res

    def get(self, path, params=None):
        res = self.rag.get(path, params)
        return res

    def rm(self, path, json):
        res = self.rag.delete(path, json)
        return res

    def put(self, path, json):
        res = self.rag.put(path, json)
        return res

    def __str__(self):
        return str(self.to_json())
