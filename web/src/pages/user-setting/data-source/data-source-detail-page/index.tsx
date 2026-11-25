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

import BackButton from '@/components/back-button';
import {
  DynamicForm,
  DynamicFormRef,
  FormFieldConfig,
  FormFieldType,
} from '@/components/dynamic-form';
import { PageFrame } from '@/components/page-frame';
import { SettingsFormContainer } from '@/components/settings-form-container';
import { Button } from '@/components/ui/button';
import { CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { RunningStatus, RunningStatusOld } from '@/constants/knowledge';
import { shouldDisableEeDataSourceField } from '@/ee/pages/user-setting/data-source/ee-extensions';
import { isEqual } from 'lodash';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { FieldValues } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router';
import {
  DataSourceFormDefaultValues,
  getCommonExtraDefaultValues,
  getDataSourceFormBaseFields,
  getDataSourceFieldsWithExtras,
  mergeDataSourceFormValues,
  useDataSourceInfo,
} from '../constant';
import {
  useAddDataSource,
  useFetchDataSourceDetail,
  useTestDataSource,
  useUpdateDataSourceStatus,
} from '../hooks';
import { DataSourceLogsTable } from './log-table';

const SourceDetailPage = () => {
  const { t } = useTranslation();
  const formRef = useRef<DynamicFormRef>(null);
  const [searchParams] = useSearchParams();
  const connectorId = searchParams.get('id')!;

  const { data: detail } = useFetchDataSourceDetail();
  const { updateStatus, loading: statusUpdateLoading } =
    useUpdateDataSourceStatus();
  const { dataSourceInfo } = useDataSourceInfo();
  const detailInfo = useMemo(() => {
    if (detail) {
      return dataSourceInfo[detail.source];
    }
  }, [detail, dataSourceInfo]);

  const [isDirty, setIsDirty] = useState(false);

  const customFields = useMemo(() => {
    return [
      {
        label: t('setting.dataSourcePruneFreq'),
        name: 'prune_freq',
        type: FormFieldType.Number,
        required: false,
        shouldRender: (values: any) => !!values?.config?.sync_deleted_files,
        render: (fieldProps: FormFieldConfig) => {
          return (
            <Input
              {...fieldProps}
              type={FormFieldType.Number}
              suffix={
                <span className="px-2 text-text-secondary italic">
                  {t('setting.minutes')}
                </span>
              }
            />
          );
        },
      },
      {
        label: t('setting.dataSourceRefreshFreq'),
        name: 'refresh_freq',
        type: FormFieldType.Number,
        required: false,
        render: (fieldProps: FormFieldConfig) => (
          <Input
            {...fieldProps}
            type={FormFieldType.Number}
            suffix={
              <span className="px-2 text-text-secondary italic">
                {t('setting.minutes')}
              </span>
            }
          />
        ),
      },
      {
        label: t('setting.dataSourceTimeoutSecs'),
        name: 'timeout_secs',
        type: FormFieldType.Number,
        required: false,
        render: (fieldProps: FormFieldConfig) => (
          <div className="flex items-center gap-1 w-full relative">
            <div className="flex-1">
              <Input
                {...fieldProps}
                type={FormFieldType.Number}
                suffix={
                  <span className="px-2 text-text-secondary italic">
                    {t('setting.seconds')}
                  </span>
                }
              />
            </div>
          </div>
        ),
      },
    ];
  }, [t]);

  const fields = useMemo<FormFieldConfig[]>(() => {
    if (!detail) {
      return [];
    }
    const baseFields = getDataSourceFormBaseFields(t).map((field) =>
      field.name === 'name' ? { ...field, disabled: true } : { ...field },
    );
    const allFields = [
      ...baseFields,
      ...getDataSourceFieldsWithExtras(t, detail.source as any),
      ...customFields,
    ] as FormFieldConfig[];
    return allFields.map((field) => ({
      ...field,
      horizontal: true,
      onChange: undefined,
      ...(shouldDisableEeDataSourceField(field.name) ? { disabled: true } : {}),
    }));
  }, [detail, customFields, t]);

  const defaultValues = useMemo<FieldValues>(() => {
    if (!detail) {
      return {};
    }
    return mergeDataSourceFormValues(
      DataSourceFormDefaultValues[
        detail.source as keyof typeof DataSourceFormDefaultValues
      ] as FieldValues,
      getCommonExtraDefaultValues(),
      detail as FieldValues,
    );
  }, [detail]);

  const { addLoading, handleAddOk } = useAddDataSource({ isEdit: true });
  const { loading: testLoading, handleTest } = useTestDataSource(
    formRef,
    connectorId,
    fields,
  );

  const onSubmit = useCallback(() => {
    formRef?.current?.submit();
  }, []);

  const isUnstarted = useMemo(
    () =>
      detail?.status === RunningStatus.UNSTART ||
      detail?.status === RunningStatusOld.UNSTART,
    [detail?.status],
  );

  const isConnectorActive = useMemo(
    () =>
      detail?.status === RunningStatus.RUNNING ||
      detail?.status === RunningStatus.SCHEDULE ||
      detail?.status === RunningStatusOld.RUNNING ||
      detail?.status === RunningStatusOld.SCHEDULE,
    [detail?.status],
  );

  const actionMode = useMemo(() => {
    if (isDirty) {
      return 'save' as const;
    }

    if (isUnstarted) {
      return 'save' as const;
    }

    if (isConnectorActive) {
      return 'stop' as const;
    }

    return 'resume' as const;
  }, [isConnectorActive, isDirty, isUnstarted]);

  const handlePrimaryAction = useCallback(() => {
    if (actionMode === 'save') {
      onSubmit();
      return;
    }
    updateStatus(
      actionMode === 'resume' ? RunningStatus.SCHEDULE : RunningStatus.CANCEL,
    );
  }, [actionMode, onSubmit, updateStatus]);

  const primaryActionLabel = useMemo(() => {
    if (actionMode === 'stop') return t('common.stop');
    if (actionMode === 'resume') return t('common.resume');
    return t('common.save');
  }, [actionMode, t]);

  useEffect(() => {
    setIsDirty(false);
  }, [detail]);

  useEffect(() => {
    const instance = formRef.current;
    if (!instance) return;

    setIsDirty(!isEqual(instance.getValues(), defaultValues));
    return instance.watchDirty((_nextIsDirty, values) => {
      setIsDirty(!isEqual(values, defaultValues));
    });
  }, [defaultValues, fields]);

  return (
    <PageFrame>
      <SettingsFormContainer className="flex min-h-0 flex-1 flex-col">
        <CardHeader className="flex flex-row items-center justify-between space-y-0 p-5 shrink-0">
          <CardTitle className="text-2xl text-text-primary flex gap-1 items-center font-normal">
            <BackButton className="border-none">
              <></>
            </BackButton>
            {detailInfo?.icon}
            {detail?.name}
          </CardTitle>
        </CardHeader>
        <CardContent className="px-5 py-0 flex-1 min-h-0 overflow-y-auto scrollbar-auto">
          <div className="flex min-h-full flex-col gap-10">
            <div className="shrink-0">
              <DynamicForm.Root
                ref={formRef}
                fields={fields}
                onSubmit={(data) => handleAddOk(data)}
                defaultValues={defaultValues}
              />
            </div>
            <div className="flex shrink-0 justify-end gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={handleTest}
                disabled={testLoading}
                loading={testLoading}
              >
                {t('setting.dataSourceTestConnection')}
              </Button>
              <Button
                type="button"
                onClick={handlePrimaryAction}
                disabled={addLoading || statusUpdateLoading}
                loading={
                  (addLoading && actionMode === 'save') ||
                  (statusUpdateLoading && actionMode !== 'save')
                }
              >
                {primaryActionLabel}
              </Button>
            </div>
            <section className="flex flex-1 flex-col gap-2">
              <div className="mb-2 shrink-0 text-2xl text-text-primary">
                {t('setting.log')}
              </div>
              <DataSourceLogsTable autoRefresh={isConnectorActive} />
            </section>
          </div>
        </CardContent>
      </SettingsFormContainer>
    </PageFrame>
  );
};
export default SourceDetailPage;
