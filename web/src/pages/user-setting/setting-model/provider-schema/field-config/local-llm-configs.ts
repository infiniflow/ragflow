/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { FormFieldType } from '@/components/dynamic-form';
import { LLMFactory } from '@/constants/llm';
import type { FieldConfig, ProviderConfig } from '../types';
import { buildModelInfoFromValues } from './utils';

/**
 * Factory configuration for local/compatible factories
 * Used for scenarios after OllamaModal merge
 */
export const LocalLlmConfigs: Record<string, ProviderConfig> = {
  [LLMFactory.Ollama]: buildLocalConfig(
    LLMFactory.Ollama,
    'Ollama',
    ['chat', 'embedding', 'rerank', 'image2text'],
    undefined,
    false,
    undefined,
    'https://github.com/infiniflow/ragflow/blob/main/docs/guides/models/deploy_local_llm.mdx',
  ),
  [LLMFactory.Xinference]: buildLocalConfig(
    LLMFactory.Xinference,
    'Xinference',
    ['chat', 'embedding', 'rerank', 'image2text', 'speech2text', 'tts'],
    'modelUid',
    false,
    undefined,
    'https://inference.readthedocs.io/en/latest/user_guide',
  ),
  [LLMFactory.FunASR]: buildLocalConfig(
    LLMFactory.FunASR,
    'FunASR',
    ['speech2text'],
    undefined,
    false,
    [
      {
        name: 'base_url',
        label: 'addLlmBaseUrl',
        type: 'inputSelect',
        required: true,
        defaultValue: 'http://localhost:8000/v1',
        placeholder: 'baseUrlNameMessage',
        shouldRender: 'hideWhenInstanceExists',
      },
    ],
    'https://github.com/modelscope/FunASR',
  ),
  [LLMFactory.ModelScope]: buildLocalConfig(
    LLMFactory.ModelScope,
    'ModelScope',
    ['chat'],
    undefined,
    false,
    undefined,
    'https://www.modelscope.cn/docs/model-service/API-Inference/intro',
  ),
  [LLMFactory.LocalAI]: buildLocalConfig(
    LLMFactory.LocalAI,
    'LocalAI',
    ['chat', 'embedding', 'rerank', 'image2text'],
    undefined,
    false,
    undefined,
    'https://localai.io/docs/getting-started/models/',
  ),
  [LLMFactory.LMStudio]: buildLocalConfig(
    LLMFactory.LMStudio,
    'LMStudio',
    ['chat', 'embedding', 'image2text'],
    undefined,
    false,
    undefined,
    'https://lmstudio.ai/docs/basics',
  ),
  [LLMFactory.OpenAiAPICompatible]: buildLocalConfig(
    LLMFactory.OpenAiAPICompatible,
    'OpenAiAPICompatible',
    ['chat', 'embedding', 'rerank', 'image2text'],
    undefined,
    false,
    undefined,
    'https://platform.openai.com/docs/models/gpt-4',
  ),
  [LLMFactory.RAGcon]: buildLocalConfig(
    LLMFactory.RAGcon,
    'RAGcon',
    ['chat', 'embedding', 'rerank', 'image2text', 'speech2text', 'tts'],
    undefined,
    false,
    undefined,
    'https://www.ragcon.ai/erste-schritte-mit-ragflow/',
  ),
  [LLMFactory.TogetherAI]: buildLocalConfig(
    LLMFactory.TogetherAI,
    'TogetherAI',
    ['chat', 'embedding', 'rerank', 'image2text'],
    undefined,
    false,
    undefined,
    'https://docs.together.ai/docs/deployment-options',
  ),
  [LLMFactory.Replicate]: buildLocalConfig(
    LLMFactory.Replicate,
    'Replicate',
    ['chat', 'embedding', 'rerank', 'image2text'],
    undefined,
    false,
    undefined,
    'https://replicate.com/docs/topics/deployments',
  ),
  [LLMFactory.OpenRouter]: buildLocalConfig(
    LLMFactory.OpenRouter,
    'OpenRouter',
    ['chat', 'image2text'],
    undefined,
    true,
    [
      {
        name: 'base_url',
        label: 'addLlmBaseUrl',
        type: 'inputSelect',
        required: false,
        placeholder: 'baseUrlNameMessage',
        shouldRender: 'hideWhenInstanceExists',
      },
    ],
    'https://openrouter.ai/docs',
  ),
  [LLMFactory.HuggingFace]: buildLocalConfig(
    LLMFactory.HuggingFace,
    'HuggingFace',
    ['embedding', 'chat', 'rerank'],
    undefined,
    false,
    undefined,
    'https://huggingface.co/docs/text-embeddings-inference/quick_tour',
  ),
  [LLMFactory.GPUStack]: buildLocalConfig(
    LLMFactory.GPUStack,
    'GPUStack',
    ['chat', 'embedding', 'rerank', 'speech2text', 'tts'],
    undefined,
    false,
    undefined,
    'https://docs.gpustack.ai/latest/quickstart',
  ),
  [LLMFactory.VLLM]: buildLocalConfig(
    LLMFactory.VLLM,
    'VLLM',
    ['chat', 'embedding', 'rerank', 'image2text'],
    undefined,
    false,
    undefined,
    'https://docs.vllm.ai/en/latest/',
  ),
  [LLMFactory.NewAPI]: buildLocalConfig(
    LLMFactory.NewAPI,
    'New API',
    ['chat', 'embedding', 'rerank', 'image2text', 'tts', 'speech2text'],
    undefined,
    false,
    undefined,
    'https://github.com/QuantumNous/new-api',
  ),
  // [LLMFactory.TokenPony]: buildLocalConfig(
  //   LLMFactory.TokenPony,
  //   'TokenPony',
  //   ['chat', 'embedding', 'rerank', 'image2text'],
  //   undefined,
  //   false,
  //   undefined,
  //   'https://docs.tokenpony.cn/#/',
  // ),
};

/**
 * Build the default configuration for local factories
 */
function buildLocalConfig(
  llmFactory: string,
  title: string,
  modelTypes: string[],
  modelNameLabel?: string,
  addProviderOrder = false,
  customFields?: FieldConfig[],
  docLink?: string,
): ProviderConfig {
  const fields: FieldConfig[] = [
    {
      name: 'instance_name',
      label: 'instanceName',
      type: FormFieldType.Text,
      required: true,
      placeholder: 'instanceNameMessage',
      tooltip: 'instanceNameTip',
    },
    // {
    //   name: 'model_type',
    //   label: 'modelType',
    //   type: FormFieldType.MultiSelect,
    //   required: true,
    //   options: modelTypes.map((t) => ({ label: capitalize(t), value: t })),
    // },
    // {
    //   name: 'model_name',
    //   label: modelNameLabel ?? 'modelName',
    //   type: FormFieldType.Text,
    //   required: true,
    //   placeholder: 'modelNameMessage',
    // },
    {
      name: 'base_url',
      label: 'addLlmBaseUrl',
      type: 'inputSelect',
      required: true,
      placeholder: 'baseUrlNameMessage',
      shouldRender: 'hideWhenInstanceExists',
    },
    {
      name: 'api_key',
      label: 'apiKey',
      type: FormFieldType.Password,
      required: false,
      placeholder: 'apiKeyMessage',
      shouldRender: 'hideWhenInstanceExists',
    },
    // {
    //   name: 'max_tokens',
    //   label: 'maxTokens',
    //   type: FormFieldType.Number,
    //   required: true,
    //   placeholder: 'maxTokensTip',
    //   defaultValue: 8192,
    //   validation: { min: 0, message: 'maxTokensMessage' },
    // },
    // {
    //   name: 'is_tools',
    //   label: 'enableToolCall',
    //   type: FormFieldType.Switch,
    //   required: false,
    //   shouldRender: 'modelTypeSupportsToolCall',
    //   defaultValue: false,
    // },
  ];

  if (addProviderOrder) {
    fields.push({
      name: 'provider_order',
      label: 'providerOrder',
      type: FormFieldType.Text,
      required: false,
    });
  }

  fields.push({
    name: 'vision',
    label: 'vision',
    type: FormFieldType.Switch,
    required: false,
    defaultValue: false,
    shouldRender: 'modelTypeIncludesChat',
  });

  const customFieldMap = new Map((customFields ?? []).map((f) => [f.name, f]));
  const mergedFields = fields
    .map((f) => customFieldMap.get(f.name) ?? f)
    .concat(
      (customFields ?? []).filter(
        (f) => !fields.some((ef) => ef.name === f.name),
      ),
    );
  return {
    llmFactory,
    title,
    fields: mergedFields,
    ...(docLink ? { docLink, docLinkI18nKey: 'ollamaLink' } : {}),
    verifyTransform: (values) => ({
      apiKey: values.api_key || '',
      baseUrl: values.base_url,
      modelInfo: buildModelInfoFromValues(values),
    }),
    submitTransform: (values) => {
      const apiKey = values.provider_order
        ? {
            api_key: values.api_key ?? '',
            provider_order: values.provider_order,
          }
        : (values.api_key ?? '');
      return {
        instance_name: values.instance_name,
        llm_factory: llmFactory,
        model_info: buildModelInfoFromValues(values),
        api_base: values.base_url,
        api_key: apiKey,
      };
    },
  };
}
