import { RAGFlowFormItem } from '@/components/ragflow-form';
import { ButtonLoading } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Form } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { RAGFlowSelect } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { LLMFactory } from '@/constants/llm';
import { IModalProps } from '@/interfaces/common';
import { VerifyResult } from '@/pages/user-setting/setting-model/hooks';
import { buildOptions } from '@/utils/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { t } from 'i18next';
import { memo } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { LLMHeader } from '../../components/llm-header';
import VerifyButton from '../verify-button';

const FormSchema = z.object({
  llm_name: z.string().min(1, {
    message: t('setting.mineru.modelNameRequired'),
  }),
  mineru_access_mode: z.enum(['self_hosted', 'official_v4']),
  mineru_apiserver: z.string().url().optional(),
  mineru_output_dir: z.string().optional(),
  mineru_backend: z.enum([
    'pipeline',
    'vlm-transformers',
    'vlm-vllm-engine',
    'vlm-http-client',
    'vlm-mlx-engine',
    'vlm-vllm-async-engine',
    'vlm-lmdeploy-engine',
  ]),
  mineru_server_url: z.string().url().optional(),
  mineru_api_base_url: z.string().url().optional(),
  mineru_api_token: z.string().trim().optional(),
  mineru_model_version: z.enum(['pipeline', 'vlm', 'MinerU-HTML']).optional(),
  mineru_poll_interval: z.coerce.number().int().min(1).max(60).optional(),
  mineru_poll_timeout: z.coerce.number().int().min(30).max(3600).optional(),
  mineru_delete_output: z.boolean(),
}).superRefine((data, ctx) => {
  if (data.mineru_access_mode === 'self_hosted') {
    if (!data.mineru_apiserver) {
      ctx.addIssue({
        path: ['mineru_apiserver'],
        message: t('setting.mineru.apiServerRequired'),
        code: 'custom',
      });
    }
  } else {
    if (!data.mineru_api_base_url) {
      ctx.addIssue({
        path: ['mineru_api_base_url'],
        message: t('setting.mineru.officialBaseUrlRequired'),
        code: 'custom',
      });
    }
    if (!data.mineru_api_token) {
      ctx.addIssue({
        path: ['mineru_api_token'],
        message: t('setting.mineru.officialTokenRequired'),
        code: 'custom',
      });
    }
  }
});

export type MinerUFormValues = z.infer<typeof FormSchema>;

const MinerUModal = ({
  visible,
  hideModal,
  onOk,
  onVerify,
  loading,
}: IModalProps<MinerUFormValues> & {
  onVerify?: (
    postBody: any,
  ) => Promise<boolean | void | VerifyResult | undefined>;
}) => {
  const { t } = useTranslation();

  const backendOptions = buildOptions([
    'pipeline',
    'vlm-transformers',
    'vlm-vllm-engine',
    'vlm-http-client',
    'vlm-mlx-engine',
    'vlm-vllm-async-engine',
    'vlm-lmdeploy-engine',
  ]);
  const accessModeOptions = [
    {
      value: 'self_hosted',
      label: t('setting.mineru.accessModeSelfHosted'),
    },
    {
      value: 'official_v4',
      label: t('setting.mineru.accessModeOfficialV4'),
    },
  ];
  const modelVersionOptions = buildOptions(['pipeline', 'vlm', 'MinerU-HTML']);

  const form = useForm<MinerUFormValues>({
    resolver: zodResolver(FormSchema),
    defaultValues: {
      mineru_access_mode: 'self_hosted',
      mineru_backend: 'pipeline',
      mineru_model_version: 'vlm',
      mineru_api_base_url: 'https://mineru.net',
      mineru_poll_interval: 3,
      mineru_poll_timeout: 300,
      mineru_delete_output: true,
    },
  });

  const accessMode = useWatch({
    control: form.control,
    name: 'mineru_access_mode',
  });

  const backend = useWatch({
    control: form.control,
    name: 'mineru_backend',
  });

  const handleOk = async (values: MinerUFormValues) => {
    const ret = await onOk?.(values as any);
    if (ret) {
      hideModal?.();
    }
  };

  return (
    <Dialog open={visible} onOpenChange={hideModal}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            <LLMHeader name={LLMFactory.MinerU} />
          </DialogTitle>
        </DialogHeader>
        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(handleOk)}
            className="space-y-6"
            id="mineru-form"
          >
            <RAGFlowFormItem
              name="llm_name"
              label={t('setting.modelName')}
              required
            >
              <Input placeholder="mineru-from-env-1" />
            </RAGFlowFormItem>
            <RAGFlowFormItem
              name="mineru_access_mode"
              label={t('setting.mineru.accessMode')}
              required
            >
              {(field) => (
                <RAGFlowSelect
                  value={field.value}
                  onChange={(value) => {
                    field.onChange(value);
                    if (value !== 'self_hosted') {
                      form.setValue('mineru_server_url', undefined);
                    }
                  }}
                  options={accessModeOptions}
                  placeholder={t('setting.mineru.selectAccessMode')}
                />
              )}
            </RAGFlowFormItem>
            {accessMode === 'self_hosted' && (
              <>
                <RAGFlowFormItem
                  name="mineru_apiserver"
                  label={t('setting.mineru.apiserver')}
                  required
                >
                  <Input placeholder="http://host.docker.internal:9987" />
                </RAGFlowFormItem>
                <RAGFlowFormItem
                  name="mineru_output_dir"
                  label={t('setting.mineru.outputDir')}
                >
                  <Input placeholder="/tmp/mineru" />
                </RAGFlowFormItem>
                <RAGFlowFormItem
                  name="mineru_backend"
                  label={t('setting.mineru.backend')}
                >
                  {(field) => (
                    <RAGFlowSelect
                      value={field.value}
                      onChange={(value) => {
                        field.onChange(value);
                        if (value !== 'vlm-http-client') {
                          form.setValue('mineru_server_url', undefined);
                        }
                      }}
                      options={backendOptions}
                      placeholder={t('setting.mineru.selectBackend')}
                    />
                  )}
                </RAGFlowFormItem>
                {backend === 'vlm-http-client' && (
                  <RAGFlowFormItem
                    name="mineru_server_url"
                    label={t('setting.mineru.serverUrl')}
                  >
                    <Input placeholder="http://your-vllm-server:30000" />
                  </RAGFlowFormItem>
                )}
              </>
            )}
            {accessMode === 'official_v4' && (
              <>
                <RAGFlowFormItem
                  name="mineru_api_base_url"
                  label={t('setting.mineru.officialBaseUrl')}
                  required
                >
                  <Input placeholder="https://mineru.net" />
                </RAGFlowFormItem>
                <RAGFlowFormItem
                  name="mineru_api_token"
                  label={t('setting.mineru.officialToken')}
                  required
                >
                  <Input type="password" placeholder="eyJ0eXBlIjoiSldU..." />
                </RAGFlowFormItem>
                <RAGFlowFormItem
                  name="mineru_model_version"
                  label={t('setting.mineru.officialModelVersion')}
                >
                  {(field) => (
                    <RAGFlowSelect
                      value={field.value || 'vlm'}
                      onChange={field.onChange}
                      options={modelVersionOptions}
                      placeholder={t('setting.mineru.selectModelVersion')}
                    />
                  )}
                </RAGFlowFormItem>
                <RAGFlowFormItem
                  name="mineru_poll_interval"
                  label={t('setting.mineru.officialPollInterval')}
                >
                  <Input type="number" min={1} max={60} placeholder="3" />
                </RAGFlowFormItem>
                <RAGFlowFormItem
                  name="mineru_poll_timeout"
                  label={t('setting.mineru.officialPollTimeout')}
                >
                  <Input type="number" min={30} max={3600} placeholder="300" />
                </RAGFlowFormItem>
              </>
            )}
            <RAGFlowFormItem
              name="mineru_delete_output"
              label={t('setting.mineru.deleteOutput')}
              labelClassName="!mb-0"
            >
              {(field) => (
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              )}
            </RAGFlowFormItem>
            {onVerify && (
              <VerifyButton
                onVerify={onVerify as (postBody: any) => Promise<VerifyResult>}
              />
            )}
          </form>
        </Form>
        <DialogFooter>
          <div className="flex gap-2">
            <ButtonLoading type="submit" form="mineru-form" loading={loading}>
              {t('common.save', 'Save')}
            </ButtonLoading>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default memo(MinerUModal);
