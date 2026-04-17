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
  mineru_apiserver: z.string().url(),
  mineru_output_dir: z.string().optional(),
  mineru_backend: z.enum([
    'pipeline',
    'vlm-auto-engine',        // MinerU official: VLM auto engine
    'vlm-http-client',         // MinerU official: VLM HTTP client
    'hybrid-auto-engine',      // MinerU official: Hybrid auto engine (recommended)
    'hybrid-http-client',      // MinerU official: Hybrid HTTP client
    // Extended backends
    'vlm-transformers',
    'vlm-vllm-engine',
    'vlm-mlx-engine',
    'vlm-vllm-async-engine',
    'vlm-lmdeploy-engine',
  ]),
  mineru_server_url: z.string().url().optional(),
  mineru_delete_output: z.boolean(),
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
    'vlm-auto-engine',
    'vlm-http-client',
    'hybrid-auto-engine',
    'hybrid-http-client',
    'vlm-transformers',
    'vlm-vllm-engine',
    'vlm-mlx-engine',
    'vlm-vllm-async-engine',
    'vlm-lmdeploy-engine',
  ]);

  const form = useForm<MinerUFormValues>({
    resolver: zodResolver(FormSchema),
    defaultValues: {
      mineru_backend: 'hybrid-auto-engine',  // Recommended by MinerU
      mineru_delete_output: true,
    },
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
                    // Only vlm-http-client and hybrid-http-client need server_url
                    if (!value?.includes('http-client')) {
                      form.setValue('mineru_server_url', undefined);
                    }
                  }}
                  options={backendOptions}
                  placeholder={t('setting.mineru.selectBackend')}
                />
              )}
            </RAGFlowFormItem>
            {(backend === 'vlm-http-client' || backend === 'hybrid-http-client') && (
              <RAGFlowFormItem
                name="mineru_server_url"
                label={t('setting.mineru.serverUrl')}
              >
                <Input placeholder="http://your-vllm-server:30000" />
              </RAGFlowFormItem>
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
