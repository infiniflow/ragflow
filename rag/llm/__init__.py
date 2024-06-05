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
from .embedding_model import *
from .chat_model import *
from .cv_model import *
from .rerank_model import *


EmbeddingModel = {
    "Ollama": OllamaEmbed,
    "OpenAI": OpenAIEmbed,
    "Xinference": XinferenceEmbed,
    "Tongyi-Qianwen": QWenEmbed,
    "ZHIPU-AI": ZhipuEmbed,
    "FastEmbed": FastEmbed,
    "Youdao": YoudaoEmbed,
    "BaiChuan": BaiChuanEmbed,
    "Jina": JinaEmbed,
    "BAAI": DefaultEmbedding
}


CvModel = {
    "OpenAI": GptV4,
    "Ollama": OllamaCV,
    "Xinference": XinferenceCV,
    "Tongyi-Qianwen": QWenCV,
    "ZHIPU-AI": Zhipu4V,
    "Moonshot": LocalCV
}


ChatModel = {
    "OpenAI": GptTurbo,
    "ZHIPU-AI": ZhipuChat,
    "Tongyi-Qianwen": QWenChat,
    "Ollama": OllamaChat,
    "Xinference": XinferenceChat,
    "Moonshot": MoonshotChat,
    "DeepSeek": DeepSeekChat,
    "BaiChuan": BaiChuanChat,
    "MiniMax": MiniMaxChat
}


RerankModel = {
    "BAAI": DefaultRerank,
    "Jina": JinaRerank,
    "Youdao": YoudaoRerank,
}
