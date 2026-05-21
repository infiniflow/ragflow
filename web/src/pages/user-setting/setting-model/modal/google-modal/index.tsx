import {
  DynamicForm,
  FormFieldConfig,
  FormFieldType,
} from '@/components/dynamic-form';
import { Modal } from '@/components/ui/modal/modal';
import { useCommonTranslation, useTranslate } from '@/hooks/common-hooks';
import { useBuildModelTypeOptions } from '@/hooks/logic-hooks/use-build-options';
import { IModalProps } from '@/interfaces/common';
import { IAddLlmRequestBody } from '@/interfaces/request/llm';
import { VerifyResult } from '@/pages/user-setting/setting-model/hooks';
import { memo, useCallback } from 'react';
import { FieldValues } from 'react-hook-form';
import { LLMHeader } from '../../components/llm-header';
import VerifyButton from '../../modal/verify-button';

const GoogleModal = ({
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

  const fields: FormFieldConfig[] = [
    {
      name: 'model_type',
      label: t('modelType'),
      type: FormFieldType.Select,
      required: true,
      options: buildModelTypeOptions(['chat', 'image2text']),
      defaultValue: 'chat',
      validation: {
        message: t('modelTypeMessage'),
      },
    },
    {
      name: 'llm_name',
      label: t('modelID'),
      type: FormFieldType.Text,
      required: true,
      placeholder: t('GoogleModelIDMessage'),
      validation: {
        message: t('GoogleModelIDMessage'),
      },
    },
    {
      name: 'google_project_id',
      label: t('addGoogleProjectID'),
      type: FormFieldType.Text,
      required: true,
      placeholder: t('GoogleProjectIDMessage'),
      validation: {
        message: t('GoogleProjectIDMessage'),
      },
    },
    {
      name: 'google_region',
      label: t('addGoogleRegion'),
      type: FormFieldType.Text,
      required: true,
      placeholder: t('GoogleRegionMessage'),
      validation: {
        message: t('GoogleRegionMessage'),
      },
    },
    {
      name: 'google_service_account_key',
      label: t('addGoogleServiceAccountKey'),
      type: FormFieldType.Text,
      required: true,
      placeholder: t('GoogleServiceAccountKeyMessage'),
      validation: {
        message: t('GoogleServiceAccountKeyMessage'),
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
        message: t('maxTokensMinMessage'),
      },
      customValidate: (value: any) => {
        if (value === undefined || value === null || value === '') {
          return t('maxTokensMessage');
        }
        if (value < 0) {
          return t('maxTokensMinMessage');
        }
        return true;
      },
    },
  ];

  const handleOk = async (values?: FieldValues) => {
    if (!values) return;

    const data = {
      llm_factory: llmFactory,
      model_type: values.model_type,
      llm_name: values.llm_name,
      google_project_id: values.google_project_id,
      google_region: values.google_region,
      google_service_account_key: values.google_service_account_key,
      max_tokens: values.max_tokens,
    } as IAddLlmRequestBody;

    await onOk?.(data);
  };

  const verifyParamsFunc = useCallback(() => {
    return {
      llm_factory: llmFactory,
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
        onSubmit={() => {
          // Form submission is handled by SavingButton
        }}
        defaultValues={
          {
            model_type: 'chat',
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

export default memo(GoogleModal);
