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
import pathlib
import re
from .user_service import UserService


def duplicate_name(query_func, **kwargs):
    fnm = kwargs["name"]
    objs = query_func(**kwargs)
    if not objs: return fnm
    ext = pathlib.Path(fnm).suffix #.jpg
    nm = re.sub(r"%s$"%ext, "", fnm)
    r = re.search(r"\(([0-9]+)\)$", nm)
    c = 0
    if r:
        c = int(r.group(1))
        nm = re.sub(r"\([0-9]+\)$", "", nm)
    c += 1
    nm = f"{nm}({c})"
    if ext: nm += f"{ext}"

    kwargs["name"] = nm
    return duplicate_name(query_func, **kwargs)

