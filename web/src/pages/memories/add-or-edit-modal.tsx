import { DynamicForm, DynamicFormRef } from '@/components/dynamic-form';
import { useModelOptions } from '@/components/llm-setting-items/llm-form-field';
import { HomeIcon } from '@/components/svg-icon';
import { Modal } from '@/components/ui/modal/modal';
import { t } from 'i18next';
import { useCallback, useEffect, useState } from 'react';
import { createMemoryFields } from './constants';
import { IMemory } from './interface';

type IProps = {
  open: boolean;
  onClose: () => void;
  onSubmit?: (data: any) => void;
  initialMemory: IMemory;
  loading?: boolean;
};
export const AddOrEditModal = (props: IProps) => {
  const { open, onClose, onSubmit, initialMemory } = props;
  //   const [fields, setFields] = useState<FormFieldConfig[]>(createMemoryFields);
  //   const formRef = useRef<DynamicFormRef>(null);
  const [formInstance, setFormInstance] = useState<DynamicFormRef | null>(null);

  const formCallbackRef = useCallback((node: DynamicFormRef | null) => {
    if (node) {
      // formRef.current = node;
      setFormInstance(node);
    }
  }, []);
  const { modelOptions } = useModelOptions();

  useEffect(() => {
    if (initialMemory && initialMemory.id) {
      formInstance?.onFieldUpdate('memory_type', { hidden: true });
      formInstance?.onFieldUpdate('embedding', { hidden: true });
      formInstance?.onFieldUpdate('llm', { hidden: true });
    } else {
      formInstance?.onFieldUpdate('llm', { options: modelOptions as any });
    }
  }, [modelOptions, formInstance, initialMemory]);

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
          {t('memory.createMemory')}
        </div>
      }
      showfooter={false}
      confirmLoading={props.loading}
    >
      <DynamicForm.Root
        ref={formCallbackRef}
        fields={createMemoryFields}
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
};
