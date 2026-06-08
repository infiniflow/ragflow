import { FormFieldType } from '@/components/dynamic-form';
import { LLMFactory } from '@/constants/llm';
import type { FieldConfig, ProviderConfig } from '../types';
import { capitalize } from './utils';

/**
 * Factory configuration for local/compatible factories
 * Used for scenarios after OllamaModal merge
 */
export const LocalLlmConfigs: Record<string, ProviderConfig> = {
  [LLMFactory.Ollama]: buildLocalConfig(LLMFactory.Ollama, 'Ollama', [
    'chat',
    'embedding',
    'rerank',
    'image2text',
  ]),
  [LLMFactory.Xinference]: buildLocalConfig(
    LLMFactory.Xinference,
    'Xinference',
    ['chat', 'embedding', 'rerank', 'image2text', 'speech2text', 'tts'],
    'modelUid',
  ),
  [LLMFactory.ModelScope]: buildLocalConfig(
    LLMFactory.ModelScope,
    'ModelScope',
    ['chat'],
  ),
  [LLMFactory.LocalAI]: buildLocalConfig(LLMFactory.LocalAI, 'LocalAI', [
    'chat',
    'embedding',
    'rerank',
    'image2text',
  ]),
  [LLMFactory.LMStudio]: buildLocalConfig(LLMFactory.LMStudio, 'LMStudio', [
    'chat',
    'embedding',
    'image2text',
  ]),
  [LLMFactory.OpenAiAPICompatible]: buildLocalConfig(
    LLMFactory.OpenAiAPICompatible,
    'OpenAiAPICompatible',
    ['chat', 'embedding', 'rerank', 'image2text'],
  ),
  [LLMFactory.RAGcon]: buildLocalConfig(LLMFactory.RAGcon, 'RAGcon', [
    'chat',
    'embedding',
    'rerank',
    'image2text',
    'speech2text',
    'tts',
  ]),
  [LLMFactory.TogetherAI]: buildLocalConfig(
    LLMFactory.TogetherAI,
    'TogetherAI',
    ['chat', 'embedding', 'rerank', 'image2text'],
  ),
  [LLMFactory.Replicate]: buildLocalConfig(LLMFactory.Replicate, 'Replicate', [
    'chat',
    'embedding',
    'rerank',
    'image2text',
  ]),
  [LLMFactory.OpenRouter]: buildLocalConfig(
    LLMFactory.OpenRouter,
    'OpenRouter',
    ['chat', 'image2text'],
    undefined,
    true,
  ),
  [LLMFactory.HuggingFace]: buildLocalConfig(
    LLMFactory.HuggingFace,
    'HuggingFace',
    ['embedding', 'chat', 'rerank'],
  ),
  [LLMFactory.GPUStack]: buildLocalConfig(LLMFactory.GPUStack, 'GPUStack', [
    'chat',
    'embedding',
    'rerank',
    'speech2text',
    'tts',
  ]),
  [LLMFactory.VLLM]: buildLocalConfig(LLMFactory.VLLM, 'VLLM', [
    'chat',
    'embedding',
    'rerank',
    'image2text',
  ]),
  [LLMFactory.TokenPony]: buildLocalConfig(LLMFactory.TokenPony, 'TokenPony', [
    'chat',
    'embedding',
    'rerank',
    'image2text',
  ]),
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
    {
      name: 'model_type',
      label: 'modelType',
      type: FormFieldType.MultiSelect,
      required: true,
      options: modelTypes.map((t) => ({ label: capitalize(t), value: t })),
    },
    {
      name: 'model_name',
      label: modelNameLabel ?? 'modelName',
      type: FormFieldType.Text,
      required: true,
      placeholder: 'modelNameMessage',
    },
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
      type: FormFieldType.Text,
      required: false,
      placeholder: 'apiKeyMessage',
      shouldRender: 'hideWhenInstanceExists',
    },
    {
      name: 'max_tokens',
      label: 'maxTokens',
      type: FormFieldType.Number,
      required: true,
      placeholder: 'maxTokensTip',
      defaultValue: 8192,
      validation: { min: 0, message: 'maxTokensMessage' },
    },
    {
      name: 'is_tools',
      label: 'enableToolCall',
      type: FormFieldType.Switch,
      required: false,
      shouldRender: 'modelTypeSupportsToolCall',
      defaultValue: false,
    },
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

  return {
    llmFactory,
    title,
    fields,
    verifyTransform: (values, modelInfo) => ({
      apiKey: values.api_key || '',
      baseUrl: values.base_url,
      modelInfo,
    }),
    submitTransform: (values, modelInfo) => ({
      instance_name: values.instance_name,
      llm_factory: llmFactory,
      model_info: modelInfo,
      api_base: values.base_url,
      api_key: values.api_key,
      ...(values.provider_order
        ? { provider_order: values.provider_order }
        : {}),
    }),
  };
}
