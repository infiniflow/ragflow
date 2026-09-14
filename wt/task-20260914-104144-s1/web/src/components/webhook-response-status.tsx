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

import { useTranslation } from 'react-i18next';
import { RAGFlowFormItem } from './ragflow-form';
import { Input } from './ui/input';

type WebHookResponseStatusFormFieldProps = {
  name: string;
};

export function WebHookResponseStatusFormField({
  name,
}: WebHookResponseStatusFormFieldProps) {
  const { t } = useTranslation();

  return (
    <RAGFlowFormItem name={name} label={t('flow.webhook.status')}>
      <Input type="number"></Input>
    </RAGFlowFormItem>
  );
}
