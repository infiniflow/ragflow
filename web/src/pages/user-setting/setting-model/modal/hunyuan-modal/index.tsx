import {
  DynamicForm,
  FormFieldConfig,
  FormFieldType,
} from '@/components/dynamic-form';
import { Modal } from '@/components/ui/modal/modal';
import { useCommonTranslation, useTranslate } from '@/hooks/common-hooks';
import { IModalProps } from '@/interfaces/common';
import { IAddLlmRequestBody } from '@/interfaces/request/llm';
import { FieldValues } from 'react-hook-form';
import { LLMHeader } from '../../components/llm-header';

const HunyuanModal = ({
  visible,
  hideModal,
  onOk,
  loading,
  llmFactory,
}: IModalProps<IAddLlmRequestBody> & { llmFactory: string }) => {
  const { t } = useTranslate('setting');
  const { t: tc } = useCommonTranslation();

  const fields: FormFieldConfig[] = [
    {
      name: 'hunyuan_sid',
      label: t('addHunyuanSID'),
      type: FormFieldType.Text,
      required: true,
      placeholder: t('HunyuanSIDMessage'),
      validation: {
        message: t('HunyuanSIDMessage'),
      },
    },
    {
      name: 'hunyuan_sk',
      label: t('addHunyuanSK'),
      type: FormFieldType.Text,
      required: true,
      placeholder: t('HunyuanSKMessage'),
      validation: {
        message: t('HunyuanSKMessage'),
      },
    },
  ];

  const handleOk = async (values?: FieldValues) => {
    if (!values) return;

    const data = {
      hunyuan_sid: values.hunyuan_sid as string,
      hunyuan_sk: values.hunyuan_sk as string,
      llm_factory: llmFactory,
    } as unknown as IAddLlmRequestBody;

    await onOk?.(data);
  };

  return (
    <Modal
      title={<LLMHeader name={llmFactory} />}
      open={visible || false}
      onOpenChange={(open) => !open && hideModal?.()}
      maskClosable={false}
      footer={<div className="p-4"></div>}
      className="max-w-[600px]"
    >
      <DynamicForm.Root
        fields={fields}
        onSubmit={() => {}}
        labelClassName="font-normal"
      >
        <div className="absolute bottom-0 right-0 left-0 flex items-center justify-end w-full gap-2 py-6 px-6">
          <DynamicForm.CancelButton
            handleCancel={() => {
              hideModal?.();
            }}
          />
          <DynamicForm.SavingButton
            submitLoading={loading || false}
            buttonText={tc('ok')}
            submitFunc={(values: FieldValues) => {
              handleOk(values);
            }}
          />
        </div>
      </DynamicForm.Root>
    </Modal>
  );
};

export default HunyuanModal;
