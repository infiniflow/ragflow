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

import { RAGFlowFormItem } from '@/components/ragflow-form';
import { useCompilationTemplateGroupOptions } from '@/hooks/use-compilation-template-group-request';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useTranslation } from 'react-i18next';
import { SelectWithSearch } from './originui/select-with-search';

type CompilationTemplateFormFieldProps = {
  horizontal?: boolean;
  name?: string;
};

export function CompilationTemplateFormField({
  horizontal,
  name = 'parser_config.compilation_template_group_id',
}: CompilationTemplateFormFieldProps) {
  const { t } = useTranslation();
  const { navigateToAgents } = useNavigatePage();
  const options = useCompilationTemplateGroupOptions();

  return (
    <RAGFlowFormItem
      name={name}
      label={t('knowledgeConfiguration.compilationTemplate')}
      labelLink={{
        text: t('knowledgeConfiguration.createTemplate'),
        onClick: navigateToAgents,
      }}
      className="pb-4"
      horizontal={horizontal}
      required
    >
      {(field) => (
        <SelectWithSearch
          value={field.value}
          onChange={field.onChange}
          options={options}
        />
      )}
    </RAGFlowFormItem>
  );
}
