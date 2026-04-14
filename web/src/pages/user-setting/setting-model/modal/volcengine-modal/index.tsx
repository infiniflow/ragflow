import {
  DynamicForm,
  DynamicFormRef,
  FormFieldConfig,
  FormFieldType,
} from '@/components/dynamic-form';
import { Modal } from '@/components/ui/modal/modal';
import { useCommonTranslation, useTranslate } from '@/hooks/common-hooks';
import { useBuildModelTypeOptions } from '@/hooks/logic-hooks/use-build-options';
import { IModalProps } from '@/interfaces/common';
import { IAddLlmRequestBody } from '@/interfaces/request/llm';
import { VerifyResult } from '@/pages/user-setting/setting-model/hooks';
import { memo, useCallback, useRef } from 'react';
import { FieldValues } from 'react-hook-form';
import { LLMHeader } from '../../components/llm-header';
import VerifyButton from '../../modal/verify-button';

type VolcEngineLlmRequest = IAddLlmRequestBody & {
  endpoint_id: string;
  ark_api_key: string;
};

const VolcEngineModal = ({
  visible,
  hideModal,
  onOk,
  onVerify,
  loading,
  llmFactory,
}: IModalProps<IAddLlmRequestBody> & {
  llmFactory: string;
  onVerify?: (
    postBody: any,
  ) => Promise<boolean | void | VerifyResult | undefined>;
}) => {
  const { t } = useTranslate('setting');
  const { t: tc } = useCommonTranslation();
  const { buildModelTypeOptions } = useBuildModelTypeOptions();
  const formRef = useRef<DynamicFormRef>(null);
  const fields: FormFieldConfig[] = [
    {
      name: 'model_type',
      label: t('modelType'),
      type: FormFieldType.Select,
      required: true,
      options: buildModelTypeOptions(['chat', 'embedding', 'image2text']),
      defaultValue: 'chat',
    },
    {
      name: 'llm_name',
      label: t('modelName'),
      type: FormFieldType.Text,
      required: true,
      placeholder: t('volcModelNameMessage'),
    },
    {
      name: 'endpoint_id',
      label: t('addEndpointID'),
      type: FormFieldType.Text,
      required: true,
      placeholder: t('endpointIDMessage'),
    },
    {
      name: 'ark_api_key',
      label: t('addArkApiKey'),
      type: FormFieldType.Text,
      required: true,
      placeholder: t('ArkApiKeyMessage'),
    },
    {
      name: 'max_tokens',
      label: t('maxTokens'),
      type: FormFieldType.Number,
      required: true,
      placeholder: t('maxTokensTip'),
      validation: {
        min: 0,
      },
    },
  ];

  const handleOk = async (values?: FieldValues) => {
    if (!values) return;

    const modelType =
      values.model_type === 'chat' && values.vision
        ? 'image2text'
        : values.model_type;

    const data: VolcEngineLlmRequest = {
      llm_factory: llmFactory,
      llm_name: values.llm_name as string,
      model_type: modelType,
      endpoint_id: values.endpoint_id as string,
      ark_api_key: values.ark_api_key as string,
      max_tokens: values.max_tokens as number,
    };

    console.info(data);

    await onOk?.(data);
  };

  const verifyParamsFunc = useCallback(() => {
    const values = formRef.current?.getValues();
    const modelType =
      values.model_type === 'chat' && values.vision
        ? 'image2text'
        : values.model_type;
    return {
      llm_factory: llmFactory,
      model_type: modelType,
    };
  }, [llmFactory]);

  const handleVerify = useCallback(
    async (params: any) => {
      const verifyParams = verifyParamsFunc();
      const res = await onVerify?.({ ...params, ...verifyParams });
      return (res || { isValid: null, logs: '' }) as VerifyResult;
    },
    [verifyParamsFunc, onVerify],
  );

  return (
    <Modal
      title={<LLMHeader name={llmFactory} />}
      open={visible || false}
      onOpenChange={(open) => !open && hideModal?.()}
      maskClosable={false}
      footer={<div className="p-4"></div>}
    >
      <DynamicForm.Root
        fields={fields}
        onSubmit={(data) => {
          console.log(data);
        }}
        ref={formRef}
        defaultValues={
          {
            model_type: 'chat',
            vision: false,
          } as FieldValues
        }
        labelClassName="font-normal"
      >
        {onVerify && (
          <VerifyButton onVerify={handleVerify} isAbsolute={false} />
        )}
        <div className="absolute bottom-0 right-0 left-0 flex items-center justify-between w-full py-6 px-6">
          <a
            href="https://www.volcengine.com/docs/82379/1302008"
            target="_blank"
            rel="noreferrer"
          >
            {t('ollamaLink', { name: llmFactory })}
          </a>
          <div className="flex gap-2">
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
        </div>
      </DynamicForm.Root>
    </Modal>
  );
};

export default memo(VolcEngineModal);
