import { MissingModelLabel } from '@/components/llm-select/llm-label';
import { buildModelTree } from '@/components/model-tree-select';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { TreeSelect } from '@/components/tree-select';
import message from '@/components/ui/message';
import { Switch } from '@/components/ui/switch';
import { useFetchAllAddedModels } from '@/hooks/use-llm-request';
import {
  ComponentPropsWithoutRef,
  forwardRef,
  useCallback,
  useMemo,
} from 'react';
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

const DeepDocModelId = '__image_ocr_deepdoc__';

// Keep the UI-only option out of the form and serialized parser configuration.
const ImageOCRModelSelect = forwardRef<
  HTMLButtonElement,
  ComponentPropsWithoutRef<typeof TreeSelect>
>(function ImageOCRModelSelect({ value, onChange, allowClear, ...props }, ref) {
  const handleModelChange = useCallback(
    (modelId: string) => {
      onChange?.(modelId === DeepDocModelId ? '' : modelId);
    },
    [onChange],
  );
  return (
    <TreeSelect
      {...props}
      ref={ref}
      value={value || DeepDocModelId}
      onChange={handleModelChange}
      allowClear={allowClear && !!value}
    />
  );
});

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

/** Lists enrolled PaddleOCR models using the canvas owner's tenant context. */
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
  const options = useMemo(
    () => [{ id: DeepDocModelId, title: t('flow.imageOcrLocal') }, ...models],
    [models, t],
  );
  const description = isError ? (
    <span role="alert" className="text-state-error">
      {t('flow.imageOcrModelsError')}
    </span>
  ) : isFetched && models.length === 0 ? (
    <span role="status">{t('flow.imageOcrNoModels')}</span>
  ) : undefined;

  return (
    <RAGFlowFormItem
      name={buildFieldNameWithPrefix('ocr_model_id', prefix)}
      label={t('flow.imageOcrModel')}
      tooltip={t('flow.imageOcrModelTip')}
      description={description}
    >
      {(field) => (
        <ImageOCRModelSelect
          ref={field.ref}
          data={options}
          value={field.value}
          onChange={field.onChange}
          placeholder={t('flow.imageOcrLocal')}
          loading={!isFetched}
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

/** Keeps optional OCR selection separate from the global VLM enhancement. */
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
