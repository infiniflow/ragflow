import { Button } from '@/components/ui/button';
import { Form } from '@/components/ui/form';
import { ScrollArea } from '@/components/ui/scroll-area';
import { DatasetMetadata } from '@/constants/chat';
import { useSetModalState } from '@/hooks/common-hooks';
import { useFetchChat, useUpdateChat } from '@/hooks/use-chat-request';
import { useFindLlmByUuid } from '@/hooks/use-llm-request';
import {
  useRevalidateStaleDatasetIds,
  useStaleDatasetFormSchema,
} from '@/hooks/use-stale-dataset-validation';
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
import { ChatPromptEngine } from './chat-prompt-engine';
import { SavingButton } from './saving-button';
import { useChatSettingSchema } from './use-chat-setting-schema';
import { useRevealSubmitErrors } from './use-reveal-submit-errors';
import { getWebSearchProvider } from '../web-search-api-key';

type ChatSettingsProps = { hasSingleChatBox: boolean };

export function ChatSettings({ hasSingleChatBox }: ChatSettingsProps) {
  const { data } = useFetchChat();

  const chatSettingSchema = useChatSettingSchema();
  const { formSchema, datasetsFetched } = useStaleDatasetFormSchema(
    chatSettingSchema,
    data?.dataset_ids,
  );
  const { updateChat, loading } = useUpdateChat();
  const findLlmByUuid = useFindLlmByUuid();
  const { id } = useParams();
  const { t } = useTranslation();

  const { visible: settingVisible, switchVisible: switchSettingVisible } =
    useSetModalState(false);

  const {
    formContainerRef,
    handleInvalidSubmit,
    modelSettingOpen,
    onModelSettingOpenChange,
    advancedSettingOpen,
    onAdvancedSettingOpenChange,
  } = useRevealSubmitErrors();

  type FormSchemaType = z.infer<typeof formSchema>;

  const form = useForm<FormSchemaType>({
    resolver: zodResolver(formSchema),
    shouldUnregister: false,
    mode: 'onChange',
    defaultValues: {
      name: '',
      icon: '',
      description: '',
      dataset_ids: [],
      prompt_config: {
        quote: true,
        keyword: false,
        tts: false,
        refine_multiturn: true,
        system: '',
        parameters: [],
        reasoning: false,
        cross_languages: [],
        reference_metadata: {
          include: false,
          fields: undefined,
        },
      },
      top_n: 8,
      rerank_candidates_count: 64,
      similarity_threshold: 0.2,
      vector_similarity_weight: 0.2,
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
    const referenceMetadata = nextValues?.prompt_config?.reference_metadata;
    if (
      referenceMetadata &&
      Array.isArray(referenceMetadata.fields) &&
      referenceMetadata.fields.length === 0
    ) {
      referenceMetadata.fields = undefined;
    }

    // Add model_type to llm_setting based on the selected llm_id
    if (nextValues.llm_id) {
      nextValues.llm_setting = {
        ...nextValues.llm_setting,
        model_type: findLlmByUuid(nextValues.llm_id)?.model_type || 'chat',
      };
    }

    updateChat({
      chatId: id!,
      params: {
        ...omit(data, [
          'operator_permission',
          'tenant_id',
          'tenant_llm_id',
          'tenant_rerank_id',
          'created_by',
          'create_time',
          'create_date',
          'update_time',
          'update_date',
          'id',
          'top_k',
        ]),
        ...nextValues,
      },
    });
  }

  useEffect(() => {
    const llmSettingEnabledValues = setLLMSettingEnabledValues(
      data.llm_setting,
    );
    const referenceMetadata = data?.prompt_config?.reference_metadata;
    const normalizedReferenceMetadata =
      referenceMetadata &&
      Array.isArray(referenceMetadata.fields) &&
      referenceMetadata.fields.length === 0
        ? { ...referenceMetadata, fields: undefined }
        : referenceMetadata;

    const nextData = {
      ...omit(data, 'top_k'),
      prompt_config: {
        ...data.prompt_config,
        // reset() skips undefined values, so fall back to '' to clear the field
        web_search_provider: getWebSearchProvider(data.prompt_config) ?? '',
        reference_metadata: normalizedReferenceMetadata,
      },
      ...llmSettingEnabledValues,
    };

    if (!isEmpty(data)) {
      form.reset(nextData as FormSchemaType);
    }
  }, [data, form]);

  useRevalidateStaleDatasetIds(form, datasetsFetched);

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
                ref={formContainerRef}
                onSubmit={form.handleSubmit(onSubmit, handleInvalidSubmit)}
                className="flex-1 flex flex-col min-h-0"
              >
                <ScrollArea viewportClassName="[&>div]:!block">
                  <section className="p-5 space-y-6 overflow-auto flex-1 min-h-0">
                    <ChatBasicSetting
                      collapseOpen={modelSettingOpen}
                      onCollapseOpenChange={onModelSettingOpenChange}
                    ></ChatBasicSetting>
                    <ChatPromptEngine
                      collapseOpen={advancedSettingOpen}
                      onCollapseOpenChange={onAdvancedSettingOpenChange}
                    ></ChatPromptEngine>
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
