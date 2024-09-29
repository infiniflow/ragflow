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
import dotenv
import typing


def get_versions() -> typing.Mapping[str, typing.Any]:
    dotenv.load_dotenv(dotenv.find_dotenv())
    return dotenv.dotenv_values()


def get_rag_version() -> typing.Optional[str]:
    return get_versions().get("RAGFLOW_IMAGE", "infiniflow/ragflow:dev").split(":")[-1]