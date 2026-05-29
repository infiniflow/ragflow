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

const YiyanModal = ({
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

  const fields = useMemo<FormFieldConfig[]>(
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
        options: buildModelTypeOptions(['chat', 'embedding', 'rerank']),
        defaultValue: ['chat'],
      },
      {
        name: 'llm_name',
        label: t('modelName'),
        type: FormFieldType.Text,
        required: true,
        placeholder: t('yiyanModelNameMessage'),
      },
      {
        name: 'yiyan_ak',
        label: t('addyiyanAK'),
        type: FormFieldType.Text,
        required: true,
        placeholder: t('yiyanAKMessage'),
        shouldRender: hideWhenInstanceExists,
      },
      {
        name: 'yiyan_sk',
        label: t('addyiyanSK'),
        type: FormFieldType.Text,
        required: true,
        placeholder: t('yiyanSKMessage'),
        shouldRender: hideWhenInstanceExists,
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
    ],
    [t, buildModelTypeOptions, hideWhenInstanceExists],
  );

  const handleOk = async (values?: FieldValues) => {
    if (!values) return;

    const data: IAddProviderInstanceRequestBody = {
      instance_name: values.instance_name as string,
      llm_factory: llmFactory,
      llm_name: values.llm_name as string,
      model_type: values.model_type,
      api_key: {
        yiyan_ak: values.yiyan_ak,
        yiyan_sk: values.yiyan_sk,
      },
      max_tokens: values.max_tokens as number,
    };

    await onOk?.(data);
  };

  const verifyParamsFunc = useCallback(() => {
    const values = formRef.current?.getValues();
    return {
      llm_factory: llmFactory,
      llm_name: values.llm_name as string,
      model_type: values.model_type,
      api_key: {
        yiyan_ak: values.yiyan_ak,
        yiyan_sk: values.yiyan_sk,
      },
      max_tokens: values.max_tokens as number,
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
      // footer={<div className="p-4"></div>}
      footer={<></>}
      footerClassName="pb-10"
    >
      <DynamicForm.Root
        key={`${visible}-${llmFactory}`}
        fields={fields}
        ref={formRef}
        onSubmit={(data) => {
          console.log(data);
        }}
        defaultValues={
          {
            instance_name: '',
            model_type: ['chat'],
            max_tokens: 8192,
          } as FieldValues
        }
        labelClassName="font-normal"
      >
        <div>
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
        </div>
      </DynamicForm.Root>
    </Modal>
  );
};

export default memo(YiyanModal);
