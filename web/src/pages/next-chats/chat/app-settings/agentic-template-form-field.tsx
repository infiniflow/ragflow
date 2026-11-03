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
import { getDirAttribute } from '@/utils/text-direction';
import { useEffect } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

interface AgenticTemplateFormFieldProps {
  name: string;
  config?: {
    defaults: { content: string };
    override?: { content?: string } | null;
  };
}

export function AgenticTemplateFormField({
  name,
  config,
}: AgenticTemplateFormFieldProps) {
  const { t } = useTranslation();
  const { control, setValue } = useFormContext();

  const value = useWatch({ control, name });
  const defaultContent = config?.defaults.content;

  // The template arrives after the form's own reset; fill it only while the
  // field has no value. An empty string is an explicit edit, and the submit
  // path maps it back to the shipped template.
  useEffect(() => {
    if (!config || value != null) return;
    setValue(name, config.override?.content ?? config.defaults.content);
  }, [config, value, name, setValue]);

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
