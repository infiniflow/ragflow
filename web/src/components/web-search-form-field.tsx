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

import queritLogo from '@/assets/querit.png';
import serplyLogo from '@/assets/serply.png';
import tavilyLogo from '@/assets/svg/tavily.svg';
import youcomLogo from '@/assets/svg/youcom.svg';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { WebSearchProvider } from '@/constants/chat';
import { useTranslate } from '@/hooks/common-hooks';
import { prefixName } from '@/utils/form';
import { useFormContext, useWatch } from 'react-hook-form';
import PasswordInput from './originui/password-input';
import { SelectWithSearch } from './originui/select-with-search';

interface IProps {
  prefix?: string;
}

const providerOptions = [
  {
    name: 'Tavily',
    logo: tavilyLogo,
    value: WebSearchProvider.Tavily,
  },
  {
    name: 'Querit',
    logo: queritLogo,
    value: WebSearchProvider.Querit,
  },
  {
    name: 'Serply',
    logo: serplyLogo,
    value: WebSearchProvider.Serply,
  },
  {
    name: 'You.com',
    logo: youcomLogo,
    value: WebSearchProvider.YouCom,
  },
]
  .sort((left, right) => left.name.localeCompare(right.name))
  .map(({ name, logo, value }) => ({
    label: (
      <span className="flex items-center gap-2">
        <img
          src={logo}
          alt=""
          aria-hidden="true"
          className="size-4 shrink-0 object-contain"
        />
        {name}
      </span>
    ),
    value,
  }));

const providerKeyConfig = {
  [WebSearchProvider.Tavily]: {
    name: 'prompt_config.tavily_api_key',
    label: 'Tavily API Key',
    tip: 'tavilyApiKeyTip',
    placeholder: 'tavilyApiKeyMessage',
    helpUrl: 'https://app.tavily.com/home',
  },
  [WebSearchProvider.Querit]: {
    name: 'prompt_config.querit_api_key',
    label: 'Querit API Key',
    tip: 'queritApiKeyTip',
    placeholder: 'queritApiKeyMessage',
    helpUrl: 'https://querit.ai',
  },
  [WebSearchProvider.Serply]: {
    name: 'prompt_config.serply_api_key',
    label: 'Serply API Key',
    tip: 'serplyApiKeyTip',
    placeholder: 'serplyApiKeyMessage',
    helpUrl: 'https://serply.io',
  },
  [WebSearchProvider.YouCom]: {
    name: 'prompt_config.youcom_api_key',
    label: 'You.com API Key',
    tip: 'youcomApiKeyTip',
    placeholder: 'youcomApiKeyMessage',
    helpUrl:
      'https://you.com/platform?utm_source=infiniflow-ragflow&utm_medium=oss_integration&utm_campaign=2026-08-oss-integrations&utm_content=app',
  },
} as const;

export function WebSearchFormField({ prefix = '' }: IProps) {
  const form = useFormContext();
  const { t } = useTranslate('chat');
  const providerName = prefixName(prefix, 'prompt_config.web_search_provider');
  const selectedProvider = useWatch({
    control: form.control,
    name: providerName,
  });
  const keyConfig = providerKeyConfig[selectedProvider as WebSearchProvider];

  return (
    <>
      <RAGFlowFormItem
        name={providerName}
        label={t('webSearchProvider')}
        tooltip={t('webSearchProviderTip')}
      >
        {(field) => (
          <SelectWithSearch
            value={field.value}
            onChange={field.onChange}
            options={providerOptions}
            placeholder={t('webSearchProviderPlaceholder')}
            allowClear
            testId="web-search-provider"
            optionTestIdPrefix="web-search-provider-option"
          />
        )}
      </RAGFlowFormItem>
      {keyConfig && (
        <RAGFlowFormItem
          key={selectedProvider}
          name={prefixName(prefix, keyConfig.name)}
          label={keyConfig.label}
          tooltip={t(keyConfig.tip)}
          description={
            <a href={keyConfig.helpUrl} target="_blank" rel="noreferrer">
              {t('tavilyApiKeyHelp')}
            </a>
          }
        >
          {(field) => (
            <PasswordInput
              {...field}
              value={field.value ?? ''}
              placeholder={t(keyConfig.placeholder)}
              autoComplete="new-password"
            />
          )}
        </RAGFlowFormItem>
      )}
    </>
  );
}
