'use client';

import { RerankFormFields } from '@/components/rerank';
import { SimilaritySliderFormField } from '@/components/similarity-slider';
import { SwitchFormField } from '@/components/switch-fom-field';
import { TopNFormField } from '@/components/top-n-item';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Textarea } from '@/components/ui/textarea';
import { UseKnowledgeGraphFormField } from '@/components/use-knowledge-graph-item';
import { useTranslate } from '@/hooks/common-hooks';
import { useFormContext } from 'react-hook-form';
import { Subhead } from './subhead';

export function ChatPromptEngine() {
  const { t } = useTranslate('chat');
  const form = useFormContext();

  return (
    <section>
      <Subhead>Prompt Engine</Subhead>
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
        <SimilaritySliderFormField></SimilaritySliderFormField>
        <TopNFormField></TopNFormField>
        <SwitchFormField
          name={'prompt_config.refine_multiturn'}
          label={t('multiTurn')}
        ></SwitchFormField>
        <UseKnowledgeGraphFormField name="prompt_config.use_kg"></UseKnowledgeGraphFormField>
        <RerankFormFields></RerankFormFields>
      </div>
    </section>
  );
}
