import { MissingModelLabel } from '@/components/llm-select/llm-label';
import { buildModelTree } from '@/components/model-tree-select';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { TreeSelect } from '@/components/tree-select';
import message from '@/components/ui/message';
import { Switch } from '@/components/ui/switch';
import { useFetchAllAddedModels } from '@/hooks/use-llm-request';
import { useCallback, useMemo } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useOwnerTenantId } from '../../context';
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

function ImageOCRModelField({ prefix }: CommonProps) {
  const { t } = useTranslation();
  const ownerTenantId = useOwnerTenantId();
  const { data, isFetched, isError } = useFetchAllAddedModels(
    'ocr',
    ownerTenantId,
  );
  const models = useMemo(
    () =>
      buildModelTree(
        data.filter(
          (model) =>
            model.model_id &&
            ['paddleocr', 'paddleocr.local'].includes(
              model.provider_name.toLowerCase(),
            ),
        ),
        ['ocr'],
      ),
    [data],
  );
  const renderMissingModel = useCallback(
    (value: string) => (
      <MissingModelLabel value={value} ownerTenantId={ownerTenantId} />
    ),
    [ownerTenantId],
  );

  return (
    <RAGFlowFormItem
      name={buildFieldNameWithPrefix('ocr_model_id', prefix)}
      label={t('flow.imageOcrModel')}
      tooltip={t('flow.imageOcrModelTip')}
    >
      {(field) => (
        <TreeSelect
          ref={field.ref}
          data={models}
          value={field.value}
          onChange={field.onChange}
          placeholder={t('flow.imageOcrLocal')}
          loading={!isFetched || isError}
          renderMissingValue={renderMissingModel}
          allowClear
          showSearch
          defaultExpandAll
          testId="image-ocr-model"
        />
      )}
    </RAGFlowFormItem>
  );
}

// OCR engine selection never grants permission for VLM descriptions: that model
// and prompt still belong exclusively to the global enhancement block.
export function ImageFormFields({ prefix }: CommonProps) {
  const { t } = useTranslation();
  const ocrEnabled = useWatch({
    name: buildFieldNameWithPrefix('ocr_enabled', prefix),
  });

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
          <ImageOcrSwitch
            checked={!!field.value}
            onCheckedChange={field.onChange}
          />
        )}
      </RAGFlowFormItem>
      {ocrEnabled && <ImageOCRModelField prefix={prefix} />}
    </>
  );
}
