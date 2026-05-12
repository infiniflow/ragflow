import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Button, ButtonLoading } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Form } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { LLMFactory } from '@/constants/llm';
import { VerifyResult } from '@/pages/user-setting/setting-model/hooks';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo, useMemo } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { LLMHeader } from '../../components/llm-header';
import VerifyButton from '../verify-button';

export type OpenDataLoaderFormValues = {
  llm_name: string;
  opendataloader_apiserver: string;
  opendataloader_api_key?: string;
};

export interface IModalProps<T> {
  visible: boolean;
  hideModal: () => void;
  onOk?: (data: T) => Promise<boolean>;
  onVerify?: (
    postBody: any,
  ) => Promise<boolean | void | VerifyResult | undefined>;
  loading?: boolean;
}

const OpenDataLoaderModal = ({
  visible,
  hideModal,
  onOk,
  onVerify,
  loading,
}: IModalProps<OpenDataLoaderFormValues>) => {
  const { t } = useTranslation();

  const FormSchema = useMemo(
    () =>
      z.object({
        llm_name: z.string().min(1, {
          message: t('setting.modelNameMessage'),
        }),
        opendataloader_apiserver: z.string().min(1, {
          message: t('setting.apiServerMessage'),
        }),
        opendataloader_api_key: z.string().optional(),
      }),
    [t],
  );

  const form = useForm<OpenDataLoaderFormValues>({
    resolver: zodResolver(FormSchema),
    defaultValues: {
      opendataloader_apiserver: '',
      opendataloader_api_key: '',
    },
  });

  const handleOk = async (values: OpenDataLoaderFormValues) => {
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
            <LLMHeader name={LLMFactory.OpenDataLoader} />
          </DialogTitle>
        </DialogHeader>
        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(handleOk)}
            className="space-y-6"
            id="opendataloader-form"
          >
            <RAGFlowFormItem
              name="llm_name"
              label={t('setting.modelName')}
              required
            >
              <Input placeholder="my-opendataloader" />
            </RAGFlowFormItem>
            <RAGFlowFormItem
              name="opendataloader_apiserver"
              label={t('setting.baseUrl')}
              required
            >
              <Input placeholder="http://your-opendataloader-service:9383" />
            </RAGFlowFormItem>
            <RAGFlowFormItem
              name="opendataloader_api_key"
              label={t('setting.apiKey')}
            >
              <Input
                type="password"
                placeholder={t('setting.apiKeyPlaceholder')}
              />
            </RAGFlowFormItem>
            {onVerify && (
              <VerifyButton
                onVerify={onVerify as (postBody: any) => Promise<VerifyResult>}
              />
            )}
          </form>
        </Form>
        <DialogFooter className="flex justify-end space-x-2">
          <Button type="button" variant="secondary" onClick={hideModal}>
            {t('common.cancel')}
          </Button>
          <ButtonLoading
            type="submit"
            form="opendataloader-form"
            loading={loading}
          >
            {t('common.add')}
          </ButtonLoading>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default memo(OpenDataLoaderModal);
