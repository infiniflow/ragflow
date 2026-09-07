import type { ProviderConfig } from '../types';
import { GenericApiKeyConfig } from './generic-api-key-config';
import { LocalLlmConfigs } from './local-llm-configs';
import { ProviderConfigMap } from './provider-config-map';

/**
 * Get the configuration for the given factory
 * First look up in ProviderConfigMap, then LocalLlmConfigs, finally fall back to GenericApiKeyConfig
 */
export function getProviderConfig(llmFactory: string): ProviderConfig {
  // Check whether it is a special factory (11 in ModalMap)
  // Among which AzureOpenAI/VolcEngine/GoogleCloud/TencentCloud/XunFeiSpark/BaiduYiYan/FishAudio are in ProviderConfigMap
  // Bedrock/MinerU/PaddleOCR/OpenDataLoader are out of the merge scope and use the original modal

  if (ProviderConfigMap[llmFactory]) {
    return ProviderConfigMap[llmFactory];
  }

  if (LocalLlmConfigs[llmFactory]) {
    return LocalLlmConfigs[llmFactory];
  }

  // Generic ApiKey modal
  return {
    ...GenericApiKeyConfig,
    llmFactory,
  };
}
