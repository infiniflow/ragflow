'use client';

import { AvatarNameDescription } from '@/components/avatar-name-description';
import { KnowledgeBaseFormField } from '@/components/knowledge-base-item';
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
import { getDirAttribute } from '@/utils/text-direction';
import { useFormContext, useWatch } from 'react-hook-form';

export default function ChatBasicSetting() {
  const { t } = useTranslate('chat');
  const form = useFormContext();

  const prologueValue = useWatch({
    control: form.control,
    name: 'prompt_config.prologue',
  });

  return (
    <div className="space-y-8">
      <AvatarNameDescription />
      <LlmSettingFieldItems
        prefix="llm_setting"
        llmId="llm_id"
        showCollapse
      ></LlmSettingFieldItems>

      <FormField
        control={form.control}
        name={'prompt_config.prologue'}
        render={({ field }) => (
          <FormItem>
            <FormLabel tooltip={t('setAnOpenerTip')}>
              {t('setAnOpener')}
            </FormLabel>
            <FormControl>
              <Textarea
                {...field}
                dir={getDirAttribute(prologueValue || '')}
              ></Textarea>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      <KnowledgeBaseFormField></KnowledgeBaseFormField>
    </div>
  );
}
