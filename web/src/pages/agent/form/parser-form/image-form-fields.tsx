import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Switch } from '@/components/ui/switch';
import { useTranslation } from 'react-i18next';
import { CommonProps } from './interface';
import { buildFieldNameWithPrefix } from './utils';

// The image setup carries a single switch: run local OCR or not. The vision
// model, its response language and its description prompt all belong to the
// global enhancement block (see VisionEnhancementFormFields).
export function ImageFormFields({ prefix }: CommonProps) {
  const { t } = useTranslation();

  return (
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
  );
}
