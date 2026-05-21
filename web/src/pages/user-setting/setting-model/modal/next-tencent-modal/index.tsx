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

const TencentCloudModal = ({
  visible,
  hideModal,
  onOk,
  onVerify,
  loading,
  llmFactory,
}: IModalProps<Omit<IAddLlmRequestBody, 'max_tokens'>> & {
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
      options: buildModelTypeOptions(['speech2text']),
      defaultValue: 'speech2text',
      validation: {
        message: t('modelTypeMessage'),
      },
    },
    {
      name: 'llm_name',
      label: t('modelName'),
      type: FormFieldType.Select,
      required: true,
      options: [
        { label: '16k_zh', value: '16k_zh' },
        { label: '16k_zh_large', value: '16k_zh_large' },
        { label: '16k_multi_lang', value: '16k_multi_lang' },
        { label: '16k_zh_dialect', value: '16k_zh_dialect' },
        { label: '16k_en', value: '16k_en' },
        { label: '16k_yue', value: '16k_yue' },
        { label: '16k_zh-PY', value: '16k_zh-PY' },
        { label: '16k_ja', value: '16k_ja' },
        { label: '16k_ko', value: '16k_ko' },
        { label: '16k_vi', value: '16k_vi' },
        { label: '16k_ms', value: '16k_ms' },
        { label: '16k_id', value: '16k_id' },
        { label: '16k_fil', value: '16k_fil' },
        { label: '16k_th', value: '16k_th' },
        { label: '16k_pt', value: '16k_pt' },
        { label: '16k_tr', value: '16k_tr' },
        { label: '16k_ar', value: '16k_ar' },
        { label: '16k_es', value: '16k_es' },
        { label: '16k_hi', value: '16k_hi' },
        { label: '16k_fr', value: '16k_fr' },
        { label: '16k_zh_medical', value: '16k_zh_medical' },
        { label: '16k_de', value: '16k_de' },
      ],
      defaultValue: '16k_zh',
      validation: {
        message: t('SparkModelNameMessage'),
      },
    },
    {
      name: 'TencentCloud_sid',
      label: t('addTencentCloudSID'),
      type: FormFieldType.Text,
      required: true,
      placeholder: t('TencentCloudSIDMessage'),
      validation: {
        message: t('TencentCloudSIDMessage'),
      },
    },
    {
      name: 'TencentCloud_sk',
      label: t('addTencentCloudSK'),
      type: FormFieldType.Text,
      required: true,
      placeholder: t('TencentCloudSKMessage'),
      validation: {
        message: t('TencentCloudSKMessage'),
      },
    },
  ];

  const handleOk = async (values?: FieldValues) => {
    if (!values) return;

    const modelType = values.model_type;

    const data = {
      model_type: modelType,
      llm_name: values.llm_name as string,
      TencentCloud_sid: values.TencentCloud_sid as string,
      TencentCloud_sk: values.TencentCloud_sk as string,
      llm_factory: llmFactory,
    } as Omit<IAddLlmRequestBody, 'max_tokens'>;

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
      footer={null}
    >
      <DynamicForm.Root
        fields={fields}
        onSubmit={() => {}}
        defaultValues={
          {
            model_type: 'speech2text',
            llm_name: '16k_zh',
          } as FieldValues
        }
        labelClassName="font-normal"
      >
        {onVerify && (
          <VerifyButton onVerify={handleVerify} isAbsolute={false} />
        )}
        <div className="absolute bottom-0 right-0 left-0 flex items-center justify-between w-full py-6 px-6">
          <a
            href="https://cloud.tencent.com/document/api/1093/37823"
            target="_blank"
            rel="noreferrer"
            className="text-primary hover:underline"
          >
            {t('TencentCloudLink')}
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

export default memo(TencentCloudModal);
