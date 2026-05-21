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
import { IAddProviderInstanceRequestBody } from '@/interfaces/request/llm';
import {
  useFetchInstanceNameSet,
  useHideWhenInstanceExists,
  VerifyResult,
} from '@/pages/user-setting/setting-model/hooks';
import { memo, useCallback, useMemo, useRef } from 'react';
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
}: IModalProps<IAddProviderInstanceRequestBody> & {
  llmFactory: string;
  onVerify?: (
    postBody: any,
  ) => Promise<boolean | void | VerifyResult | undefined>;
}) => {
  const { t } = useTranslate('setting');
  const { t: tc } = useCommonTranslation();
  const { buildModelTypeOptions } = useBuildModelTypeOptions();
  const formRef = useRef<DynamicFormRef>(null);
  const { instanceNameSet } = useFetchInstanceNameSet(llmFactory);

  const hideWhenInstanceExists = useHideWhenInstanceExists(instanceNameSet);

  const fields: FormFieldConfig[] = useMemo(
    () => [
      {
        name: 'instance_name',
        label: t('instanceName'),
        type: FormFieldType.Text,
        required: true,
        placeholder: t('instanceNameMessage'),
        tooltip: t('instanceNameTip'),
        validation: { message: t('instanceNameMessage') },
      },
      {
        name: 'model_type',
        label: t('modelType'),
        type: FormFieldType.MultiSelect,
        required: true,
        options: buildModelTypeOptions(['chat', 'tts']),
        defaultValue: ['chat'],
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
        shouldRender: hideWhenInstanceExists,
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
        dependencies: ['model_type', 'instance_name'],
        shouldRender: (formValues: any) => {
          if (!hideWhenInstanceExists(formValues)) return false;
          const modelType = formValues?.model_type;
          if (Array.isArray(modelType)) {
            return modelType.includes('tts');
          }
          return modelType === 'tts';
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
        dependencies: ['model_type', 'instance_name'],
        shouldRender: (formValues: any) => {
          if (!hideWhenInstanceExists(formValues)) return false;
          const modelType = formValues?.model_type;
          if (Array.isArray(modelType)) {
            return modelType.includes('tts');
          }
          return modelType === 'tts';
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
        dependencies: ['model_type', 'instance_name'],
        shouldRender: (formValues: any) => {
          if (!hideWhenInstanceExists(formValues)) return false;
          const modelType = formValues?.model_type;
          if (Array.isArray(modelType)) {
            return modelType.includes('tts');
          }
          return modelType === 'tts';
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
    ],
    [t, buildModelTypeOptions, hideWhenInstanceExists],
  );

  const handleOk = async (values?: FieldValues) => {
    if (!values) return;

    const data = {
      instance_name: values.instance_name as string,
      model_type: values.model_type,
      llm_factory: llmFactory,
      max_tokens: values.max_tokens,
    };

    await onOk?.(data as IAddProviderInstanceRequestBody);
  };

  const verifyParamsFunc = useCallback(() => {
    const values = formRef.current?.getValues();
    return {
      llm_factory: llmFactory,
      model_type: values.model_type,
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
            instance_name: '',
            model_type: ['chat'],
            max_tokens: 8192,
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
