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
from .sequence2txt_model import *

EmbeddingModel = {
    "Ollama": OllamaEmbed,
    "LocalAI": LocalAIEmbed,
    "OpenAI": OpenAIEmbed,
    "Azure-OpenAI": AzureEmbed,
    "Xinference": XinferenceEmbed,
    "Tongyi-Qianwen": QWenEmbed,
    "ZHIPU-AI": ZhipuEmbed,
    "FastEmbed": FastEmbed,
    "Youdao": YoudaoEmbed,
    "BaiChuan": BaiChuanEmbed,
    "Jina": JinaEmbed,
    "BAAI": DefaultEmbedding,
    "Mistral": MistralEmbed,
    "Bedrock": BedrockEmbed,
    "Gemini": GeminiEmbed,
    "NVIDIA": NvidiaEmbed,
    "LM-Studio": LmStudioEmbed
}


CvModel = {
    "OpenAI": GptV4,
    "Azure-OpenAI": AzureGptV4,
    "Ollama": OllamaCV,
    "Xinference": XinferenceCV,
    "Tongyi-Qianwen": QWenCV,
    "ZHIPU-AI": Zhipu4V,
    "Moonshot": LocalCV,
    "Gemini": GeminiCV,
    "OpenRouter": OpenRouterCV,
    "LocalAI": LocalAICV,
    "NVIDIA": NvidiaCV,
    "LM-Studio": LmStudioCV,
    "StepFun":StepFunCV
}


ChatModel = {
    "OpenAI": GptTurbo,
    "Azure-OpenAI": AzureChat,
    "ZHIPU-AI": ZhipuChat,
    "Tongyi-Qianwen": QWenChat,
    "Ollama": OllamaChat,
    "LocalAI": LocalAIChat,
    "Xinference": XinferenceChat,
    "Moonshot": MoonshotChat,
    "DeepSeek": DeepSeekChat,
    "VolcEngine": VolcEngineChat,
    "BaiChuan": BaiChuanChat,
    "MiniMax": MiniMaxChat,
    "Minimax": MiniMaxChat,
    "Mistral": MistralChat,
    "Gemini": GeminiChat,
    "Bedrock": BedrockChat,
    "Groq": GroqChat,
    "OpenRouter": OpenRouterChat,
    "StepFun": StepFunChat,
    "NVIDIA": NvidiaChat,
    "LM-Studio": LmStudioChat
}


RerankModel = {
    "BAAI": DefaultRerank,
    "Jina": JinaRerank,
    "Youdao": YoudaoRerank,
    "Xinference": XInferenceRerank,
    "NVIDIA": NvidiaRerank,
    "LM-Studio": LmStudioRerank
}


Seq2txtModel = {
    "OpenAI": GPTSeq2txt,
    "Tongyi-Qianwen": QWenSeq2txt,
    "Ollama": OllamaSeq2txt,
    "Azure-OpenAI": AzureSeq2txt,
    "Xinference": XinferenceSeq2txt
}
