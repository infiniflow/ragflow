import { Button, ButtonLoading } from '@/components/ui/button';
import { Form } from '@/components/ui/form';
import { Separator } from '@/components/ui/separator';
import { DatasetMetadata } from '@/constants/chat';
import { useFetchDialog, useSetDialog } from '@/hooks/use-chat-request';
import { transformBase64ToFile, transformFile2Base64 } from '@/utils/file-util';
import {
  removeUselessFieldsFromValues,
  setLLMSettingEnabledValues,
} from '@/utils/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { omit } from 'lodash';
import { X } from 'lucide-react';
import { useEffect } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useParams } from 'umi';
import { z } from 'zod';
import ChatBasicSetting from './chat-basic-settings';
import { ChatModelSettings } from './chat-model-settings';
import { ChatPromptEngine } from './chat-prompt-engine';
import { useChatSettingSchema } from './use-chat-setting-schema';

type ChatSettingsProps = { switchSettingVisible(): void };
export function ChatSettings({ switchSettingVisible }: ChatSettingsProps) {
  const formSchema = useChatSettingSchema();
  const { data } = useFetchDialog();
  const { setDialog, loading } = useSetDialog();
  const { id } = useParams();
  const { t } = useTranslation();

  type FormSchemaType = z.infer<typeof formSchema>;

  const form = useForm<FormSchemaType>({
    resolver: zodResolver(formSchema),
    shouldUnregister: true,
    defaultValues: {
      name: '',
      icon: [],
      language: 'English',
      description: '',
      kb_ids: [],
      prompt_config: {
        quote: true,
        keyword: false,
        tts: false,
        use_kg: false,
        refine_multiturn: true,
        system: '',
        parameters: [],
      },
      top_n: 8,
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
    const icon = nextValues.icon;
    const avatar =
      Array.isArray(icon) && icon.length > 0
        ? await transformFile2Base64(icon[0])
        : '';
    setDialog({
      ...omit(data, 'operator_permission'),
      ...nextValues,
      icon: avatar,
      dialog_id: id,
    });
  }

  function onInvalid(errors: any) {
    console.log('Form validation failed:', errors);
  }

  useEffect(() => {
    const llmSettingEnabledValues = setLLMSettingEnabledValues(
      data.llm_setting,
    );

    const nextData = {
      ...data,
      icon: data.icon ? [transformBase64ToFile(data.icon)] : [],
      ...llmSettingEnabledValues,
    };
    form.reset(nextData as FormSchemaType);
  }, [data, form]);

  return (
    <section className="p-5  w-[440px] border-l flex flex-col">
      <div className="flex justify-between items-center text-base pb-2">
        {t('chat.chatSetting')}
        <X className="size-4 cursor-pointer" onClick={switchSettingVisible} />
      </div>
      <Form {...form}>
        <form
          onSubmit={form.handleSubmit(onSubmit, onInvalid)}
          className="flex-1 flex flex-col min-h-0"
        >
          <section className="space-y-6 overflow-auto flex-1 pr-4 min-h-0">
            <ChatBasicSetting></ChatBasicSetting>
            <Separator />
            <ChatPromptEngine></ChatPromptEngine>
            <Separator />
            <ChatModelSettings></ChatModelSettings>
          </section>
          <div className="space-x-5 text-right pt-4">
            <Button variant={'outline'} onClick={switchSettingVisible}>
              {t('chat.cancel')}
            </Button>
            <ButtonLoading type="submit" loading={loading}>
              {t('common.save')}
            </ButtonLoading>
          </div>
        </form>
      </Form>
    </section>
  );
}
