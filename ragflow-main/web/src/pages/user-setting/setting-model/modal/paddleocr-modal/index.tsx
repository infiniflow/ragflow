import { RAGFlowFormItem } from '@/components/ragflow-form';
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
import { LLMFactory } from '@/constants/llm';
import { VerifyResult } from '@/pages/user-setting/setting-model/hooks';
import { zodResolver } from '@hookform/resolvers/zod';
import { t } from 'i18next';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { LLMHeader } from '../../components/llm-header';
import VerifyButton from '../verify-button';

const FormSchema = z.object({
  llm_name: z.string().min(1, {
    message: t('setting.paddleocr.modelNameRequired'),
  }),
  paddleocr_api_url: z.string().min(1, {
    message: t('setting.paddleocr.apiUrlRequired'),
  }),
  paddleocr_access_token: z.string().optional(),
  paddleocr_algorithm: z.string().default('PaddleOCR-VL'),
});

export type PaddleOCRFormValues = z.infer<typeof FormSchema>;

export interface IModalProps<T> {
  visible: boolean;
  hideModal: () => void;
  onOk?: (data: T) => Promise<boolean>;
  onVerify?: (
    postBody: any,
  ) => Promise<boolean | void | VerifyResult | undefined>;
  loading?: boolean;
}

const algorithmOptions: RAGFlowSelectOptionType[] = [
  { label: 'PaddleOCR-VL', value: 'PaddleOCR-VL' },
];

const PaddleOCRModal = ({
  visible,
  hideModal,
  onOk,
  onVerify,
  loading,
}: IModalProps<PaddleOCRFormValues>) => {
  const { t } = useTranslation();

  const form = useForm<PaddleOCRFormValues>({
    resolver: zodResolver(FormSchema),
    defaultValues: {
      paddleocr_algorithm: 'PaddleOCR-VL',
    },
  });

  const handleOk = async (values: PaddleOCRFormValues) => {
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
            <LLMHeader name={LLMFactory.PaddleOCR} />
          </DialogTitle>
        </DialogHeader>
        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(handleOk)}
            className="space-y-6"
            id="paddleocr-form"
          >
            <RAGFlowFormItem
              name="llm_name"
              label={t('setting.modelName')}
              required
            >
              <Input
                placeholder={t('setting.paddleocr.modelNamePlaceholder')}
              />
            </RAGFlowFormItem>
            <RAGFlowFormItem
              name="paddleocr_api_url"
              label={t('setting.paddleocr.apiUrl')}
              required
            >
              <Input placeholder={t('setting.paddleocr.apiUrlPlaceholder')} />
            </RAGFlowFormItem>
            <RAGFlowFormItem
              name="paddleocr_access_token"
              label={t('setting.paddleocr.accessToken')}
            >
              <Input
                placeholder={t('setting.paddleocr.accessTokenPlaceholder')}
              />
            </RAGFlowFormItem>
            <RAGFlowFormItem
              name="paddleocr_algorithm"
              label={t('setting.paddleocr.algorithm')}
            >
              {(field) => (
                <RAGFlowSelect
                  value={field.value}
                  onChange={field.onChange}
                  options={algorithmOptions}
                  placeholder={t('setting.paddleocr.selectAlgorithm')}
                />
              )}
            </RAGFlowFormItem>
            {onVerify && (
              <VerifyButton
                onVerify={onVerify as (postBody: any) => Promise<VerifyResult>}
              />
            )}
            <DialogFooter>
              <div className="flex justify-end space-x-2">
                <button
                  type="button"
                  onClick={hideModal}
                  className="btn btn-secondary"
                >
                  {t('common.cancel')}
                </button>
                <button
                  type="submit"
                  disabled={loading}
                  className="btn btn-primary"
                >
                  {t('common.add')}
                </button>
              </div>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
};

export default memo(PaddleOCRModal);
