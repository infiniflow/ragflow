from abc import ABC
import openapi
import os

class Base(ABC):
    def chat(self, system, history, gen_conf):
        raise NotImplementedError("Please implement encode method!")


class GptTurbo(Base):
    def __init__(self):
        openapi.api_key = os.environ["OPENAPI_KEY"]

    def chat(self, system, history, gen_conf):
        history.insert(0, {"role": "system", "content": system})
        res = openapi.ChatCompletion.create(model="gpt-3.5-turbo",
                                           messages=history,
                                          **gen_conf)
        return res.choices[0].message.content.strip()


class QWen(Base):
    def chat(self, system, history, gen_conf):
        from http import HTTPStatus
        from dashscope import Generation
        from dashscope.api_entities.dashscope_response import Role
        response = Generation.call(
                    Generation.Models.qwen_turbo,
                    messages=messages,
                    result_format='message'
        )
        if response.status_code == HTTPStatus.OK:
            return response.output.choices[0]['message']['content']
        return response.message
