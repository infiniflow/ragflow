import { Button } from '@/components/ui/button';
import { Separator } from '@/components/ui/separator';
import { useFetchDialog } from '@/hooks/use-chat-request';
import { transformBase64ToFile } from '@/utils/file-util';
import { zodResolver } from '@hookform/resolvers/zod';
import { PanelRightClose } from 'lucide-react';
import { useEffect } from 'react';
import { FormProvider, useForm } from 'react-hook-form';
import { z } from 'zod';
import ChatBasicSetting from './chat-basic-settings';
import { ChatModelSettings } from './chat-model-settings';
import { ChatPromptEngine } from './chat-prompt-engine';
import { useChatSettingSchema } from './use-chat-setting-schema';

type ChatSettingsProps = { switchSettingVisible(): void };
export function ChatSettings({ switchSettingVisible }: ChatSettingsProps) {
  const formSchema = useChatSettingSchema();
  const { data } = useFetchDialog();

  const form = useForm<z.infer<typeof formSchema>>({
    resolver: zodResolver(formSchema),
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
    },
  });

  function onSubmit(values: z.infer<typeof formSchema>) {
    console.log(values);
  }

  useEffect(() => {
    const nextData = {
      ...data,
      icon: data.icon ? [transformBase64ToFile(data.icon)] : [],
    };
    form.reset(nextData as z.infer<typeof formSchema>);
  }, [data, form]);

  return (
    <section className="p-5  w-[400px] max-w-[20%]">
      <div className="flex justify-between items-center text-base">
        Chat Settings
        <PanelRightClose
          className="size-4 cursor-pointer"
          onClick={switchSettingVisible}
        />
      </div>
      <FormProvider {...form}>
        <form
          onSubmit={form.handleSubmit(onSubmit)}
          className="space-y-6 overflow-auto max-h-[87vh] pr-4"
        >
          <ChatBasicSetting></ChatBasicSetting>
          <Separator />
          <ChatPromptEngine></ChatPromptEngine>
          <Separator />
          <ChatModelSettings></ChatModelSettings>
        </form>
      </FormProvider>

      <Button className="w-full my-4">Update</Button>
    </section>
  );
}
