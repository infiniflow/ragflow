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

import { ModelTreeSelect } from '@/components/model-tree-select';
import { useTranslation } from 'react-i18next';
import { RAGFlowFormItem } from '../ragflow-form';

export type LLMFormFieldProps = {
  modelTypes?: string[];
  name?: string;
  testId?: string;
  optionTestIdPrefix?: string;
  config?: any;
  ownerTenantId?: string;
  required?: boolean;
};

export function LLMFormField({
  name,
  config,
  modelTypes,
  ownerTenantId,
  required = false,
}: LLMFormFieldProps) {
  const { t } = useTranslation();

  return (
    <RAGFlowFormItem
      name={name || 'llm_id'}
      label={t('chat.model')}
      required={required}
    >
      <ModelTreeSelect
        allowClear={config?.allowClear ?? false}
        modelTypes={modelTypes}
        ownerTenantId={ownerTenantId}
      />
    </RAGFlowFormItem>
  );
}
