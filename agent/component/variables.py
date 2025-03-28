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
import logging
import re
from abc import ABC
from api.db import LLMType
from api.db.services.llm_service import LLMBundle
from agent.component import GenerateParam, Generate
import json


class VariablesExtractParam(GenerateParam):
    """
    Define the VariablesExtract component parameters.
    """

    def __init__(self):
        super().__init__()
        self.temperature = 0.9
        self.prompt = ""


    def check(self):
        super().check()



    def get_prompt(self, conv, params):
        prompt = f"""
You are a data expert extracting information. DON'T generate anything except the information extracted by the template. 
######################################
Example
######################################
# Example 1
REQUEST: Get 'UserName', 'Address' from the conversation.
## Conversation
    -ASSISTANT: What is your name?
    -USER: My name is Jennifer, I live in Washington.

## Output template: 
```json
{{
    "UserName":"Jennifer",
    "Address":"Washington"
}}
```
###########
# Example 2
REQUEST: Get 'UserCode', 'Department' from the conversation.
## Conversation
    -USER: My employee code is 39211.
    -ASSISTANT: What department are you in?
    -USER: I am in HR department.
## Output template: 
```json
{{
    "UserCode":"39211",
    "Department":"HR"
}}
```
###########
# Example 3
REQUEST: Get 'Topic', 'The languages of the latest question' from the conversation.
## Conversation
    -USER: Xin chào.
    -ASSISTANT: Chào bạn, Ngày mới tốt lành!
    -USER: How are you?.
## Output template: 
```json
{{
    "Topic":"Greetings",
    "The languages of the latest question":"English"
}}
```
###########
# Example 4
REQUEST: Get 'The languages of the latest question', 'Topic' from the conversation.
## Conversation
    -USER: 今天是个美好的一天.
    -ASSISTANT: 是的，如果你今天走在街上，那就太好了
    -USER: 你知道哪里有意义吗？.
## Output template: 
```json
{{
    "Topic":"外出",
    "The languages of the latest question":"Chinese"
}}
```
###################
# Real Data
REQUEST: Get '{", ".join(params.keys())}' from the conversation.

## Conversation
    {conv}
######################################
"""
        return prompt


class VariablesExtract(Generate, ABC):
    component_name = "VariablesExtract"

    def _run(self, history, **kwargs):
        args = {}
        logging.info(f"VariablesExtract: _run: {self._param.variables}")
        for para in self._param.variables:
            if para.get("key"):
                if 'begin@' in para["key"]:
                    field = para["key"].split('@')[1]
                    field = field.strip()
                    if field:
                        args[field]=""

        logging.info(f"VariablesExtract: _run: args: {args}")
        query = self.get_input()
        query = str(query["content"][0]) if "content" in query else ""
        
     
        
    
        hist = self._canvas.get_history(self._param.message_history_window_size)
        conv = []
        for m in hist:
            if m["role"] not in ["user"]:
                continue
            conv.append("{}: {}".format(m["role"].upper(), m["content"]))
        conv = "\n".join(conv)
        chat_mdl = LLMBundle(self._canvas.get_tenant_id(), LLMType.CHAT, self._param.llm_id)
        ans = chat_mdl.chat(self._param.get_prompt(conv, args),
                            [{"role": "user", "content": "Output template:"}], self._param.gen_conf())
        ans = re.sub(r"\s+", " ", ans)
        match = re.search(r"```json\s*(.*?)\s*```", ans, re.DOTALL)
        if match:
            ans = match.group(1)
         
            logging.info(ans)
        if not ans:
            logging.info(ans)
            return VariablesExtract.be_output(query)

        
        logging.info(f"ans: {ans}")
       
        try:
            kwargs ={}
            ans_json = json.loads(ans)
            for v in ans_json:
                if  ans_json[v] != "Unknown" and  ans_json[v] != "" and ans_json[v] != "None":
                    kwargs[v] = "{}".format(ans_json[v]) 

            self._canvas.set_global_param(**kwargs)
            logging.info("Begin: query: {}".format( self._canvas.components["begin"]["obj"]._param.query)) 

            return VariablesExtract.be_output(query)
        except json.JSONDecodeError:
            logging.warning(f"VariablesExtract: LLM returned non-JSON output: {ans}")
            return VariablesExtract.be_output(query)

    def debug(self, **kwargs):
        return self._run([], **kwargs)