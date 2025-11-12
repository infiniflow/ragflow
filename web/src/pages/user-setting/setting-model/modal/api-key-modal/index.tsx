import { IModalManagerChildrenProps } from '@/components/modal-manager';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Modal } from '@/components/ui/modal/modal';
import { LLMFactory } from '@/constants/llm';
import { useTranslate } from '@/hooks/common-hooks';
import { KeyboardEventHandler, useCallback, useEffect } from 'react';
import { useForm } from 'react-hook-form';
import { ApiKeyPostBody } from '../../../interface';

interface IProps extends Omit<IModalManagerChildrenProps, 'showModal'> {
  loading: boolean;
  initialValue: string;
  llmFactory: string;
  editMode?: boolean;
  onOk: (postBody: ApiKeyPostBody) => void;
  showModal?(): void;
}

type FieldType = {
  api_key?: string;
  base_url?: string;
  group_id?: string;
};

const modelsWithBaseUrl = [
  LLMFactory.OpenAI,
  LLMFactory.AzureOpenAI,
  LLMFactory.TongYiQianWen,
];

const ApiKeyModal = ({
  visible,
  hideModal,
  llmFactory,
  loading,
  initialValue,
  editMode = false,
  onOk,
}: IProps) => {
  const form = useForm<FieldType>();
  const { t } = useTranslate('setting');

  const handleOk = useCallback(async () => {
    await form.handleSubmit((values) => onOk(values))();
  }, [form, onOk]);

  const handleKeyDown: KeyboardEventHandler<HTMLInputElement> = useCallback(
    async (e) => {
      if (e.key === 'Enter') {
        await handleOk();
      }
    },
    [handleOk],
  );

  useEffect(() => {
    if (visible) {
      form.setValue('api_key', initialValue);
    }
  }, [initialValue, form, visible]);

  return (
    <Modal
      title={t('configureModelTitle')}
      open={visible}
      onOpenChange={(open) => !open && hideModal()}
      onOk={handleOk}
      onCancel={hideModal}
      confirmLoading={loading}
      okText={t('save')}
      cancelText={t('cancel')}
      className="!w-[600px]"
    >
      <Form {...form}>
        <div className="space-y-4 py-4">
          <FormField
            name="api_key"
            rules={{ required: t('apiKeyMessage') }}
            render={({ field }) => (
              <FormItem>
                <FormLabel
                  className="text-sm font-medium text-text-secondary"
                  required
                >
                  {t('apiKey')}
                </FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    onKeyDown={handleKeyDown}
                    className="w-full"
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          {modelsWithBaseUrl.some((x) => x === llmFactory) && (
            <FormField
              name="base_url"
              render={({ field }) => (
                <FormItem>
                  <FormLabel className="text-sm font-medium text-text-primary">
                    {t('baseUrl')}
                  </FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      placeholder={
                        llmFactory === LLMFactory.TongYiQianWen
                          ? t('tongyiBaseUrlPlaceholder')
                          : 'https://api.openai.com/v1'
                      }
                      onKeyDown={handleKeyDown}
                      className="w-full"
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          )}

          {llmFactory?.toLowerCase() === 'Anthropic'.toLowerCase() && (
            <FormField
              name="base_url"
              render={({ field }) => (
                <FormItem>
                  <FormLabel className="text-sm font-medium text-text-primary">
                    {t('baseUrl')}
                  </FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      placeholder="https://api.anthropic.com/v1"
                      onKeyDown={handleKeyDown}
                      className="w-full"
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          )}

          {llmFactory?.toLowerCase() === 'Minimax'.toLowerCase() && (
            <FormField
              name="group_id"
              render={({ field }) => (
                <FormItem>
                  <FormLabel className="text-sm font-medium text-text-primary">
                    Group ID
                  </FormLabel>
                  <FormControl>
                    <Input {...field} className="w-full" />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          )}
        </div>
      </Form>
    </Modal>
  );
};

export default ApiKeyModal;
