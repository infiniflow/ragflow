import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { ButtonLoading } from '@/components/ui/button';
import { Form } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Modal } from '@/components/ui/modal/modal';
import { Segmented } from '@/components/ui/segmented';
import { useCommonTranslation, useTranslate } from '@/hooks/common-hooks';
import { useBuildModelTypeOptions } from '@/hooks/logic-hooks/use-build-options';
import { IModalProps } from '@/interfaces/common';
import { IAddLlmRequestBody } from '@/interfaces/request/llm';
import { VerifyResult } from '@/pages/user-setting/setting-model/hooks';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo, useCallback, useMemo } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { z } from 'zod';
import { LLMHeader } from '../../components/llm-header';
import { BedrockRegionList } from '../../constant';
import VerifyButton from '../../modal/verify-button';

type FieldType = IAddLlmRequestBody & {
  auth_mode?: 'access_key_secret' | 'iam_role' | 'assume_role';
  bedrock_ak: string;
  bedrock_sk: string;
  bedrock_region: string;
  aws_role_arn?: string;
};

const BedrockModal = ({
  visible = false,
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
  const { t: ct } = useCommonTranslation();
  const { buildModelTypeOptions } = useBuildModelTypeOptions();

  const FormSchema = z
    .object({
      model_type: z.enum(['chat', 'embedding'], {
        required_error: t('modelTypeMessage'),
      }),
      llm_name: z.string().min(1, { message: t('bedrockModelNameMessage') }),
      bedrock_region: z.string().min(1, { message: t('bedrockRegionMessage') }),
      max_tokens: z
        .number({
          required_error: t('maxTokensMessage'),
          invalid_type_error: t('maxTokensInvalidMessage'),
        })
        .nonnegative({ message: t('maxTokensMinMessage') }),
      auth_mode: z
        .enum(['access_key_secret', 'iam_role', 'assume_role'])
        .default('access_key_secret'),
      bedrock_ak: z.string().optional(),
      bedrock_sk: z.string().optional(),
      aws_role_arn: z.string().optional(),
    })
    .superRefine((data, ctx) => {
      if (data.auth_mode === 'access_key_secret') {
        if (!data.bedrock_ak || data.bedrock_ak.trim() === '') {
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            message: t('bedrockAKMessage'),
            path: ['bedrock_ak'],
          });
        }
        if (!data.bedrock_sk || data.bedrock_sk.trim() === '') {
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            message: t('bedrockSKMessage'),
            path: ['bedrock_sk'],
          });
        }
      }

      if (data.auth_mode === 'iam_role') {
        if (!data.aws_role_arn || data.aws_role_arn.trim() === '') {
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            message: t('awsRoleArnMessage'),
            path: ['aws_role_arn'],
          });
        }
      }
    });

  const form = useForm<FieldType>({
    resolver: zodResolver(FormSchema),
    defaultValues: {
      model_type: 'chat',
      auth_mode: 'access_key_secret',
    },
  });

  const authMode = useWatch({
    control: form.control,
    name: 'auth_mode',
  });

  const options = useMemo(
    () => BedrockRegionList.map((x) => ({ value: x, label: t(x) })),
    [t],
  );

  const handleOk = async (values: FieldType) => {
    const cleanedValues: Record<string, any> = { ...values };

    const fieldsByMode: Record<string, string[]> = {
      access_key_secret: ['bedrock_ak', 'bedrock_sk'],
      iam_role: ['aws_role_arn'],
      assume_role: [],
    };

    cleanedValues.auth_mode = authMode;

    Object.keys(fieldsByMode).forEach((mode) => {
      if (mode !== authMode) {
        fieldsByMode[mode].forEach((field) => {
          delete cleanedValues[field];
        });
      }
    });

    const data = {
      ...cleanedValues,
      llm_factory: llmFactory,
      max_tokens: values.max_tokens,
    };

    onOk?.(data as unknown as IAddLlmRequestBody);
  };

  const verifyParamsFunc = useCallback(() => {
    const values = form.getValues();
    const cleanedValues: Record<string, any> = { ...values };
    const fieldsByMode: Record<string, string[]> = {
      access_key_secret: ['bedrock_ak', 'bedrock_sk'],
      iam_role: ['aws_role_arn'],
      assume_role: [],
    };

    cleanedValues.auth_mode = authMode;

    Object.keys(fieldsByMode).forEach((mode) => {
      if (mode !== authMode) {
        fieldsByMode[mode].forEach((field) => {
          delete cleanedValues[field];
        });
      }
    });
    return {
      ...cleanedValues,
      llm_factory: llmFactory,
      max_tokens: values.max_tokens,
    };
  }, [llmFactory, authMode, form]);

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
      open={visible}
      onOpenChange={(open) => !open && hideModal?.()}
      maskClosable={false}
      footer={
        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={hideModal}
            className="px-2 py-1 border border-border-button rounded-md hover:bg-bg-card"
          >
            {t('cancel')}
          </button>
          <ButtonLoading type="submit" form="bedrock-form" loading={loading}>
            {ct('ok')}
          </ButtonLoading>
        </div>
      }
    >
      <Form {...form}>
        <form
          onSubmit={form.handleSubmit(handleOk)}
          className="space-y-6"
          id="bedrock-form"
        >
          <RAGFlowFormItem name="model_type" label={t('modelType')} required>
            {(field) => (
              <SelectWithSearch
                value={field.value}
                onChange={field.onChange}
                options={buildModelTypeOptions(['chat', 'embedding'])}
                placeholder={t('modelTypeMessage')}
              />
            )}
          </RAGFlowFormItem>

          <RAGFlowFormItem name="llm_name" label={t('modelName')} required>
            <Input placeholder={t('bedrockModelNameMessage')} />
          </RAGFlowFormItem>

          <div className="mb-4">
            <RAGFlowFormItem name="auth_mode">
              {(field) => (
                <Segmented
                  value={field.value}
                  onChange={(value) => {
                    // Clear non-active fields so they won't be validated/submitted by accident.
                    if (value !== 'access_key_secret') {
                      form.setValue('bedrock_ak', '');
                      form.setValue('bedrock_sk', '');
                    }
                    if (value !== 'iam_role') {
                      form.setValue('aws_role_arn', '');
                    }
                    field.onChange(value);
                  }}
                  options={[
                    {
                      label: t('awsAuthModeAccessKeySecret'),
                      value: 'access_key_secret',
                    },
                    { label: t('awsAuthModeIamRole'), value: 'iam_role' },
                    { label: t('awsAuthModeAssumeRole'), value: 'assume_role' },
                  ]}
                />
              )}
            </RAGFlowFormItem>
          </div>

          {authMode === 'access_key_secret' && (
            <>
              <RAGFlowFormItem
                name="bedrock_ak"
                label={t('awsAccessKeyId')}
                required
              >
                <Input placeholder={t('bedrockAKMessage')} />
              </RAGFlowFormItem>
              <RAGFlowFormItem
                name="bedrock_sk"
                label={t('awsSecretAccessKey')}
                required
              >
                <Input placeholder={t('bedrockSKMessage')} />
              </RAGFlowFormItem>
            </>
          )}

          {authMode === 'iam_role' && (
            <RAGFlowFormItem
              name="aws_role_arn"
              label={t('awsRoleArn')}
              required
            >
              <Input placeholder={t('awsRoleArnMessage')} />
            </RAGFlowFormItem>
          )}

          {authMode === 'assume_role' && (
            <div className="text-sm text-text-secondary mt-2 mb-4">
              {t('awsAssumeRoleTip')}
            </div>
          )}

          <RAGFlowFormItem
            name="bedrock_region"
            label={t('bedrockRegion')}
            required
          >
            {(field) => (
              <SelectWithSearch
                value={field.value}
                onChange={field.onChange}
                options={options}
                placeholder={t('bedrockRegionMessage')}
                allowClear
              />
            )}
          </RAGFlowFormItem>

          <RAGFlowFormItem name="max_tokens" label={t('maxTokens')} required>
            {(field) => (
              <Input
                type="number"
                placeholder={t('maxTokensTip')}
                value={field.value}
                onChange={(e) => field.onChange(Number(e.target.value))}
              />
            )}
          </RAGFlowFormItem>
          {onVerify && <VerifyButton onVerify={handleVerify} />}
        </form>
      </Form>
    </Modal>
  );
};

export default memo(BedrockModal);
