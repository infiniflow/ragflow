import os
from .embedding_model import *
from .chat_model import *
from .cv_model import *

EmbeddingModel = None
ChatModel = None
CvModel = None


if os.environ.get("OPENAI_API_KEY"):
    EmbeddingModel = GptEmbed()
    ChatModel = GptTurbo()
    CvModel = GptV4()

elif os.environ.get("DASHSCOPE_API_KEY"):
    EmbeddingModel = QWenEmbd()
    ChatModel = QWenChat()
    CvModel = QWenCV()
else:
    EmbeddingModel = HuEmbedding()
