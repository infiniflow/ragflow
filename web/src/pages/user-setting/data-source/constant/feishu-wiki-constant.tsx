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
import { TFunction } from 'i18next';

export const feishuWikiConstant = (t: TFunction) => [
  {
    label: t('setting.dataSourceFieldFeishuAppId'),
    name: 'config.credentials.app_id',
    type: FormFieldType.Text,
    required: true,
    placeholder: 'cli_xxxxxxxxxxxxxxxx',
  },
  {
    label: t('setting.dataSourceFieldFeishuAppSecret'),
    name: 'config.credentials.app_secret',
    type: FormFieldType.Password,
    required: true,
  },
  {
    label: t('setting.dataSourceFieldWikiSpaceId'),
    name: 'config.space_id',
    type: FormFieldType.Text,
    required: true,
    placeholder: '1234567890123456789',
  },
  {
    label: t('setting.dataSourceFieldRootNodeToken'),
    name: 'config.root_node_token',
    type: FormFieldType.Text,
    required: true,
    placeholder: 'wikcnExampleRootNodeToken',
  },
  {
    label: t('setting.dataSourceFieldIncludeExtensions'),
    name: 'config.include_extensions',
    type: FormFieldType.Tag,
    required: false,
    placeholder: 'pdf, docx, xlsx, pptx, jpg, png',
  },
  {
    label: t('setting.dataSourceFieldIncludeKeywords'),
    name: 'config.include_keywords',
    type: FormFieldType.Tag,
    required: false,
    placeholder: 'project, handbook',
  },
  {
    label: t('setting.dataSourceFieldExcludeKeywords'),
    name: 'config.exclude_keywords',
    type: FormFieldType.Tag,
    required: false,
    placeholder: 'draft, archived',
  },
  {
    label: t('setting.dataSourceFieldMaxFileSizeBytes'),
    name: 'config.max_file_size_bytes',
    type: FormFieldType.Number,
    required: false,
    validation: {
      min: 1,
      message: t('setting.dataSourceValidationMinOne', {
        label: t('setting.dataSourceFieldMaxFileSizeBytes'),
      }),
    },
  },
  {
    label: t('setting.dataSourceFieldBatchSize'),
    name: 'config.batch_size',
    type: FormFieldType.Number,
    required: false,
    validation: {
      min: 1,
      max: 10,
      message: t('setting.dataSourceValidationFeishuBatchSize'),
    },
  },
];

export const feishuWikiDefaultValues = {
  name: '',
  source: 'feishu_wiki',
  config: {
    space_id: '',
    root_node_token: '',
    include_extensions: [],
    include_keywords: [],
    exclude_keywords: [],
    max_file_size_bytes: 50 * 1024 * 1024,
    batch_size: 2,
    credentials: {
      app_id: '',
      app_secret: '',
    },
  },
};
