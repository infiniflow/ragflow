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
from mcp import ClientSession
from mcp.client.streamable_http import streamablehttp_client


async def main():
    try:
        async with streamablehttp_client("http://localhost:9382/mcp/") as (read_stream, write_stream, _):
            async with ClientSession(read_stream, write_stream) as session:
                await session.initialize()
                tools = await session.list_tools()
                print(f"{tools.tools=}")
                response = await session.call_tool(name="ragflow_retrieval", arguments={"dataset_ids": ["bc4177924a7a11f09eff238aa5c10c94"], "document_ids": [], "question": "How to install neovim?"})
                print(f"Tool response: {response.model_dump()}")
    except Exception as e:
        print(e)


if __name__ == "__main__":
    from anyio import run

    run(main)
