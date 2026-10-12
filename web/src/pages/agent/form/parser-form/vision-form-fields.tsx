import {
  ModelTreeSelectFormField,
  ModelTypeMap,
} from '@/components/model-tree-select';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import message from '@/components/ui/message';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { FileType } from '@/constants/file';
import { ModelTypeToField } from '@/constants/llm';
import { useFetchDefaultModelDictionary } from '@/hooks/use-llm-request';
import { useCallback } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useOwnerTenantId } from '../../context';
import { ParserFormSchemaType } from './schema';

type VisionEnhancementSwitchProps = {
  enabled: boolean | undefined;
  onEnabledChange: (value: boolean) => void;
};

// Turning the enhancement on prefills an empty model with the tenant's
// image2text default.
function VisionEnhancementSwitch({
  enabled,
  onEnabledChange,
}: VisionEnhancementSwitchProps) {
  const { t } = useTranslation();
  const form = useFormContext<ParserFormSchemaType>();
  const defaultModelDictionary = useFetchDefaultModelDictionary();

  const handleCheckedChange = useCallback(
    (checked: boolean) => {
      // Turning the enhancement off while an image setup runs without OCR
      // leaves the image with no text source, so keep it on and say why.
      if (!checked) {
        const setups = form.getValues('setups') ?? [];
        const imageOcrOff = setups.some(
          (setup) =>
            setup?.fileFormat === FileType.Image &&
            setup?.ocr_enabled === false,
        );
        if (imageOcrOff) {
          message.warning(t('flow.imageTextSourceRequired'));
          return;
        }
      }
      onEnabledChange(checked);
      if (checked && !form.getValues('vlm.llm_id')) {
        const modelId = defaultModelDictionary[ModelTypeToField.vision];
        if (modelId) {
          form.setValue('vlm.llm_id', modelId, { shouldDirty: true });
        }
      }
    },
    [onEnabledChange, form, defaultModelDictionary, t],
  );

  return <Switch checked={!!enabled} onCheckedChange={handleCheckedChange} />;
}

// Global vision enhancement: one switch + img2txt model + description prompt.
// The fields live at the form's top level (`enable_vision_enhancement` and
// `vlm.{llm_id, system_prompt}`) alongside `setups`, matching the backend Parser
// component's params contract. Response language is not offered here on purpose:
// it belongs to the knowledge base, which also drives tokenization.
export function VisionEnhancementFormFields() {
  const { t } = useTranslation();
  const ownerTenantId = useOwnerTenantId();
  const enabled = useWatch({ name: 'enable_vision_enhancement' });

  return (
    <>
      <RAGFlowFormItem
        name="enable_vision_enhancement"
        label={t('flow.enableVisionEnhancement')}
        tooltip={t('flow.enableVisionEnhancementTip')}
        horizontal={true}
        labelClassName="w-full"
        valueClassName="w-8"
      >
        {(field) => (
          <VisionEnhancementSwitch
            enabled={field.value}
            onEnabledChange={field.onChange}
          />
        )}
      </RAGFlowFormItem>
      {enabled && (
        <ModelTreeSelectFormField
          name="vlm.llm_id"
          label={t('chat.model')}
          modelTypes={ModelTypeMap.img2txt_id}
          allowClear
          ownerTenantId={ownerTenantId}
        />
      )}
      {enabled && (
        <RAGFlowFormItem
          name="vlm.system_prompt"
          label={t('flow.systemPrompt')}
          tooltip={t('flow.visionEnhancementPromptTip')}
        >
          <Textarea placeholder={t('flow.systemPromptPlaceholder')} />
        </RAGFlowFormItem>
      )}
    </>
  );
}
