import { DynamicForm } from '@/components/dynamic-form';
import { HomeIcon } from '@/components/svg-icon';
import { Modal } from '@/components/ui/modal/modal';
import { memo, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
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
  const { t } = useTranslation();
  // const { modelOptions } = useModelOptions();

  const fields = useMemo(() => {
    if (!isCreate) {
      return createMemoryFields(t).filter(
        (field: any) => field.name === 'name',
      );
    } else {
      // const tempFields = createMemoryFields(t).map((field: any) => {
      //   if (field.name === 'llm_id') {
      //     return {
      //       ...field,
      //       options: modelOptions,
      //     };
      //   } else {
      //     return {
      //       ...field,
      //     };
      //   }
      // });
      // return tempFields;
      return createMemoryFields(t);
    }
  }, [isCreate, t]);

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
          {isCreate ? t('memories.createMemory') : t('memories.editName')}
        </div>
      }
      showfooter={false}
      confirmLoading={props.loading}
    >
      <DynamicForm.Root
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
