import { Button } from '@/components/ui/button';
import { zodResolver } from '@hookform/resolvers/zod';
import { FormProvider, useForm } from 'react-hook-form';
import { z } from 'zod';
import ChatBasicSetting from './chat-basic-settings';
import { ChatModelSettings } from './chat-model-settings';
import { ChatPromptEngine } from './chat-prompt-engine';
import { useChatSettingSchema } from './use-chat-setting-schema';

export function AppSettings() {
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
    <section className="py-6 w-[500px] max-w-[25%] ">
      <div className="text-2xl font-bold mb-4 text-colors-text-neutral-strong px-6">
        App settings
      </div>
      <div className="overflow-auto max-h-[81vh] px-6 ">
        <FormProvider {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-6">
            <ChatBasicSetting></ChatBasicSetting>
            <ChatPromptEngine></ChatPromptEngine>
            <ChatModelSettings></ChatModelSettings>
          </form>
        </FormProvider>
      </div>
      <div className="p-6 text-center">
        <p className="text-colors-text-neutral-weak mb-1">
          There are unsaved changes
        </p>
        <Button variant={'tertiary'} className="w-full">
          Update
        </Button>
      </div>
    </section>
  );
}
