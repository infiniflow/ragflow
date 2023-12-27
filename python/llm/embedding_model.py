from abc import ABC
from FlagEmbedding import FlagModel
import torch
import numpy as np

class Base(ABC):
    def encode(self, texts: list, batch_size=32):
        raise NotImplementedError("Please implement encode method!")


class HuEmbedding(Base):
    def __init__(self):
        """
        If you have trouble downloading HuggingFace models, -_^ this might help!!

        For Linux:
        export HF_ENDPOINT=https://hf-mirror.com

        For Windows:
        Good luck
        ^_-

        """
        self.model = FlagModel("BAAI/bge-large-zh-v1.5",
                              query_instruction_for_retrieval="为这个句子生成表示以用于检索相关文章：",
                              use_fp16=torch.cuda.is_available())

    def encode(self, texts: list, batch_size=32):
        res = []
        for i in range(0, len(texts), batch_size):
            res.extend(self.model.encode(texts[i:i+batch_size]).tolist())
        return np.array(res)



class GptEmbed(Base):
    def __init__(self):
        import openapi,os
        from openai import OpenAI
        openapi.api_key = os.environ["OPENAPI_KEY"]
        self.client = OpenAI()

    def encode(self, texts: list, batch_size=32):
        res = self.client.embeddings.create(input = texts,
                                            model="text-embedding-ada-002")
        return [d["embedding"] for d in res["data"]]


class QWen(base):
    def encode(self, texts: list, batch_size=32, text_type="document"):
        import dashscope
        from http import HTTPStatus
        res = []
        for txt in texts:
            resp = dashscope.TextEmbedding.call(
                model=dashscope.TextEmbedding.Models.text_embedding_v2,
                input=txt[:2048],
                text_type=text_type
            )
            res.append(resp["output"]["embeddings"][0]["embedding"])
        return res
