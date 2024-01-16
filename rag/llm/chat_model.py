#
#  Copyright 2019 The RAG Flow Authors. All Rights Reserved.
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
from abc import ABC
from openai import OpenAI
import os


class Base(ABC):
    def chat(self, system, history, gen_conf):
        raise NotImplementedError("Please implement encode method!")


class GptTurbo(Base):
    def __init__(self):
        self.client = OpenAI(api_key=os.environ["OPENAI_API_KEY"])

    def chat(self, system, history, gen_conf):
        history.insert(0, {"role": "system", "content": system})
        res = self.client.chat.completions.create(
            model="gpt-3.5-turbo",
            messages=history,
            **gen_conf)
        return res.choices[0].message.content.strip()


class QWenChat(Base):
    def chat(self, system, history, gen_conf):
        from http import HTTPStatus
        from dashscope import Generation
        # export DASHSCOPE_API_KEY=YOUR_DASHSCOPE_API_KEY
        history.insert(0, {"role": "system", "content": system})
        response = Generation.call(
            Generation.Models.qwen_turbo,
            messages=history,
            result_format='message'
        )
        if response.status_code == HTTPStatus.OK:
            return response.output.choices[0]['message']['content']
        return response.message
