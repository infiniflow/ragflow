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
import type { ProviderConfig } from '../types';

/**
 * Generic ApiKey configuration (used for factories not in ProviderConfigMap)
 */
export const GenericApiKeyConfig: ProviderConfig = {
  llmFactory: '__generic__',
  title: 'API Key',
  fields: [
    {
      name: 'instance_name',
      label: 'instanceName',
      type: FormFieldType.Text,
      required: true,
      placeholder: 'instanceNameMessage',
      tooltip: 'instanceNameTip',
      validation: { message: 'instanceNameMessage' },
    },
    {
      name: 'api_key',
      label: 'apiKey',
      type: FormFieldType.Password,
      required: true,
      placeholder: 'apiKeyMessage',
      validation: { message: 'apiKeyMessage' },
    },
    {
      name: 'base_url',
      label: 'baseUrl',
      type: 'inputSelect',
      required: false,
      tooltip: (factory) => {
        if (factory === LLMFactory.MiniMax) return 'minimaxBaseUrlTip';
        if (factory === LLMFactory.TongYiQianWen) return 'tongyiBaseUrlTip';
        if (factory === LLMFactory.SILICONFLOW) return 'siliconBaseUrlTip';
        return 'baseUrlTip';
      },
      placeholder: (factory) => {
        if (factory === LLMFactory.MiniMax) return 'minimaxBaseUrlPlaceholder';
        if (factory === LLMFactory.TongYiQianWen)
          return 'tongyiBaseUrlPlaceholder';
        if (factory === LLMFactory.SILICONFLOW)
          return 'siliconflowBaseUrlPlaceholder';
        if (factory?.toLowerCase() === 'Anthropic')
          return 'anthropicBaseUrlPlaceholder';
        return 'openaiBaseUrlPlaceholder';
      },
      shouldRender: 'showBaseUrl',
    },
    {
      name: 'group_id',
      label: 'groupId',
      type: FormFieldType.Text,
      required: false,
      shouldRender: 'showGroupId',
    },
  ],
  verifyTransform: (values) => ({
    apiKey: values.api_key,
    baseUrl: values.base_url,
  }),
  submitTransform: (values) => {
    const apiKey = values.group_id
      ? { api_key: values.api_key ?? '', group_id: values.group_id }
      : (values.api_key ?? '');
    return {
      instance_name: values.instance_name,
      api_key: apiKey,
      api_base: values.base_url || '',
    };
  },
};

/**
 * List of factories supporting base_url (used for the generic ApiKey modal)
 */
export const FACTORIES_WITH_BASE_URL = [
  LLMFactory.OpenAI,
  LLMFactory.AzureOpenAI,
  LLMFactory.TongYiQianWen,
  LLMFactory.MiniMax,
  LLMFactory.SILICONFLOW,
];
