from abc import ABC
from FlagEmbedding import FlagModel
import torch

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
            res.extend(self.encode(texts[i:i+batch_size]))
        return res
