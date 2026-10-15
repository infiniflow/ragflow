'use client';

import { Collapse } from '@/components/collapse';
import { CrossLanguageFormField } from '@/components/cross-language-form-field';
import { MetadataFilter } from '@/components/metadata-filter';
import { RerankCandidatesCountFormField } from '@/components/rerank-candidates-count-item';
import { RerankFormFields } from '@/components/rerank';
import { SimilaritySliderFormField } from '@/components/similarity-slider';
import { SwitchFormField } from '@/components/switch-form-field';
import { TopNFormField } from '@/components/top-n-item';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Textarea } from '@/components/ui/textarea';
import { WebSearchFormField } from '@/components/web-search-form-field';
import { prefixName } from '@/utils/form';
import { getDirAttribute } from '@/utils/text-direction';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { DynamicVariableForm } from './dynamic-variable';

interface ChatPromptEngineProps {
  prefix?: string;
  collapseOpen?: boolean;
  onCollapseOpenChange?: (open: boolean) => void;
}

export function ChatPromptEngine({
  prefix = '',
  collapseOpen,
  onCollapseOpenChange,
}: ChatPromptEngineProps) {
  const { t } = useTranslation();
  const form = useFormContext();
  const systemPromptValue = form.watch(
    prefixName(prefix, 'prompt_config.system'),
  );

  const emptyResponseValue = form.watch(
    prefixName(prefix, 'prompt_config.empty_response'),
  );
  return (
    <Collapse
      title={t('flow.advancedSettings')}
      open={collapseOpen}
      onOpenChange={onCollapseOpenChange}
    >
      <div className="space-y-8">
        <FormField
          control={form.control}
          name={prefixName(prefix, 'prompt_config.empty_response')}
          render={({ field }) => (
            <FormItem>
              <FormLabel tooltip={t('chat.emptyResponseTip')}>
                {t('chat.emptyResponse')}
              </FormLabel>
              <FormControl>
                <Textarea
                  {...field}
                  placeholder={t('chat.emptyResponsePlaceholder')}
                  dir={getDirAttribute(emptyResponseValue || '')}
                ></Textarea>
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <SwitchFormField
          name={prefixName(prefix, 'prompt_config.quote')}
          label={t('chat.quote')}
          tooltip={t('chat.quoteTip')}
        ></SwitchFormField>
        <SwitchFormField
          name={prefixName(prefix, 'prompt_config.keyword')}
          label={t('chat.keyword')}
          tooltip={t('chat.keywordTip')}
        ></SwitchFormField>
        <SwitchFormField
          name={prefixName(prefix, 'prompt_config.tts')}
          label={t('chat.tts')}
          tooltip={t('chat.ttsTip')}
        ></SwitchFormField>
        <WebSearchFormField prefix={prefix} />
        <MetadataFilter></MetadataFilter>
        <FormField
          control={form.control}
          name={prefixName(prefix, 'prompt_config.system')}
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('chat.system')}</FormLabel>
              <FormControl>
                <Textarea
                  {...field}
                  rows={8}
                  placeholder={t('chat.systemPlaceholder')}
                  className="overflow-y-auto"
                  dir={getDirAttribute(systemPromptValue || '')}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <SimilaritySliderFormField
          isTooltipShown
          similarityName={prefixName(prefix, 'similarity_threshold')}
          similarityWeightName={prefixName(
            prefix,
            'keywords_similarity_weight',
          )}
        ></SimilaritySliderFormField>
        <RerankCandidatesCountFormField
          name={prefixName(prefix, 'rerank_candidates_count')}
        ></RerankCandidatesCountFormField>
        <TopNFormField name={prefixName(prefix, 'top_n')}></TopNFormField>

        <RerankFormFields prefix={prefix}></RerankFormFields>
        <CrossLanguageFormField
          name={prefixName(prefix, 'prompt_config.cross_languages')}
        ></CrossLanguageFormField>
        <SwitchFormField
          name={prefixName(prefix, 'prompt_config.refine_multiturn')}
          label={t('chat.multiTurn')}
          tooltip={t('chat.multiTurnTip')}
        ></SwitchFormField>
        <DynamicVariableForm
          name={prefixName(prefix, 'prompt_config.parameters')}
        ></DynamicVariableForm>
      </div>
    </Collapse>
  );
}
