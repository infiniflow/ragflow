import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import {
  DynamicForm,
  DynamicFormRef,
  FormFieldConfig,
  FormFieldType,
} from '@/components/dynamic-form';
import { Checkbox } from '@/components/ui/checkbox';
import { DialogProps } from '@radix-ui/react-dialog';
import { memo, useCallback, useEffect, useRef, useState } from 'react';
import { ControllerRenderProps } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

export const ReparseDialog = memo(
  ({
    handleOperationIconClick,
    chunk_num,
    enable_metadata = false,
    alwaysClearChunks = false,
    visible = true,
    hideModal,
  }: DialogProps & {
    chunk_num: number;
    handleOperationIconClick: (options?: {
      delete: boolean;
      apply_kb: boolean;
    }) => void;
    enable_metadata?: boolean;
    // The Go ingestion pipeline always replaces a document's prior chunks on a
    // rerun; keeping them is not a supported outcome, so the dialog drops the
    // choice and submits `delete: true` whenever chunks exist.
    alwaysClearChunks?: boolean;
    visible: boolean;
    hideModal: () => void;
  }) => {
    const [defaultValues, setDefaultValues] = useState<any>(null);
    const [fields, setFields] = useState<FormFieldConfig[]>([]);
    const { t } = useTranslation();

    useEffect(() => {
      setDefaultValues({
        delete: chunk_num > 0,
        apply_kb: false,
      });
      const deleteField = {
        name: 'delete',
        label: '',
        type: FormFieldType.Checkbox,
        render: (fieldProps: ControllerRenderProps) => (
          <div className="flex items-center text-text-secondary p-5 border border-border-button rounded-lg">
            <Checkbox
              {...fieldProps}
              checked={fieldProps.value}
              onCheckedChange={(checked: boolean) => {
                fieldProps.onChange(checked);
              }}
            />
            <span className="ml-2">
              {chunk_num > 0
                ? t(`knowledgeDetails.redo`, {
                    chunkNum: chunk_num,
                  })
                : t('knowledgeDetails.redoAll')}
            </span>
          </div>
        ),
      };
      const applyKBField = {
        name: 'apply_kb',
        label: '',
        type: FormFieldType.Checkbox,
        defaultValue: false,
        render: (fieldProps: ControllerRenderProps) => (
          <div className="flex items-center text-text-secondary p-5 border border-border-button rounded-lg">
            <Checkbox
              {...fieldProps}
              checked={fieldProps.value}
              onCheckedChange={(checked: boolean) => {
                fieldProps.onChange(checked);
              }}
            />
            <span className="ml-2">
              {t('knowledgeDetails.applyAutoMetadataSettings')}
            </span>
          </div>
        ),
      };
      const nextFields: FormFieldConfig[] = [];
      if (chunk_num > 0 && !alwaysClearChunks) {
        nextFields.push(deleteField);
      }
      if (enable_metadata) {
        nextFields.push(applyKBField);
      }
      setFields(nextFields);
    }, [chunk_num, t, enable_metadata, alwaysClearChunks]);

    const formCallbackRef = useRef<DynamicFormRef>(null);

    const handleCancel = useCallback(() => {
      hideModal?.();
      formCallbackRef?.current?.reset();
    }, [formCallbackRef, hideModal]);

    const handleSave = useCallback(async () => {
      const instance = formCallbackRef?.current;
      if (!instance) {
        console.error('Form instance is null');
        return;
      }

      const check = await instance.trigger();
      if (check) {
        instance.submit();
        const formValues = instance.getValues();
        handleOperationIconClick({
          delete: alwaysClearChunks && chunk_num > 0 ? true : formValues.delete,
          apply_kb: formValues.apply_kb,
        });
      }
    }, [
      formCallbackRef,
      handleOperationIconClick,
      alwaysClearChunks,
      chunk_num,
    ]);

    return (
      <ConfirmDeleteDialog
        title={t(`knowledgeDetails.parseFile`)}
        onOk={() => handleSave()}
        onCancel={() => handleCancel()}
        open={visible}
        okButtonText={t('common.confirm')}
        content={{
          title: t(`knowledgeDetails.parseFileTip`),
          node: (
            <div>
              <DynamicForm.Root
                onSubmit={() => {}}
                ref={formCallbackRef}
                fields={fields}
                defaultValues={defaultValues}
              ></DynamicForm.Root>
            </div>
          ),
        }}
      ></ConfirmDeleteDialog>
    );
  },
);

ReparseDialog.displayName = 'ReparseDialog';
