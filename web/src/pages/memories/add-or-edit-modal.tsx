import { DynamicForm, DynamicFormRef } from '@/components/dynamic-form';
import { useModelOptions } from '@/components/llm-setting-items/llm-form-field';
import { HomeIcon } from '@/components/svg-icon';
import { Modal } from '@/components/ui/modal/modal';
import { t } from 'i18next';
import { memo, useCallback, useMemo, useState } from 'react';
import { createMemoryFields } from './constants';
import { IMemory } from './interface';

type IProps = {
  open: boolean;
  onClose: () => void;
  onSubmit?: (data: any) => void;
  initialMemory: IMemory;
  loading?: boolean;
  isCreate?: boolean;
};
export const AddOrEditModal = memo((props: IProps) => {
  const { open, onClose, onSubmit, initialMemory, isCreate } = props;
  const [formInstance, setFormInstance] = useState<DynamicFormRef | null>(null);

  const formCallbackRef = useCallback((node: DynamicFormRef | null) => {
    if (node) {
      // formRef.current = node;
      setFormInstance(node);
    }
  }, []);
  const { modelOptions } = useModelOptions();

  const fields = useMemo(() => {
    if (!isCreate) {
      return createMemoryFields.filter((field: any) => field.name === 'name');
    } else {
      const tempFields = createMemoryFields.map((field: any) => {
        if (field.name === 'llm_id') {
          return {
            ...field,
            options: modelOptions,
          };
        } else {
          return {
            ...field,
          };
        }
      });
      return tempFields;
    }
  }, [modelOptions, isCreate]);

  return (
    <Modal
      open={open}
      onOpenChange={onClose}
      className="!w-[480px]"
      title={
        <div className="flex flex-col">
          <div>
            <HomeIcon name="memory" width={'24'} />
          </div>
          {t('memories.createMemory')}
        </div>
      }
      showfooter={false}
      confirmLoading={props.loading}
    >
      <DynamicForm.Root
        ref={formCallbackRef}
        fields={fields}
        onSubmit={() => {}}
        defaultValues={initialMemory}
      >
        <div className="flex justify-end gap-2 pb-5">
          <DynamicForm.CancelButton handleCancel={onClose} />
          <DynamicForm.SavingButton
            submitLoading={false}
            submitFunc={(data) => {
              onSubmit?.(data);
            }}
          />
        </div>
      </DynamicForm.Root>
    </Modal>
  );
});
