//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package models

import (
	"strings"
)

// ModelFactory creates ModelDriver instances based on provider name
type ModelFactory struct {
}

// NewModelFactory creates a new ModelFactory
func NewModelFactory() *ModelFactory {
	return &ModelFactory{}
}

// CreateModelDriver creates a ModelDriver for the given provider and model
func (f *ModelFactory) CreateModelDriver(providerName string, baseURL map[string]string, urlSuffix URLSuffix) (ModelDriver, error) {
	providerLower := strings.ToLower(providerName)
	switch providerLower {
	case "anthropic":
		return NewAnthropicModel(baseURL, urlSuffix), nil
	case "zhipu-ai":
		return NewZhipuAIModel(baseURL, urlSuffix), nil
	case "deepseek":
		return NewDeepSeekModel(baseURL, urlSuffix), nil
	case "moonshot":
		return NewMoonshotModel(baseURL, urlSuffix), nil
	case "minimax":
		return NewMinimaxModel(baseURL, urlSuffix), nil
	case "gitee":
		return NewGiteeModel(baseURL, urlSuffix), nil
	case "siliconflow":
		return NewSiliconflowModel(baseURL, urlSuffix), nil
	case "google":
		return NewGoogleModel(baseURL, urlSuffix), nil
	case "aliyun":
		return NewAliyunModel(baseURL, urlSuffix), nil
	case "volcengine":
		return NewVolcEngine(baseURL, urlSuffix), nil
	case "vllm":
		return NewVllmModel(baseURL, urlSuffix), nil
	case "xai":
		return NewXAIModel(baseURL, urlSuffix), nil
	case "lmstudio":
		return NewLmStudioModel(baseURL, urlSuffix), nil
	case "ollama":
		return NewOllamaModel(baseURL, urlSuffix), nil
	case "openai":
		return NewOpenAIModel(baseURL, urlSuffix), nil
	case "groq":
		return NewGroqModel(baseURL, urlSuffix), nil
	case "azure-openai":
		return NewAzureOpenAIModel(baseURL, urlSuffix), nil
	case "nvidia":
		return NewNvidiaModel(baseURL, urlSuffix), nil
	case "openrouter":
		return NewOpenRouterModel(baseURL, urlSuffix), nil
	case "huggingface":
		return NewHuggingFaceModel(baseURL, urlSuffix), nil
	case "baidu":
		return NewBaiduModel(baseURL, urlSuffix), nil
	case "cohere":
		return NewCoHereModel(baseURL, urlSuffix), nil
	case "cometapi":
		return NewCometAPIModel(baseURL, urlSuffix), nil
	case "fishaudio":
		return NewFishAudioModel(baseURL, urlSuffix), nil
	case "mistral":
		return NewMistralModel(baseURL, urlSuffix), nil
	case "upstage":
		return NewUpstageModel(baseURL, urlSuffix), nil
	case "stepfun":
		return NewStepFunModel(baseURL, urlSuffix), nil
	case "baichuan":
		return NewBaichuanModel(baseURL, urlSuffix), nil
	case "jina":
		return NewJinaModel(baseURL, urlSuffix), nil
	case "localai":
		return NewLocalAIModel(baseURL, urlSuffix), nil
	case "xinference":
		return NewXinferenceModel(baseURL, urlSuffix), nil
	case "astraflow":
		return NewAstraflowModel(baseURL, urlSuffix), nil
	case "modelscope":
		return NewModelScopeModel(baseURL, urlSuffix), nil
	case "longcat":
		return NewLongCatModel(baseURL, urlSuffix), nil
	case "hunyuan":
		return NewHunyuanModel(baseURL, urlSuffix), nil
	case "tokenpony":
		return NewTokenPonyModel(baseURL, urlSuffix), nil
	case "tokenhub":
		return NewTokenHubModel(baseURL, urlSuffix), nil
	case "novita":
		return NewNovitaModel(baseURL, urlSuffix), nil
	case "avian":
		return NewAvianModel(baseURL, urlSuffix), nil
	case "replicate":
		return NewReplicateModel(baseURL, urlSuffix), nil
	case "togetherai":
		return NewTogetherAIModel(baseURL, urlSuffix), nil
	case "ppio":
		return NewPPIOModel(baseURL, urlSuffix), nil
	case "voyage":
		return NewVoyageModel(baseURL, urlSuffix), nil
	case "paddleocr.net":
		return NewPaddleOCRModel(baseURL, urlSuffix), nil
	case "xunfei":
		return NewXunFeiModel(baseURL, urlSuffix), nil
	case "deepinfra":
		return NewDeepInfraModel(baseURL, urlSuffix), nil
	case "mineru.net":
		return NewMinerUModel(baseURL, urlSuffix), nil
	case "jiekouai":
		return NewJieKouAIModel(baseURL, urlSuffix), nil
	case "302.ai":
		return NewAI302Model(baseURL, urlSuffix), nil
	case "mineru":
		return NewMinerLocalUModel(baseURL, urlSuffix), nil
	case "futurmix":
		return NewFuturMixModel(baseURL, urlSuffix), nil
	case "perplexity":
		return NewPerplexityModel(baseURL, urlSuffix), nil
	case "gpustack":
		return NewGPUStackModel(baseURL, urlSuffix), nil
	case "n1n":
		return NewN1NModel(baseURL, urlSuffix), nil
	case "bedrock":
		return NewBedrockModel(baseURL, urlSuffix), nil
	case "paddleocr":
		return NewPaddleOCRLocalModel(baseURL, urlSuffix), nil
	case "orcarouter":
		return NewOrcaRouterModel(baseURL, urlSuffix), nil
	case "huaweicloud":
		return NewHuaweiCloudModel(baseURL, urlSuffix), nil
	case "qiniu":
		return NewQiniuModel(baseURL, urlSuffix), nil
	case "xiaomi":
		return NewXiaomiModel(baseURL, urlSuffix), nil
	default:
		return NewDummyModel(baseURL, urlSuffix), nil
	}
}
