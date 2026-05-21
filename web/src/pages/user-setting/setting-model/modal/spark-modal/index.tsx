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
import omit from 'lodash/omit';
import { memo, useCallback, useRef } from 'react';
import { FieldValues } from 'react-hook-form';
import { LLMHeader } from '../../components/llm-header';
import VerifyButton from '../../modal/verify-button';

const SparkModal = ({
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
      options: buildModelTypeOptions(['chat', 'tts']),
      defaultValue: 'chat',
      validation: {
        message: t('modelTypeMessage'),
      },
    },
    {
      name: 'llm_name',
      label: t('modelName'),
      type: FormFieldType.Text,
      required: true,
      placeholder: t('modelNameMessage'),
      validation: {
        message: t('SparkModelNameMessage'),
      },
    },
    {
      name: 'spark_api_password',
      label: t('addSparkAPIPassword'),
      type: FormFieldType.Text,
      required: true,
      placeholder: t('SparkAPIPasswordMessage'),
      validation: {
        message: t('SparkAPIPasswordMessage'),
      },
    },
    {
      name: 'spark_app_id',
      label: t('addSparkAPPID'),
      type: FormFieldType.Text,
      required: true,
      placeholder: t('SparkAPPIDMessage'),
      validation: {
        message: t('SparkAPPIDMessage'),
      },
      dependencies: ['model_type'],
      shouldRender: (formValues: any) => {
        return formValues?.model_type === 'tts';
      },
    },
    {
      name: 'spark_api_secret',
      label: t('addSparkAPISecret'),
      type: FormFieldType.Text,
      required: true,
      placeholder: t('SparkAPISecretMessage'),
      validation: {
        message: t('SparkAPISecretMessage'),
      },
      dependencies: ['model_type'],
      shouldRender: (formValues: any) => {
        return formValues?.model_type === 'tts';
      },
    },
    {
      name: 'spark_api_key',
      label: t('addSparkAPIKey'),
      type: FormFieldType.Text,
      required: true,
      placeholder: t('SparkAPIKeyMessage'),
      validation: {
        message: t('SparkAPIKeyMessage'),
      },
      dependencies: ['model_type'],
      shouldRender: (formValues: any) => {
        return formValues?.model_type === 'tts';
      },
    },
    {
      name: 'max_tokens',
      label: t('maxTokens'),
      type: FormFieldType.Number,
      required: true,
      placeholder: t('maxTokensTip'),
      validation: {
        min: 0,
        message: t('maxTokensInvalidMessage'),
      },
    },
  ];

  const handleOk = async (values?: FieldValues) => {
    if (!values) return;

    const modelType =
      values.model_type === 'chat' && values.vision
        ? 'image2text'
        : values.model_type;

    const data = {
      ...omit(values, ['vision']),
      model_type: modelType,
      llm_factory: llmFactory,
      max_tokens: values.max_tokens,
    };

    await onOk?.(data as IAddLlmRequestBody);
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
        {onVerify && <VerifyButton onVerify={handleVerify} />}
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

export default memo(SparkModal);
