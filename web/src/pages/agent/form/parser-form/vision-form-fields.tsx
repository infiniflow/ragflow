import {
  ModelTreeSelectFormField,
  ModelTypeMap,
} from '@/components/model-tree-select';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Switch } from '@/components/ui/switch';
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
  const form = useFormContext<ParserFormSchemaType>();
  const defaultModelDictionary = useFetchDefaultModelDictionary();

  const handleCheckedChange = useCallback(
    (checked: boolean) => {
      onEnabledChange(checked);
      if (checked && !form.getValues('vlm.llm_id')) {
        const modelId = defaultModelDictionary[ModelTypeToField.vision];
        if (modelId) {
          form.setValue('vlm.llm_id', modelId, { shouldDirty: true });
        }
      }
    },
    [onEnabledChange, form, defaultModelDictionary],
  );

  return <Switch checked={!!enabled} onCheckedChange={handleCheckedChange} />;
}

// Global vision enhancement: one switch + img2txt model shared by every
// vision-capable file type. The fields live at the form's top level
// (`enable_vision_enhancement` and `vlm.llm_id`) alongside `setups`, matching
// the backend Parser component's params contract.
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
    </>
  );
}
