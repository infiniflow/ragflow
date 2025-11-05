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
#
import importlib


def from_dict_hook(in_dict: dict):
    if "type" in in_dict and "data" in in_dict:
        if in_dict["module"] is None:
            return in_dict["data"]
        else:
            return getattr(importlib.import_module(
                in_dict["module"]), in_dict["type"])(**in_dict["data"])
    else:
        return in_dict
