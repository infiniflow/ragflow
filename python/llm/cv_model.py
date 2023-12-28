from abc import ABC
from openai import OpenAI
import os
import base64
from io import BytesIO


class Base(ABC):
    def describe(self, image, max_tokens=300):
        raise NotImplementedError("Please implement encode method!")

    def image2base64(self, image):
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
    def __init__(self):
        self.client = OpenAI(api_key=os.environ["OPENAI_API_KEY"])

    def describe(self, image, max_tokens=300):
        b64 = self.image2base64(image)

        res = self.client.chat.completions.create(
            model="gpt-4-vision-preview",
            messages=self.prompt(b64),
            max_tokens=max_tokens,
        )
        return res.choices[0].message.content.strip()


class QWenCV(Base):
    def describe(self, image, max_tokens=300):
        from http import HTTPStatus
        from dashscope import MultiModalConversation
        # export DASHSCOPE_API_KEY=YOUR_DASHSCOPE_API_KEY
        response = MultiModalConversation.call(model=MultiModalConversation.Models.qwen_vl_chat_v1,
                                               messages=self.prompt(self.image2base64(image)))
        if response.status_code == HTTPStatus.OK:
            return response.output.choices[0]['message']['content']
        return response.message
