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

import {
  DynamicForm,
  DynamicFormRef,
  FormFieldConfig,
} from '@/components/dynamic-form';
import { Button } from '@/components/ui/button';
import { Modal } from '@/components/ui/modal/modal';
import { IModalProps } from '@/interfaces/common';
import { useCallback, useMemo, useRef } from 'react';
import { FieldValues } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import {
  DataSourceFormDefaultValues,
  getCommonExtraDefaultValues,
  getDataSourceFormBaseFields,
  getDataSourceFieldsWithExtras,
  mergeDataSourceFormValues,
} from './constant';
import { useTestDataSource } from './hooks';
import { IDataSorceInfo } from './interface';

const AddDataSourceModal = ({
  visible,
  hideModal,
  loading,
  sourceData,
  onOk,
}: IModalProps<FieldValues> & { sourceData?: IDataSorceInfo }) => {
  const { t } = useTranslation();
  const formRef = useRef<DynamicFormRef>(null);
  const fields = useMemo<FormFieldConfig[]>(() => {
    if (!sourceData) {
      return [];
    }
    return [
      ...getDataSourceFormBaseFields(t),
      ...getDataSourceFieldsWithExtras(t, sourceData.id as any),
    ] as FormFieldConfig[];
  }, [sourceData, t]);
  const { loading: testLoading, handleTest } = useTestDataSource(
    formRef,
    sourceData?.id,
    fields,
  );

  const defaultValues = useMemo<FieldValues>(
    () =>
      mergeDataSourceFormValues(
        DataSourceFormDefaultValues[
          sourceData?.id as keyof typeof DataSourceFormDefaultValues
        ] as FieldValues,
        getCommonExtraDefaultValues(),
      ) as FieldValues,
    [sourceData],
  );

  const handleOk = useCallback(
    async (values?: FieldValues) => {
      const success = await onOk?.(values);
      if (success) {
        hideModal?.();
      }
    },
    [hideModal, onOk],
  );

  const handleCancel = useCallback(() => {
    hideModal?.();
  }, [hideModal]);

  const handleSubmit = useCallback(
    (values: FieldValues) => {
      handleOk(values);
    },
    [handleOk],
  );

  const handleFormSubmit = useCallback(() => {}, []);

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
        ref={formRef}
        fields={fields}
        onSubmit={handleFormSubmit}
        defaultValues={defaultValues}
        labelClassName="font-normal"
      >
        <div className=" absolute bottom-0 right-0 left-0 flex items-center justify-end w-full gap-2 py-6 px-6">
          <DynamicForm.CancelButton handleCancel={handleCancel} />
          <Button
            type="button"
            variant="outline"
            onClick={handleTest}
            disabled={testLoading}
            loading={testLoading}
          >
            {t('setting.dataSourceTestConnection')}
          </Button>
          <DynamicForm.SavingButton
            submitLoading={loading || false}
            buttonText={t('common.confirm')}
            submitFunc={handleSubmit}
          />
        </div>
      </DynamicForm.Root>
    </Modal>
  );
};

export default AddDataSourceModal;
