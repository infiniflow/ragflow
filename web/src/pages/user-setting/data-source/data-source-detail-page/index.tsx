import BackButton from '@/components/back-button';
import {
  DynamicForm,
  DynamicFormRef,
  FormFieldConfig,
  FormFieldType,
} from '@/components/dynamic-form';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Separator } from '@/components/ui/separator';
import { RunningStatus } from '@/constants/knowledge';
import { t } from 'i18next';
import { debounce } from 'lodash';
import { CirclePause, Repeat } from 'lucide-react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { FieldValues } from 'react-hook-form';
import {
  DataSourceFormBaseFields,
  DataSourceFormDefaultValues,
  DataSourceFormFields,
  DataSourceInfo,
} from '../contant';
import {
  useAddDataSource,
  useDataSourceResume,
  useFetchDataSourceDetail,
} from '../hooks';
import { DataSourceLogsTable } from './log-table';

const SourceDetailPage = () => {
  const formRef = useRef<DynamicFormRef>(null);

  const { data: detail } = useFetchDataSourceDetail();
  const { handleResume } = useDataSourceResume();

  const detailInfo = useMemo(() => {
    if (detail) {
      return DataSourceInfo[detail.source];
    }
  }, [detail]);

  const [fields, setFields] = useState<FormFieldConfig[]>([]);
  const [defaultValues, setDefaultValues] = useState<FieldValues>(
    DataSourceFormDefaultValues[
      detail?.source as keyof typeof DataSourceFormDefaultValues
    ] as FieldValues,
  );

  const runSchedule = useCallback(() => {
    handleResume({
      resume:
        detail?.status === RunningStatus.RUNNING ||
        detail?.status === RunningStatus.SCHEDULE
          ? false
          : true,
    });
  }, [detail, handleResume]);

  const customFields = useMemo(() => {
    return [
      {
        label: 'Refresh Freq',
        name: 'refresh_freq',
        type: FormFieldType.Number,
        required: false,
        render: (fieldProps: FormFieldConfig) => (
          <div className="flex items-center  gap-1 w-full relative">
            <Input {...fieldProps} type={FormFieldType.Number} />
            <span className="absolute right-0 -translate-x-[58px] text-text-secondary italic ">
              minutes
            </span>
            <button
              type="button"
              className="text-text-secondary bg-bg-input rounded-sm text-xs h-full p-2 border border-border-button "
              onClick={() => {
                runSchedule();
              }}
            >
              {detail?.status === RunningStatus.RUNNING ||
              detail?.status === RunningStatus.SCHEDULE ? (
                <CirclePause size={12} />
              ) : (
                <Repeat size={12} />
              )}
            </button>
          </div>
        ),
      },
      {
        label: 'Prune Freq',
        name: 'prune_freq',
        type: FormFieldType.Number,
        required: false,
        hidden: true,
        render: (fieldProps: FormFieldConfig) => {
          return (
            <div className="flex items-center  gap-1 w-full relative">
              <Input {...fieldProps} type={FormFieldType.Number} />
              <span className="absolute right-0 -translate-x-6 text-text-secondary italic ">
                hours
              </span>
            </div>
          );
        },
      },
      {
        label: 'Timeout Secs',
        name: 'timeout_secs',
        type: FormFieldType.Number,
        required: false,
        render: (fieldProps: FormFieldConfig) => (
          <div className="flex items-center  gap-1 w-full relative">
            <Input {...fieldProps} type={FormFieldType.Number} />
            <span className="absolute right-0 -translate-x-6 text-text-secondary italic ">
              seconds
            </span>
          </div>
        ),
      },
    ];
  }, [detail, runSchedule]);

  const { handleAddOk } = useAddDataSource();

  const onSubmit = useCallback(() => {
    formRef?.current?.submit();
  }, [formRef]);

  useEffect(() => {
    if (detail) {
      const fields = [
        ...DataSourceFormBaseFields,
        ...DataSourceFormFields[
          detail.source as keyof typeof DataSourceFormFields
        ],
        ...customFields,
      ] as FormFieldConfig[];

      const neweFields = fields.map((field) => {
        return {
          ...field,
          horizontal: true,
          onChange: () => {
            onSubmit();
          },
        };
      });
      setFields(neweFields);

      const defultValueTemp = {
        ...(DataSourceFormDefaultValues[
          detail?.source as keyof typeof DataSourceFormDefaultValues
        ] as FieldValues),
        ...detail,
      };
      console.log('defaultValue', defultValueTemp);
      setDefaultValues(defultValueTemp);
    }
  }, [detail, customFields, onSubmit]);

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
        <CardContent className="p-2 flex flex-col gap-2 max-h-[calc(100vh-190px)] overflow-y-auto scrollbar-auto">
          <div className="max-w-[1200px]">
            <DynamicForm.Root
              ref={formRef}
              fields={fields}
              onSubmit={debounce((data) => {
                handleAddOk(data);
              }, 500)}
              defaultValues={defaultValues}
            />
          </div>
          <section className="flex flex-col gap-2 mt-6">
            <div className="text-2xl text-text-primary">{t('setting.log')}</div>
            <DataSourceLogsTable refresh_freq={detail?.refresh_freq || false} />
          </section>
        </CardContent>
      </Card>
    </div>
  );
};
export default SourceDetailPage;
