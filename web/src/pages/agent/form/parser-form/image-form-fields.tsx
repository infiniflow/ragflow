import { RAGFlowFormItem } from '@/components/ragflow-form';
import message from '@/components/ui/message';
import { Switch } from '@/components/ui/switch';
import { useCallback } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { CommonProps } from './interface';
import { ParserFormSchemaType } from './schema';
import { buildFieldNameWithPrefix } from './utils';

type ImageOcrSwitchProps = {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
};

// OCR can only be turned off while the global vision enhancement is on: with
// neither, the image carries no text and the backend rejects the canvas.
function ImageOcrSwitch({ checked, onCheckedChange }: ImageOcrSwitchProps) {
  const { t } = useTranslation();
  const { control } = useFormContext<ParserFormSchemaType>();
  const visionEnabled = useWatch({
    control,
    name: 'enable_vision_enhancement',
  });

  const handleCheckedChange = useCallback(
    (next: boolean) => {
      if (!next && !visionEnabled) {
        message.warning(t('flow.imageTextSourceRequired'));
        return;
      }
      onCheckedChange(next);
    },
    [onCheckedChange, visionEnabled, t],
  );

  return <Switch checked={checked} onCheckedChange={handleCheckedChange} />;
}

// The image setup carries a single switch: run local OCR or not. The vision
// model and its description prompt belong to the global enhancement block (see
// VisionEnhancementFormFields); response language is not a setting anywhere and
// follows the knowledge base, which also drives tokenization.
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
        <ImageOcrSwitch
          checked={!!field.value}
          onCheckedChange={field.onChange}
        />
      )}
    </RAGFlowFormItem>
  );
}
