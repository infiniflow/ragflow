import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import {
  DynamicForm,
  DynamicFormRef,
  FormFieldConfig,
  FormFieldType,
} from '@/components/dynamic-form';
import { BlockButton, Button } from '@/components/ui/button';
import { Modal } from '@/components/ui/modal/modal';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { useSetModalState } from '@/hooks/common-hooks';
import { useFetchAgent } from '@/hooks/use-agent-request';
import { GlobalVariableType } from '@/interfaces/database/agent';
import { cn } from '@/lib/utils';
import { t } from 'i18next';
import { Trash2 } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { FieldValues } from 'react-hook-form';
import { useSaveGraph } from '../hooks/use-save-graph';
import {
  GobalFormFields,
  GobalVariableFormDefaultValues,
  TypeMaps,
  TypesWithArray,
} from './contant';

export type IGobalParamModalProps = {
  data: any;
  hideModal: (open: boolean) => void;
};
export const GobalParamSheet = (props: IGobalParamModalProps) => {
  const { hideModal } = props;
  const { data, refetch } = useFetchAgent();
  const [fields, setFields] = useState<FormFieldConfig[]>(GobalFormFields);
  const { visible, showModal, hideModal: hideAddModal } = useSetModalState();
  const [defaultValues, setDefaultValues] = useState<FieldValues>(
    GobalVariableFormDefaultValues,
  );
  const formRef = useRef<DynamicFormRef>(null);

  const handleFieldUpdate = (
    fieldName: string,
    updatedField: Partial<FormFieldConfig>,
  ) => {
    setFields((prevFields) =>
      prevFields.map((field) =>
        field.name === fieldName ? { ...field, ...updatedField } : field,
      ),
    );
  };

  useEffect(() => {
    const typefileld = fields.find((item) => item.name === 'type');

    if (typefileld) {
      typefileld.onChange = (value) => {
        // setWatchType(value);
        handleFieldUpdate('value', {
          type: TypeMaps[value as keyof typeof TypeMaps],
        });
        const values = formRef.current?.getValues();
        setTimeout(() => {
          switch (value) {
            case TypesWithArray.Boolean:
              setDefaultValues({ ...values, value: false });
              break;
            case TypesWithArray.Number:
              setDefaultValues({ ...values, value: 0 });
              break;
            default:
              setDefaultValues({ ...values, value: '' });
          }
        }, 0);
      };
    }
  }, [fields]);

  const { saveGraph, loading } = useSaveGraph();

  const handleSubmit = async (value: FieldValues) => {
    const param = {
      ...(data.dsl?.variables || {}),
      [value.name]: value,
    } as Record<string, GlobalVariableType>;

    const res = await saveGraph(undefined, {
      gobalVariables: param,
    });

    if (res.code === 0) {
      refetch();
    }
    hideAddModal();
  };

  const handleDeleteGobalVariable = async (key: string) => {
    const param = {
      ...(data.dsl?.variables || {}),
    } as Record<string, GlobalVariableType>;
    delete param[key];
    const res = await saveGraph(undefined, {
      gobalVariables: param,
    });
    console.log('delete gobal variable-->', res);
    if (res.code === 0) {
      refetch();
    }
  };

  const handleEditGobalVariable = (item: FieldValues) => {
    fields.forEach((field) => {
      if (field.name === 'value') {
        switch (item.type) {
          // [TypesWithArray.String]: FormFieldType.Textarea,
          // [TypesWithArray.Number]: FormFieldType.Number,
          // [TypesWithArray.Boolean]: FormFieldType.Checkbox,
          case TypesWithArray.Boolean:
            field.type = FormFieldType.Checkbox;
            break;
          case TypesWithArray.Number:
            field.type = FormFieldType.Number;
            break;
          default:
            field.type = FormFieldType.Textarea;
        }
      }
    });
    setDefaultValues(item);
    showModal();
  };
  return (
    <>
      <Sheet open onOpenChange={hideModal} modal={false}>
        <SheetContent
          className={cn('top-20 h-auto flex flex-col p-0 gap-0')}
          onInteractOutside={(e) => e.preventDefault()}
        >
          <SheetHeader className="p-5">
            <SheetTitle className="flex items-center gap-2.5">
              {t('flow.conversationVariable')}
            </SheetTitle>
          </SheetHeader>

          <div className="px-5 pb-5">
            <BlockButton
              onClick={() => {
                setFields(GobalFormFields);
                setDefaultValues(GobalVariableFormDefaultValues);
                showModal();
              }}
            >
              {t('flow.add')}
            </BlockButton>
          </div>

          <div className="flex flex-col gap-2 px-5 ">
            {data?.dsl?.variables &&
              Object.keys(data.dsl.variables).map((key) => {
                const item = data.dsl.variables[key];
                return (
                  <div
                    key={key}
                    className="flex items-center gap-3 min-h-14 justify-between px-5 py-3 border border-border-default rounded-lg  hover:bg-bg-card group"
                    onClick={() => {
                      handleEditGobalVariable(item);
                    }}
                  >
                    <div className="flex flex-col">
                      <div className="flex items-center gap-2">
                        <span className=" font-medium">{item.name}</span>
                        <span className="text-sm font-medium text-text-secondary">
                          {item.type}
                        </span>
                      </div>
                      <div>
                        <span className="text-text-primary">{item.value}</span>
                      </div>
                    </div>
                    <div>
                      <ConfirmDeleteDialog
                        onOk={() => handleDeleteGobalVariable(key)}
                      >
                        <Button
                          variant={'secondary'}
                          className="bg-transparent hidden text-text-secondary border-none group-hover:bg-bg-card group-hover:text-text-primary group-hover:border group-hover:block"
                          onClick={(e) => {
                            e.stopPropagation();
                          }}
                        >
                          <Trash2 className="w-4 h-4" />
                        </Button>
                      </ConfirmDeleteDialog>
                    </div>
                  </div>
                );
              })}
          </div>
        </SheetContent>
        <Modal
          title={t('flow.add') + t('flow.conversationVariable')}
          open={visible}
          onCancel={hideAddModal}
          showfooter={false}
        >
          <DynamicForm.Root
            ref={formRef}
            fields={fields}
            onSubmit={(data) => {
              console.log(data);
            }}
            defaultValues={defaultValues}
            onFieldUpdate={handleFieldUpdate}
          >
            <div className="flex items-center justify-end w-full gap-2">
              <DynamicForm.CancelButton
                handleCancel={() => {
                  hideAddModal?.();
                }}
              />
              <DynamicForm.SavingButton
                submitLoading={loading || false}
                buttonText={t('common.ok')}
                submitFunc={(values: FieldValues) => {
                  handleSubmit(values);
                  // console.log(values);
                  // console.log(nodes, edges);
                  //   handleOk(values);
                }}
              />
            </div>
          </DynamicForm.Root>
        </Modal>
      </Sheet>
    </>
  );
};
