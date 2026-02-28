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


from quart import Response
from quart_schema import tag
from api.apps import login_required
from api.utils.api_utils import get_json_result
from agent.plugin import GlobalPluginManager


def set_operation_doc(summary: str, description: str = ""):
    def decorator(func):
        func.__doc__ = summary if not description else f"{summary}\n\n{description}"
        return func

    return decorator


@manager.route('/llm_tools', methods=['GET'])  # noqa: F821
@set_operation_doc("List available LLM tools provided by installed plugins.")
@tag(["Plugins"])
@login_required
def llm_tools() -> Response:
    tools = GlobalPluginManager.get_llm_tools()
    tools_metadata = [t.get_metadata() for t in tools]

    return get_json_result(data=tools_metadata)
