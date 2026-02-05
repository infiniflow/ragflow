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

const AzureOpenAIModal = ({
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
  const { t: tg } = useCommonTranslation();
  const { buildModelTypeOptions } = useBuildModelTypeOptions();
  const formRef = useRef<DynamicFormRef>(null);

  const fields: FormFieldConfig[] = [
    {
      name: 'model_type',
      label: t('modelType'),
      type: FormFieldType.Select,
      required: true,
      options: buildModelTypeOptions(['chat', 'embedding', 'image2text']),
      defaultValue: 'embedding',
      validation: {
        message: t('modelTypeMessage'),
      },
    },
    {
      name: 'api_base',
      label: t('addLlmBaseUrl'),
      type: FormFieldType.Text,
      required: true,
      placeholder: t('baseUrlNameMessage'),
      validation: {
        message: t('baseUrlNameMessage'),
      },
    },
    {
      name: 'api_key',
      label: t('apiKey'),
      type: FormFieldType.Text,
      required: false,
      placeholder: t('apiKeyMessage'),
    },
    {
      name: 'llm_name',
      label: t('modelName'),
      type: FormFieldType.Text,
      required: true,
      placeholder: t('modelNameMessage'),
      defaultValue: 'gpt-3.5-turbo',
      validation: {
        message: t('modelNameMessage'),
      },
    },
    {
      name: 'api_version',
      label: t('apiVersion'),
      type: FormFieldType.Text,
      required: false,
      placeholder: t('apiVersionMessage'),
      defaultValue: '2024-02-01',
    },
    {
      name: 'max_tokens',
      label: t('maxTokens'),
      type: FormFieldType.Number,
      required: true,
      placeholder: t('maxTokensTip'),
      validation: {
        min: 0,
        message: t('maxTokensMessage'),
      },
    },
    {
      name: 'vision',
      label: t('vision'),
      type: FormFieldType.Switch,
      defaultValue: false,
      dependencies: ['model_type'],
      shouldRender: (formValues: any) => {
        return formValues?.model_type === 'chat';
      },
    },
  ];

  const handleOk = async (values?: FieldValues) => {
    if (!values) return;

    const modelType =
      values.model_type === 'chat' && values.vision
        ? 'image2text'
        : values.model_type;

    const data: IAddLlmRequestBody & { api_version?: string } = {
      llm_factory: llmFactory,
      llm_name: values.llm_name as string,
      model_type: modelType,
      api_base: values.api_base as string,
      api_key: values.api_key as string | undefined,
      max_tokens: values.max_tokens as number,
      api_version: values.api_version as string,
    };

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
            model_type: 'embedding',
            llm_name: 'gpt-3.5-turbo',
            api_version: '2024-02-01',
            vision: false,
          } as FieldValues
        }
        labelClassName="font-normal"
      >
        <>
          {onVerify && <VerifyButton onVerify={handleVerify} />}
          <div className="absolute bottom-0 right-0 left-0 flex items-center justify-end w-full gap-2 py-6 px-6">
            <DynamicForm.CancelButton
              handleCancel={() => {
                hideModal?.();
              }}
            />
            <DynamicForm.SavingButton
              submitLoading={loading || false}
              buttonText={tg('ok')}
              submitFunc={(values: FieldValues) => {
                handleOk(values);
              }}
            />
          </div>
        </>
      </DynamicForm.Root>
    </Modal>
  );
};

export default memo(AzureOpenAIModal);
