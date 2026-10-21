'use client';

import { Button } from '@/components/ui/button';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Textarea } from '@/components/ui/textarea';
import { useFetchAgenticRagConfig } from '@/hooks/use-chat-request';
import { getDirAttribute } from '@/utils/text-direction';
import { useEffect } from 'react';
import { useFormContext, useFormState, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';

interface AgenticTemplateFormFieldProps {
  name: string;
}

export function AgenticTemplateFormField({
  name,
}: AgenticTemplateFormFieldProps) {
  const { t } = useTranslation();
  const { control, setValue } = useFormContext();
  const { id } = useParams();
  // Mount-gated: this component only renders when the settings panel is open
  // in agentic mode, which is exactly when the config is needed.
  const { data: config } = useFetchAgenticRagConfig(id);

  const value = useWatch({ control, name });
  const { isDirty } = useFormState({ control, name });
  const defaultContent = config?.defaults.content;

  // The template arrives after the form's own reset; fill it only while the
  // field is untouched and blank, so late data never overwrites a user edit.
  useEffect(() => {
    if (!config || isDirty || value?.trim()) return;
    setValue(name, config.override?.content ?? config.defaults.content);
  }, [config, isDirty, value, name, setValue]);

  const handleReset = () => {
    if (defaultContent === undefined) return;
    setValue(name, defaultContent, { shouldDirty: true, shouldValidate: true });
  };

  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <div className="flex items-center justify-between">
            <FormLabel tooltip={t('chat.agenticTemplateTip')}>
              {t('chat.agenticTemplate')}
            </FormLabel>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={handleReset}
              disabled={
                defaultContent === undefined || value === defaultContent
              }
            >
              {t('chat.resetToDefault')}
            </Button>
          </div>
          <FormControl>
            <Textarea
              {...field}
              value={field.value ?? ''}
              rows={10}
              className="overflow-y-auto"
              dir={getDirAttribute(value || '')}
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
