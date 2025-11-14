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
} from './contant';
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
        <div className="flex flex-col">
          {sourceData?.icon}
          {t('setting.addDataSourceModalTital', { name: sourceData?.name })}
        </div>
      }
      open={visible || false}
      onOpenChange={(open) => !open && hideModal?.()}
      // onOk={() => handleOk()}
      okText={t('common.ok')}
      cancelText={t('common.cancel')}
      showfooter={false}
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
      >
        <div className="flex items-center justify-end w-full gap-2 py-4">
          <DynamicForm.CancelButton
            handleCancel={() => {
              hideModal?.();
            }}
          />
          <DynamicForm.SavingButton
            submitLoading={loading || false}
            buttonText={t('common.ok')}
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
