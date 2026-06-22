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
