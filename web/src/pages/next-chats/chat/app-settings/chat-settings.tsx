import { Button } from '@/components/ui/button';
import { Form } from '@/components/ui/form';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Separator } from '@/components/ui/separator';
import { DatasetMetadata } from '@/constants/chat';
import { useSetModalState } from '@/hooks/common-hooks';
import { useFetchChat, useUpdateChat } from '@/hooks/use-chat-request';
import { cn } from '@/lib/utils';
import {
  removeUselessFieldsFromValues,
  setLLMSettingEnabledValues,
} from '@/utils/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { isEmpty, omit } from 'lodash';
import { LucidePanelRightClose, LucideSettings } from 'lucide-react';
import { useEffect } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';
import { z } from 'zod';
import ChatBasicSetting from './chat-basic-settings';
import { ChatModelSettings } from './chat-model-settings';
import { ChatPromptEngine } from './chat-prompt-engine';
import { SavingButton } from './saving-button';
import { useChatSettingSchema } from './use-chat-setting-schema';

type ChatSettingsProps = { hasSingleChatBox: boolean };

export function ChatSettings({ hasSingleChatBox }: ChatSettingsProps) {
  const formSchema = useChatSettingSchema();
  const { data } = useFetchChat();
  const { updateChat, loading } = useUpdateChat();
  const { id } = useParams();
  const { t } = useTranslation();

  const { visible: settingVisible, switchVisible: switchSettingVisible } =
    useSetModalState(false);

  type FormSchemaType = z.infer<typeof formSchema>;

  const form = useForm<FormSchemaType>({
    resolver: zodResolver(formSchema),
    shouldUnregister: false,
    defaultValues: {
      name: '',
      icon: '',
      description: '',
      dataset_ids: [],
      prompt_config: {
        quote: true,
        keyword: false,
        include_document_metadata: true,
        tts: false,
        use_kg: false,
        refine_multiturn: true,
        system: '',
        parameters: [],
        reasoning: false,
        cross_languages: [],
        toc_enhance: false,
      },
      top_n: 8,
      similarity_threshold: 0.2,
      vector_similarity_weight: 0.2,
      top_k: 1024,
      meta_data_filter: {
        method: DatasetMetadata.Disabled,
        manual: [],
      },
    },
  });

  async function onSubmit(values: FormSchemaType) {
    const nextValues: Record<string, any> = removeUselessFieldsFromValues(
      values,
      'llm_setting.',
    );

    updateChat({
      chatId: id!,
      params: {
        ...omit(data, [
          'operator_permission',
          'tenant_id',
          'created_by',
          'create_time',
          'create_date',
          'update_time',
          'update_date',
          'id',
        ]),
        ...nextValues,
      },
    });
  }

  function onInvalid(errors: any) {
    void errors;
  }

  useEffect(() => {
    const llmSettingEnabledValues = setLLMSettingEnabledValues(
      data.llm_setting,
    );
    // Align with backend default (dialog_service / db_models): missing key means true.
    const nextData = {
      ...data,
      prompt_config: {
        include_document_metadata: true,
        ...(data.prompt_config ?? {}),
      },
      ...llmSettingEnabledValues,
    };

    if (!isEmpty(data)) {
      form.reset(nextData as FormSchemaType);
    }
  }, [data, form]);

  return (
    <>
      {settingVisible || (
        <div className="p-5">
          <Button
            onClick={switchSettingVisible}
            disabled={!hasSingleChatBox}
            variant={'ghost'}
            size="icon-sm"
            data-testid="chat-settings"
          >
            <LucideSettings />
          </Button>
        </div>
      )}

      <section
        data-testid="chat-detail-settings"
        className={cn(
          'transition-[width] ease-out duration-300 flex-shrink-0 flex flex-col overflow-hidden',
          settingVisible ? 'w-[440px]' : 'w-0',
        )}
      >
        {settingVisible && (
          <>
            <div className="p-5 pb-2 flex justify-between items-center text-base">
              {t('chat.chatSetting')}

              <Button
                variant="transparent"
                size="icon-sm"
                className="border-0"
                onClick={switchSettingVisible}
                data-testid="chat-detail-settings-close"
              >
                <LucidePanelRightClose
                  className="size-4 cursor-pointer"
                  onClick={switchSettingVisible}
                />
              </Button>
            </div>

            <Form {...form}>
              <form
                onSubmit={form.handleSubmit(onSubmit, onInvalid)}
                className="flex-1 flex flex-col min-h-0"
              >
                <ScrollArea viewportClassName="[&>div]:!block">
                  <section className="p-5 space-y-6 overflow-auto flex-1 min-h-0">
                    <ChatBasicSetting></ChatBasicSetting>
                    <Separator />
                    <ChatPromptEngine></ChatPromptEngine>
                    <Separator />
                    <ChatModelSettings></ChatModelSettings>
                  </section>
                </ScrollArea>

                <div className="p-5 pt-4 space-x-5 text-right">
                  <Button
                    variant={'outline'}
                    onClick={switchSettingVisible}
                    data-testid="chat-detail-settings-cancel"
                  >
                    {t('chat.cancel')}
                  </Button>
                  <SavingButton loading={loading}></SavingButton>
                </div>
              </form>
            </Form>
          </>
        )}
      </section>
    </>
  );
}
