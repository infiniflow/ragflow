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

import { AvatarUpload } from '@/components/avatar-upload';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { useTranslation } from 'react-i18next';

export function BasicInfoStep() {
  const { t } = useTranslation();

  return (
    <div className="max-w-2xl space-y-6 p-5 mx-auto w-full">
      <RAGFlowFormItem
        name="name"
        label={t('setting.groupName')}
        required
        horizontal
      >
        <Input placeholder={t('common.namePlaceholder')} />
      </RAGFlowFormItem>

      <RAGFlowFormItem
        name="description"
        label={t('setting.groupDescription')}
        horizontal
      >
        <Textarea
          placeholder={t('common.descriptionPlaceholder')}
          rows={3}
          resize="vertical"
        />
      </RAGFlowFormItem>

      <RAGFlowFormItem name="avatar" label={t('setting.avatar')} horizontal>
        <AvatarUpload tips={t('knowledgeConfiguration.photoTip')} />
      </RAGFlowFormItem>
    </div>
  );
}
