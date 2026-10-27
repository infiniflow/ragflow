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
    visible = true,
    hideModal,
  }: DialogProps & {
    chunk_num: number;
    handleOperationIconClick: (options?: {
      delete: boolean;
      apply_kb: boolean;
    }) => void;
    enable_metadata?: boolean;
    visible: boolean;
    hideModal: () => void;
  }) => {
    const [defaultValues, setDefaultValues] = useState<any>(null);
    const [fields, setFields] = useState<FormFieldConfig[]>([]);
    const { t } = useTranslation();

    useEffect(() => {
      // delete has no checkbox in this dialog; seed it so the payload keeps
      // dropping existing chunks (the backend skips re-parse when delete is
      // false and the previous task completed).
      setDefaultValues({
        delete: true,
        apply_kb: false,
      });
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
      if (enable_metadata) {
        setFields([applyKBField]);
      } else {
        setFields([]);
      }
    }, [t, enable_metadata]);

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
          delete: formValues.delete,
          apply_kb: formValues.apply_kb,
        });
      }
    }, [formCallbackRef, handleOperationIconClick]);

    return (
      <ConfirmDeleteDialog
        title={t(`knowledgeDetails.parseFile`)}
        onOk={() => handleSave()}
        onCancel={() => handleCancel()}
        open={visible}
        okButtonText={t('common.confirm')}
        content={{
          title: t(
            chunk_num > 0
              ? `knowledgeDetails.clearChunksReparseTip`
              : `knowledgeDetails.parseFileTip`,
          ),
          node: (
            <div>
              <DynamicForm.Root
                onSubmit={(data) => {
                  console.log('submit', data);
                }}
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
