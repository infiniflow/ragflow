import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { DynamicForm, DynamicFormRef } from '@/components/dynamic-form';
import { DialogProps } from '@radix-ui/react-dialog';
import { memo, useCallback, useRef } from 'react';
import { useTranslation } from 'react-i18next';

export const ReparseDialog = memo(
  ({
    handleOperationIconClick,
    visible = true,
    hideModal,
  }: DialogProps & {
    handleOperationIconClick: (options?: {
      delete: boolean;
      apply_kb: boolean;
    }) => void;
    visible: boolean;
    hideModal: () => void;
  }) => {
    const { t } = useTranslation();

    // Existing chunks are always dropped on re-ingest, so the dialog only
    // needs a plain confirmation with no checkboxes. Seed delete: true so the
    // click payload keeps the drop behavior.
    const defaultValues = {
      delete: true,
      apply_kb: false,
    };

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
          title: t(`knowledgeDetails.clearChunksReparseTip`),
          node: (
            <div>
              <DynamicForm.Root
                onSubmit={(data) => {
                  console.log('submit', data);
                }}
                ref={formCallbackRef}
                fields={[]}
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
