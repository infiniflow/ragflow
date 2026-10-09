import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { LanguageFormField } from './common-form-fields';
import { CommonProps } from './interface';
import { useSetInitialLanguage } from './use-set-initial-language';
import { buildFieldNameWithPrefix } from './utils';

export function ImageFormFields({ prefix }: CommonProps) {
  const { t } = useTranslation();

  // Vision enhancement is a single global toggle shared by every vision-capable
  // file type (see VisionEnhancementFormFields); the per-file image setup only
  // carries the independent OCR switch. The enhancement model, language, and
  // prompt live at the form top level or the global setup, so lang/system_prompt
  // are shown when the global enhancement is on regardless of the OCR switch.
  const enableVisionEnhancement = useWatch({ name: 'enable_vision_enhancement' });

  useSetInitialLanguage({ prefix, languageShown: !!enableVisionEnhancement });

  return (
    <>
      <RAGFlowFormItem
        name={buildFieldNameWithPrefix('ocr_enabled', prefix)}
        label={t('flow.imageOcr')}
        tooltip={t('flow.imageOcrTip')}
        horizontal={true}
        labelClassName="w-full"
        valueClassName="w-8"
      >
        {(field) => (
          <Switch
            checked={!!field.value}
            onCheckedChange={(checked) => {
              field.onChange?.(checked);
            }}
          />
        )}
      </RAGFlowFormItem>
      {enableVisionEnhancement && (
        <LanguageFormField prefix={prefix}></LanguageFormField>
      )}
      {enableVisionEnhancement && (
        <RAGFlowFormItem
          name={buildFieldNameWithPrefix('system_prompt', prefix)}
          label={t('flow.systemPrompt')}
        >
          <Textarea placeholder={t('flow.systemPromptPlaceholder')} />
        </RAGFlowFormItem>
      )}
    </>
  );
}
