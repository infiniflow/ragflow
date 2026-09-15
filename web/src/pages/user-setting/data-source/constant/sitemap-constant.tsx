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

export const sitemapConstant = (t: TFunction) => [
  {
    label: t('setting.dataSourceFieldSitemapUrl'),
    name: 'config.sitemap_url',
    type: FormFieldType.Text,
    required: true,
    placeholder: 'https://example.com/sitemap.xml',
    tooltip: t('setting.sitemapUrlTip'),
  },
  {
    label: t('setting.dataSourceFieldUrlFilter'),
    name: 'config.url_filter',
    type: FormFieldType.Text,
    required: false,
    placeholder: '^https://example\\.com/docs/',
    tooltip: t('setting.sitemapUrlFilterTip'),
  },
  {
    label: t('setting.dataSourceFieldFollowPdfLinks'),
    name: 'config.follow_pdf_links',
    type: FormFieldType.Switch,
    required: false,
    defaultValue: false,
    tooltip: t('setting.sitemapFollowPdfLinksTip'),
  },
  {
    label: t('setting.dataSourceFieldRestrictPdfToDomain'),
    name: 'config.restrict_pdf_to_domain',
    type: FormFieldType.Switch,
    required: false,
    defaultValue: true,
    shouldRender: (formValues: any) =>
      formValues?.config?.follow_pdf_links === true,
    tooltip: t('setting.sitemapRestrictPdfToDomainTip'),
  },
  {
    label: t('setting.dataSourceFieldUserAgent'),
    name: 'config.user_agent',
    type: FormFieldType.Text,
    required: false,
    placeholder: 'RAGFlow-SitemapConnector/1.0',
    tooltip: t('setting.sitemapUserAgentTip'),
  },
  {
    label: t('setting.dataSourceFieldBatchSize'),
    name: 'config.batch_size',
    type: FormFieldType.Number,
    required: false,
    tooltip: t('setting.sitemapBatchSizeTip'),
    validation: {
      min: 1,
      message: t('setting.dataSourceValidationMinOne', {
        label: t('setting.dataSourceFieldBatchSize'),
      }),
    },
  },
];
