import { ButtonLoading } from '@/components/ui/button';
import { Form } from '@/components/ui/form';
import { Separator } from '@/components/ui/separator';
import { useFetchDialog, useSetDialog } from '@/hooks/use-chat-request';
import { transformBase64ToFile, transformFile2Base64 } from '@/utils/file-util';
import {
  removeUselessFieldsFromValues,
  setLLMSettingEnabledValues,
} from '@/utils/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { X } from 'lucide-react';
import { useEffect } from 'react';
import { useForm } from 'react-hook-form';
import { useParams } from 'umi';
import { z } from 'zod';
import { DatasetMetadata } from '../../constants';
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

  type FormSchemaType = z.infer<typeof formSchema>;

  const form = useForm<FormSchemaType>({
    resolver: zodResolver(formSchema),
    shouldUnregister: true,
    defaultValues: {
      name: '',
      language: 'English',
      prompt_config: {
        quote: true,
        keyword: false,
        tts: false,
        use_kg: false,
        refine_multiturn: true,
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
      ...data,
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
    <section className="p-5  w-[440px] border-l">
      <div className="flex justify-between items-center text-base pb-2">
        Chat Settings
        <X className="size-4 cursor-pointer" onClick={switchSettingVisible} />
      </div>
      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit, onInvalid)}>
          <section className="space-y-6 overflow-auto max-h-[85vh] pr-4">
            <ChatBasicSetting></ChatBasicSetting>
            <Separator />
            <ChatPromptEngine></ChatPromptEngine>
            <Separator />
            <ChatModelSettings></ChatModelSettings>
          </section>
          <ButtonLoading
            className="w-full my-4"
            type="submit"
            loading={loading}
          >
            Update
          </ButtonLoading>
        </form>
      </Form>
    </section>
  );
}
