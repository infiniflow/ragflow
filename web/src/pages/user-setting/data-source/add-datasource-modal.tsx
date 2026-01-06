import { DynamicForm, FormFieldConfig } from '@/components/dynamic-form';
import { Modal } from '@/components/ui/modal/modal';
import { IModalProps } from '@/interfaces/common';
import { useEffect, useState } from 'react';
import { FieldValues } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import {
  DataSourceFormBaseFields,
  DataSourceFormDefaultValues,
  DataSourceFormFields,
} from './constant';
import { IDataSorceInfo } from './interface';

const AddDataSourceModal = ({
  visible,
  hideModal,
  loading,
  sourceData,
  onOk,
}: IModalProps<FieldValues> & { sourceData?: IDataSorceInfo }) => {
  const { t } = useTranslation();
  const [fields, setFields] = useState<FormFieldConfig[]>([]);

  useEffect(() => {
    if (sourceData) {
      setFields([
        ...DataSourceFormBaseFields,
        ...DataSourceFormFields[
          sourceData.id as keyof typeof DataSourceFormFields
        ],
      ] as FormFieldConfig[]);
    }
  }, [sourceData]);

  const handleOk = async (values?: FieldValues) => {
    await onOk?.(values);
    hideModal?.();
  };

  return (
    <Modal
      title={
        <div className="flex flex-col gap-4">
          <div className="size-6">{sourceData?.icon}</div>
          {t('setting.addDataSourceModalTitle', { name: sourceData?.name })}
        </div>
      }
      open={visible || false}
      onOpenChange={(open) => !open && hideModal?.()}
      maskClosable={false}
      // onOk={() => handleOk()}
      okText={t('common.confirm')}
      cancelText={t('common.cancel')}
      footer={<div className="p-4"></div>}
    >
      <DynamicForm.Root
        fields={fields}
        onSubmit={(data) => {
          console.log(data);
        }}
        defaultValues={
          DataSourceFormDefaultValues[
            sourceData?.id as keyof typeof DataSourceFormDefaultValues
          ] as FieldValues
        }
        labelClassName="font-normal"
      >
        <div className=" absolute bottom-0 right-0 left-0 flex items-center justify-end w-full gap-2 py-6 px-6">
          <DynamicForm.CancelButton
            handleCancel={() => {
              hideModal?.();
            }}
          />
          <DynamicForm.SavingButton
            submitLoading={loading || false}
            buttonText={t('common.confirm')}
            submitFunc={(values: FieldValues) => {
              handleOk(values);
            }}
          />
        </div>
      </DynamicForm.Root>
    </Modal>
  );
};

export default AddDataSourceModal;
