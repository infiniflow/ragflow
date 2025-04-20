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
import asyncio
import uuid
from abc import ABC
from openai import AsyncOpenAI
from agent.component.base import ComponentBase, ComponentParamBase
from typing import List
import json
import logging
from mcp import ClientSession
from mcp.client.sse import sse_client

from api.db.services.llm_service import TenantLLMService


class MCPSSEClientParam(ComponentParamBase):
    """
    Define the Baidu component parameters.
    """

    def __init__(self):
        super().__init__()

    def check(self):
        return True


class MCPSSEClient(ComponentBase, ABC):
    component_name = "MCPSSEClient"

    def _run(self, history, **kwargs):
        ans = self.get_input()
        ans = "\n".join(ans["content"]) if "content" in ans else ""
        if not ans:
            return MCPSSEClient.be_output("")

        llm_id = self._param.llm_id
        if not llm_id:
            return MCPSSEClient.be_output("model error")
        mcp_servers = self._param.variables
        if not mcp_servers:
            return MCPSSEClient.be_output("mcp server is empty")

        mcp_servers = [mcp_server['value']  for mcp_server in mcp_servers if mcp_server['value']]
        params = {}
        params['frequency_penalty'] = self._param.frequency_penalty if self._param.frequency_penalty else 0
        params['presence_penalty'] = self._param.presence_penalty if self._param.presence_penalty else 0
        params['temperature'] = self._param.temperature if self._param.temperature else 0.5
        params['top_p'] = self._param.top_p if self._param.top_p else 3
        params['server_list'] = mcp_servers if mcp_servers else []
        split = llm_id.split("@")
        query = TenantLLMService.query(tenant_id=self._canvas.get_tenant_id(), llm_name=split[0], llm_factory=split[1])
        if not query:
            return MCPSSEClient.be_output("model error")
        params['model_name'] =query[0].llm_name
        params['base_url'] = query[0].api_base
        params['api_key'] = query[0].api_key
        params['max_tokens'] = query[0].max_tokens

        dialogue = self._parse_dialogue(ans)
        new_loop = asyncio.new_event_loop()
        asyncio.set_event_loop(new_loop)
        loop = asyncio.get_event_loop()
        task = asyncio.ensure_future(real_run(dialogue,params))
        loop.run_until_complete(asyncio.wait([task]))
        return MCPSSEClient.be_output(task.result())

    def _parse_dialogue(self,text):
        lines = text.split('\n')
        dialogue = []
        for line in lines:
            if ':' in line:
                role_part, content = line.split(':', 1)
                role_part = role_part.strip().lower()
                content = content.strip()
                if role_part == 'assistant':
                    dialogue.append({'role': 'assistant', 'content': content})
                elif role_part == 'user':
                    dialogue.append({'role': 'user', 'content': content})
            else:
                if dialogue:
                    line_content = line.strip()
                    if line_content:
                        dialogue[-1]['content'] += '\n' + line_content
        return dialogue


async def real_run( dialogue, params:dict):
    client = MCPClient(model_name=params['model_name'], base_url=params['base_url'], api_key=params['api_key'],
                       server_list=params['server_list'])
    content = ""
    try:
        await client.initialize_sessions()
        content  = await client.chat(dialogue,params)
    except Exception as e:
        logging.error(f"Error occurred during chat: {str(e)}")
        content = str(e)
    finally:
        await client.cleanup()
        return content

class MCPClient:
    def __init__(self, model_name: str, base_url: str, api_key: str, server_list: List[str]):

        self.model_name = model_name
        self.server_urls = server_list
        self.sessions = {}
        self.tool_mapping = {}

        self.client = AsyncOpenAI(base_url=base_url, api_key=api_key)

    async def initialize_sessions(self):

        for i, server_url in enumerate(self.server_urls):

            server_id = f"server_{ uuid.uuid1().hex}"
            streams_context = sse_client(url=server_url)
            streams = await streams_context.__aenter__()
            session_context = ClientSession(*streams)
            session = await session_context.__aenter__()
            await session.initialize()

            self.sessions[server_id] = (session, session_context, streams_context)

            response = await session.list_tools()
            for tool in response.tools:
                prefixed_name = f"{server_id}_{tool.name}"
                self.tool_mapping[prefixed_name] = (session, tool.name)

    async def cleanup(self):
        for server_id, (session, session_context, streams_context) in self.sessions.items():
            await session_context.__aexit__(None, None, None)
            await streams_context.__aexit__(None, None, None)

    async def process_query(self, query: List,params:dict) -> str:

        messages = query

        available_tools = []
        for server_id, (session, _, _) in self.sessions.items():
            response = await session.list_tools()
            for tool in response.tools:
                prefixed_name = f"{server_id}_{tool.name}"
                available_tools.append({
                    "type": "function",
                    "function": {
                        "name": prefixed_name,
                        "description": tool.description,
                        "parameters": tool.inputSchema,
                    },
                })

        response = await self.client.chat.completions.create(
            model=self.model_name,
            messages=messages,
            tools=available_tools,
            temperature=params['temperature'],
            top_p=params['top_p'],
            frequency_penalty=params['frequency_penalty'],
            presence_penalty=params['presence_penalty'],
            max_tokens=params['max_tokens'],
        )

        final_answer = None
        message = response.choices[0].message

        if  message.tool_calls:
            for tool_call in message.tool_calls:
                prefixed_name = tool_call.function.name
                if prefixed_name in self.tool_mapping:
                    session, original_tool_name = self.tool_mapping[prefixed_name]
                    tool_args = json.loads(tool_call.function.arguments)
                    try:
                        result = await session.call_tool(original_tool_name, tool_args)
                    except Exception as e:
                        result = {"content": f"call tool {original_tool_name} exception：{str(e)}"}
                    messages.extend([
                        {
                            "role": "assistant",
                            "tool_calls": [{
                                "id": tool_call.id,
                                "type": "function",
                                "function": {"name": prefixed_name, "arguments": json.dumps(tool_args)},
                            }],
                        },
                        {"role": "tool", "tool_call_id": tool_call.id, "content": str(result.content)},
                    ])
                else:
                    logging.info(f"Tool {prefixed_name} not found")

            response = await self.client.chat.completions.create(
                model=self.model_name,
                messages=messages,
                tools=available_tools,
                temperature=params['temperature'],
                top_p=params['top_p'],
                frequency_penalty=params['frequency_penalty'],
                presence_penalty=params['presence_penalty'],
                max_tokens=params['max_tokens'],
            )
            message = response.choices[0].message
            final_answer = message.content
        else:
            final_answer = message.content
        return final_answer

    async def chat(self,messages:List,params:dict):
            try:
              return  await self.process_query(messages,params)
            except Exception as e:
                logging.error(f"error: {str(e)}")
                return "error"


