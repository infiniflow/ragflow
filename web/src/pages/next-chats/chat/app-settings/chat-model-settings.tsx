import { LlmSettingFieldItems } from '@/components/llm-setting-items/next';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Textarea } from '@/components/ui/textarea';
import { useTranslate } from '@/hooks/common-hooks';
import { useFormContext } from 'react-hook-form';
import { Subhead } from './subhead';

export function ChatModelSettings() {
  const { t } = useTranslate('chat');
  const form = useFormContext();

  return (
    <section>
      <Subhead>Model Setting</Subhead>
      <div className="space-y-8">
        <FormField
          control={form.control}
          name="prompt_config.system"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('system')}</FormLabel>
              <FormControl>
                <Textarea
                  placeholder="Tell us a little bit about yourself"
                  className="resize-none"
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <LlmSettingFieldItems></LlmSettingFieldItems>
      </div>
    </section>
  );
}
