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
import { RAGFlowSelect, RAGFlowSelectOptionType } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { useTranslate } from '@/hooks/common-hooks';
import { IModalProps } from '@/interfaces/common';
import { zodResolver } from '@hookform/resolvers/zod';
import { useEffect } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { LLMHeader } from '../../components/llm-header';

const FormSchema = z.object({
  llm_name: z.string().min(1, {
    message: 'Model name is required',
  }),
  mineru_apiserver: z.string().optional(),
  mineru_output_dir: z.string().optional(),
  mineru_backend: z.enum([
    'pipeline',
    'vlm-transformers',
    'vlm-vllm-engine',
    'vlm-http-client',
  ]),
  mineru_server_url: z.string().optional(),
  mineru_delete_output: z.boolean(),
});

type MinerUFormValues = z.infer<typeof FormSchema>;

const backendOptions: RAGFlowSelectOptionType[] = [
  { value: 'pipeline', label: 'pipeline' },
  { value: 'vlm-transformers', label: 'vlm-transformers' },
  { value: 'vlm-vllm-engine', label: 'vlm-vllm-engine' },
  { value: 'vlm-http-client', label: 'vlm-http-client' },
];

const MinerUModal = ({
  visible,
  hideModal,
  onOk,
  loading,
  initialValues,
}: IModalProps<MinerUFormValues> & {
  initialValues?: Partial<MinerUFormValues>;
}) => {
  const { t } = useTranslate('setting');

  const form = useForm<MinerUFormValues>({
    resolver: zodResolver(FormSchema),
    defaultValues: {
      mineru_backend: 'pipeline',
      mineru_delete_output: true,
    },
  });

  const handleOk = async (values: MinerUFormValues) => {
    const ret = await onOk?.(values as any);
    if (ret) {
      hideModal?.();
    }
  };

  useEffect(() => {
    if (visible) {
      form.reset();
      if (initialValues) {
        form.reset({
          mineru_backend: 'pipeline',
          mineru_delete_output: true,
          ...initialValues,
        });
      }
    }
  }, [visible, initialValues, form]);

  return (
    <Dialog open={visible} onOpenChange={hideModal}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            <LLMHeader name="MinerU" />
          </DialogTitle>
        </DialogHeader>
        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(handleOk)}
            className="space-y-6"
            id="mineru-form"
          >
            <RAGFlowFormItem name="llm_name" label={t('modelName')} required>
              <Input placeholder="mineru-from-env-1" />
            </RAGFlowFormItem>
            <RAGFlowFormItem name="mineru_apiserver" label="MINERU_APISERVER">
              <Input placeholder="http://host.docker.internal:9987" />
            </RAGFlowFormItem>
            <RAGFlowFormItem name="mineru_output_dir" label="MINERU_OUTPUT_DIR">
              <Input placeholder="/tmp/mineru" />
            </RAGFlowFormItem>
            <RAGFlowFormItem name="mineru_backend" label="MINERU_BACKEND">
              {(field) => (
                <RAGFlowSelect
                  value={field.value}
                  onChange={field.onChange}
                  options={backendOptions}
                  placeholder="Select backend"
                />
              )}
            </RAGFlowFormItem>
            <RAGFlowFormItem name="mineru_server_url" label="MINERU_SERVER_URL">
              <Input placeholder="http://your-vllm-server:30000" />
            </RAGFlowFormItem>
            <RAGFlowFormItem
              name="mineru_delete_output"
              label="MINERU_DELETE_OUTPUT"
              className="flex flex-row items-center justify-between rounded-lg border p-3 shadow-sm"
              labelClassName="!mb-0"
            >
              {(field) => (
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              )}
            </RAGFlowFormItem>
          </form>
        </Form>
        <DialogFooter>
          <ButtonLoading type="submit" form="mineru-form" loading={loading}>
            {t('common.save', 'Save')}
          </ButtonLoading>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default MinerUModal;
