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
from abc import ABC
from openai import OpenAI
import os
import base64
from io import BytesIO


class Base(ABC):
    def __init__(self, key, model_name):
        pass

    def describe(self, image, max_tokens=300):
        raise NotImplementedError("Please implement encode method!")

    def image2base64(self, image):
        if isinstance(image, bytes):
            return base64.b64encode(image).decode("utf-8")
        if isinstance(image, BytesIO):
            return base64.b64encode(image.getvalue()).decode("utf-8")
        buffered = BytesIO()
        try:
            image.save(buffered, format="JPEG")
        except Exception as e:
            image.save(buffered, format="PNG")
        return base64.b64encode(buffered.getvalue()).decode("utf-8")

    def prompt(self, b64):
        return [
            {
                "role": "user",
                "content": [
                    {
                        "type": "text",
                        "text": "请用中文详细描述一下图中的内容，比如时间，地点，人物，事情，人物心情等。",
                    },
                    {
                        "type": "image_url",
                        "image_url": {
                            "url": f"data:image/jpeg;base64,{b64}"
                        },
                    },
                ],
            }
        ]


class GptV4(Base):
    def __init__(self, key, model_name="gpt-4-vision-preview"):
        self.client = OpenAI(api_key=key)
        self.model_name = model_name

    def describe(self, image, max_tokens=300):
        b64 = self.image2base64(image)

        res = self.client.chat.completions.create(
            model=self.model_name,
            messages=self.prompt(b64),
            max_tokens=max_tokens,
        )
        return res.choices[0].message.content.strip(), res.usage.total_tokens


class QWenCV(Base):
    def __init__(self, key, model_name="qwen-vl-chat-v1"):
        import dashscope
        dashscope.api_key = key
        self.model_name = model_name

    def describe(self, image, max_tokens=300):
        from http import HTTPStatus
        from dashscope import MultiModalConversation
        response = MultiModalConversation.call(model=self.model_name,
                                               messages=self.prompt(self.image2base64(image)))
        if response.status_code == HTTPStatus.OK:
            return response.output.choices[0]['message']['content'], response.usage.output_tokens
        return response.message, 0


from zhipuai import ZhipuAI


class Zhipu4V(Base):
    def __init__(self, key, model_name="glm-4v"):
        self.client = ZhipuAI(api_key=key)
        self.model_name = model_name

    def describe(self, image, max_tokens=1024):
        b64 = self.image2base64(image)

        res = self.client.chat.completions.create(
            model=self.model_name,
            messages=self.prompt(b64),
            max_tokens=max_tokens,
        )
        return res.choices[0].message.content.strip(), res.usage.total_tokens
