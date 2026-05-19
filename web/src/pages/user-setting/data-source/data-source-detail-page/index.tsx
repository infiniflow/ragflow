import BackButton from '@/components/back-button';
import {
  DynamicForm,
  DynamicFormRef,
  FormFieldConfig,
  FormFieldType,
} from '@/components/dynamic-form';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Separator } from '@/components/ui/separator';
import { RunningStatus, RunningStatusOld } from '@/constants/knowledge';
import { t } from 'i18next';
import { isEqual } from 'lodash';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { FieldValues } from 'react-hook-form';
import {
  DataSourceFormBaseFields,
  DataSourceFormDefaultValues,
  DataSourceKey,
  getCommonExtraDefaultValues,
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
  const formRef = useRef<DynamicFormRef>(null);

  const { data: detail } = useFetchDataSourceDetail();
  const { updateStatus, loading: statusUpdateLoading } =
    useUpdateDataSourceStatus();
  const { dataSourceInfo } = useDataSourceInfo();
  const detailInfo = useMemo(() => {
    if (detail) {
      return dataSourceInfo[detail.source];
    }
  }, [detail, dataSourceInfo]);

  const [fields, setFields] = useState<FormFieldConfig[]>([]);
  const [isDirty, setIsDirty] = useState(false);
  const [defaultValues, setDefaultValues] = useState<FieldValues>(
    DataSourceFormDefaultValues[
      detail?.source as keyof typeof DataSourceFormDefaultValues
    ] as FieldValues,
  );

  const customFields = useMemo(() => {
    return [
      {
        label: 'Prune Freq',
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
        label: 'Refresh Freq',
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
        label: 'Timeout Secs',
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
  }, []);

  const { addLoading, handleAddOk } = useAddDataSource({ isEdit: true });
  const { loading: testLoading, handleTest } = useTestDataSource();

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
    if (actionMode === 'stop') return 'Stop';
    if (actionMode === 'resume') return 'Resume';
    return 'Save';
  }, [actionMode]);

  useEffect(() => {
    const baseFields = DataSourceFormBaseFields.map((field) => {
      if (field.name === 'name') {
        return {
          ...field,
          disabled: true,
        };
      } else {
        return {
          ...field,
        };
      }
    });
    if (detail) {
      const fields = [
        ...baseFields,
        ...getDataSourceFieldsWithExtras(detail.source as any),
        ...customFields,
      ] as FormFieldConfig[];

      const newFields = fields.map((field) => {
        return {
          ...field,
          horizontal: true,
          onChange: undefined,
        };
      });
      setFields(newFields);

      const defaultValueTemp = {
        ...mergeDataSourceFormValues(
          DataSourceFormDefaultValues[
            detail?.source as keyof typeof DataSourceFormDefaultValues
          ] as FieldValues,
          getCommonExtraDefaultValues(),
          detail as FieldValues,
        ),
      };
      setDefaultValues(defaultValueTemp);
      setIsDirty(false);
    }
  }, [detail, customFields, onSubmit]);

  useEffect(() => {
    const instance = formRef.current;
    if (!instance) return;

    setIsDirty(!isEqual(instance.getValues(), defaultValues));
    return instance.watchDirty((_nextIsDirty, values) => {
      setIsDirty(!isEqual(values, defaultValues));
    });
  }, [defaultValues, fields]);

  return (
    <div className="px-10 py-5">
      <BackButton />
      <Card className="bg-transparent border border-border-button px-5 pt-[10px] pb-5 rounded-md mt-5">
        <CardHeader className="flex flex-row items-center justify-between space-y-0 p-0 pb-3">
          {/* <Users className="mr-2 h-5 w-5 text-[#1677ff]" /> */}
          <CardTitle className="text-2xl text-text-primary flex gap-1 items-center font-normal pb-3">
            {detailInfo?.icon}
            {detail?.name}
          </CardTitle>
        </CardHeader>
        <Separator className="border-border-button bg-border-button w-[calc(100%+2rem)] -translate-x-4 -translate-y-4" />
        <CardContent className="p-2 flex flex-col gap-10 max-h-[calc(100vh-190px)] overflow-y-auto scrollbar-auto">
          <div className="max-w-[1200px]">
            <DynamicForm.Root
              ref={formRef}
              fields={fields}
              onSubmit={(data) => handleAddOk(data)}
              defaultValues={defaultValues}
            />
          </div>
          <div className="max-w-[1200px] flex justify-end gap-2">
            {detail?.source === DataSourceKey.REST_API && (
              <Button
                type="button"
                variant="outline"
                onClick={handleTest}
                disabled={testLoading}
                loading={testLoading}
              >
                {t('setting.restApiTestConnection')}
              </Button>
            )}
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
          <section className="flex flex-col gap-2">
            <div className="text-2xl text-text-primary mb-2">
              {t('setting.log')}
            </div>
            <DataSourceLogsTable autoRefresh={isConnectorActive} />
          </section>
        </CardContent>
      </Card>
    </div>
  );
};
export default SourceDetailPage;
