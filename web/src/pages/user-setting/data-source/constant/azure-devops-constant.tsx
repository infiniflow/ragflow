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

import { FilterFormField, FormFieldType } from '@/components/dynamic-form';
import { TFunction } from 'i18next';

export const azureDevOpsConstant = (t: TFunction) => [
  {
    label: t('setting.dataSourceFieldAzureDevOpsPat'),
    name: 'config.credentials.azure_devops_pat',
    type: FormFieldType.Password,
    required: true,
    tooltip: t('setting.azureDevOpsPatTip'),
  },
  {
    label: t('setting.dataSourceFieldAzureDevOpsOrganization'),
    name: 'config.organization',
    type: FormFieldType.Text,
    required: true,
    tooltip: t('setting.azureDevOpsOrganizationTip'),
  },
  {
    label: t('setting.dataSourceFieldIndexMode'),
    name: 'config.index_mode',
    type: FormFieldType.Segmented,
    options: [
      {
        label: t('setting.dataSourceOptionOrganization'),
        value: 'organization',
      },
      { label: t('setting.dataSourceOptionProjects'), value: 'projects' },
      {
        label: t('setting.dataSourceOptionRepositories'),
        value: 'repositories',
      },
    ],
  },
  {
    label: t('setting.dataSourceFieldProjects'),
    name: 'config.projects',
    type: FormFieldType.Text,
    customValidate: (val: string, formValues: any) => {
      const index_mode = formValues?.config?.index_mode;
      if (!val?.trim() && index_mode === 'projects') {
        return t('setting.dataSourceValidationFieldRequired', {
          label: t('setting.dataSourceFieldProjects'),
        });
      }
      return true;
    },
    shouldRender: (formValues: any) =>
      formValues?.config?.index_mode === 'projects',
    tooltip: t('setting.azureDevOpsProjectsTip'),
  },
  {
    label: t('setting.dataSourceFieldAzureDevOpsRepositories'),
    name: 'config.repositories',
    type: FormFieldType.Text,
    customValidate: (val: string, formValues: any) => {
      const index_mode = formValues?.config?.index_mode;
      if (!val?.trim() && index_mode === 'repositories') {
        return t('setting.dataSourceValidationFieldRequired', {
          label: t('setting.dataSourceFieldAzureDevOpsRepositories'),
        });
      }
      return true;
    },
    shouldRender: (formValues: any) =>
      formValues?.config?.index_mode === 'repositories',
    tooltip: t('setting.azureDevOpsRepositoriesTip'),
  },
  {
    name: FilterFormField + '.tip',
    label: ' ',
    type: FormFieldType.Custom,
    shouldRender: (formValues: any) =>
      formValues?.config?.index_mode === 'organization',
    render: () => (
      <div className="text-sm text-text-secondary bg-bg-card border border-border-button rounded-md px-3 py-2">
        {t('setting.azureDevOpsOrganizationScopeTip')}
      </div>
    ),
  },
  {
    label: t('setting.dataSourceFieldAzureDevOpsContentTypes'),
    name: 'config.content_types',
    type: FormFieldType.Segmented,
    options: [
      { label: t('setting.dataSourceOptionCode'), value: 'code' },
      {
        label: t('setting.dataSourceOptionPullRequests'),
        value: 'pull_requests',
      },
      { label: t('setting.dataSourceOptionBoth'), value: 'both' },
    ],
    tooltip: t('setting.azureDevOpsContentTypesTip'),
  },
];
