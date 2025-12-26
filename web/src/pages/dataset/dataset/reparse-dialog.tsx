import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import {
  DynamicForm,
  DynamicFormRef,
  FormFieldType,
} from '@/components/dynamic-form';
import { Checkbox } from '@/components/ui/checkbox';
import { DialogProps } from '@radix-ui/react-dialog';
import { t } from 'i18next';
import { useCallback, useState } from 'react';

export const ReparseDialog = ({
  handleOperationIconClick,
  chunk_num,
  hidden = false,
  visible = true,
  hideModal,
  children,
}: DialogProps & {
  chunk_num: number;
  handleOperationIconClick: (options: {
    delete: boolean;
    apply_kb: boolean;
  }) => void;
  visible: boolean;
  hideModal: () => void;
  hidden?: boolean;
}) => {
  const [formInstance, setFormInstance] = useState<DynamicFormRef | null>(null);

  const formCallbackRef = useCallback((node: DynamicFormRef | null) => {
    if (node) {
      setFormInstance(node);
      console.log('Form instance assigned:', node);
    } else {
      console.log('Form instance removed');
    }
  }, []);

  const handleCancel = useCallback(() => {
    // handleOperationIconClick(false);
    hideModal?.();
    formInstance?.reset();
  }, [formInstance]);

  const handleSave = useCallback(async () => {
    const instance = formInstance;
    if (!instance) {
      console.error('Form instance is null');
      return;
    }

    const check = await instance.trigger();
    if (check) {
      instance.submit();
      const formValues = instance.getValues();
      console.log(formValues);
      handleOperationIconClick({
        delete: formValues.delete,
        apply_kb: formValues.apply_kb,
      });
    }
  }, [formInstance, handleOperationIconClick]);

  //   useEffect(() => {
  //     if (!hidden) {
  //       const timer = setTimeout(() => {
  //         if (!formInstance) {
  //           console.warn(
  //             'Form ref is still null after component should be mounted',
  //           );
  //         } else {
  //           console.log('Form ref is properly set');
  //         }
  //       }, 1000);

  //       return () => clearTimeout(timer);
  //     }
  //   }, [hidden, formInstance]);

  return (
    <ConfirmDeleteDialog
      title={t(`knowledgeDetails.parseFile`)}
      onOk={() => handleSave()}
      onCancel={() => handleCancel()}
      hidden={hidden}
      open={visible}
      okButtonText={t('common.confirm')}
      content={{
        title: t(`knowledgeDetails.parseFileTip`),
        node: (
          <div>
            <DynamicForm.Root
              onSubmit={(data) => {
                console.log('submit', data);
              }}
              ref={formCallbackRef}
              fields={[
                {
                  name: 'delete',
                  label: '',
                  type: FormFieldType.Checkbox,
                  render: (fieldProps) => (
                    <div className="flex items-center text-text-secondary p-5 border border-border-button rounded-lg">
                      <Checkbox
                        {...fieldProps}
                        onCheckedChange={(checked: boolean) => {
                          fieldProps.onChange(checked);
                        }}
                      />
                      <span className="ml-2">
                        {chunk_num > 0
                          ? t(`knowledgeDetails.redo`, { chunkNum: chunk_num })
                          : t('knowledgeDetails.redoAll')}
                      </span>
                    </div>
                  ),
                },
                {
                  name: 'apply_kb',
                  label: '',
                  type: FormFieldType.Checkbox,
                  render: (fieldProps) => (
                    <div className="flex items-center text-text-secondary p-5 border border-border-button rounded-lg">
                      <Checkbox
                        {...fieldProps}
                        onCheckedChange={(checked: boolean) => {
                          fieldProps.onChange(checked);
                        }}
                      />
                      <span className="ml-2">
                        {t('knowledgeDetails.applyAutoMetadataSettings')}
                      </span>
                    </div>
                  ),
                },
              ]}
            >
              {/* <DynamicForm.CancelButton
                handleCancel={() => handleOperationIconClick(false)}
                cancelText={t('common.cancel')}
              />
              <DynamicForm.SavingButton
                buttonText={t('common.confirm')}
                submitFunc={handleSave}
              /> */}
            </DynamicForm.Root>
          </div>
        ),
      }}
    >
      {/* {children} */}
    </ConfirmDeleteDialog>
  );
};
