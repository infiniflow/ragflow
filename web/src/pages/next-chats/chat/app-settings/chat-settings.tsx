import { Button } from '@/components/ui/button';
import { zodResolver } from '@hookform/resolvers/zod';
import { FormProvider, useForm } from 'react-hook-form';
import { z } from 'zod';
import ChatBasicSetting from './chat-basic-settings';
import { ChatModelSettings } from './chat-model-settings';
import { ChatPromptEngine } from './chat-prompt-engine';
import { useChatSettingSchema } from './use-chat-setting-schema';

export function ChatSettings() {
  const formSchema = useChatSettingSchema();

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

  return (
    <section className="py-6">
      <FormProvider {...form}>
        <form
          onSubmit={form.handleSubmit(onSubmit)}
          className="space-y-6 overflow-auto max-h-[88vh] pr-4"
        >
          <ChatBasicSetting></ChatBasicSetting>
          <ChatPromptEngine></ChatPromptEngine>
          <ChatModelSettings></ChatModelSettings>
        </form>
      </FormProvider>

      <Button className="w-full my-4">Update</Button>
    </section>
  );
}
