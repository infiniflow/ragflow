import { MissingModelLabel } from '@/components/llm-select/llm-label';
import { useModelTree } from '@/components/model-tree-select';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { TreeSelect } from '@/components/tree-select';
import message from '@/components/ui/message';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import { Switch } from '@/components/ui/switch';
import { cn } from '@/lib/utils';
import { Cloud, LucideIcon, ScanText } from 'lucide-react';
import { useCallback, useId, useMemo, useState } from 'react';
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

// The two OCR engines are a UI grouping over the backend contract: DeepDoc is
// an empty ocr_model_id, External is any enrolled PaddleOCR model ID.
const enum ImageOcrEngine {
  DeepDoc = 'deepdoc',
  External = 'external',
}

// Stable references: useModelTree keys its memoized tree on their identity.
const ImageOcrModelTypes = ['ocr'];
const ImageOcrProviders = ['PaddleOCR', 'PaddleOCR.local'];

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

type OcrEngineCardProps = {
  id: string;
  value: ImageOcrEngine;
  checked: boolean;
  disabled?: boolean;
  icon: LucideIcon;
  title: string;
  description: string;
};

function OcrEngineCard({
  id,
  value,
  checked,
  disabled,
  icon: Icon,
  title,
  description,
}: OcrEngineCardProps) {
  return (
    <label
      htmlFor={id}
      className={cn(
        'flex items-start gap-3 rounded-lg border px-3 py-2.5 transition-colors',
        checked
          ? 'border-accent-primary bg-accent-primary-5'
          : 'border-border-button',
        disabled
          ? 'cursor-not-allowed opacity-50'
          : 'cursor-pointer hover:border-border-default',
      )}
    >
      <RadioGroupItem
        value={value}
        id={id}
        disabled={disabled}
        aria-label={title}
        className="mt-0.5 data-[state=checked]:border-accent-primary data-[state=checked]:text-accent-primary"
      />
      <Icon
        className={cn(
          'mt-0.5 size-4 shrink-0',
          checked ? 'text-accent-primary' : 'text-text-secondary',
        )}
      />
      <span className="min-w-0">
        <span className="block text-sm font-medium text-text-primary">
          {title}
        </span>
        <span className="block text-xs text-text-secondary">{description}</span>
      </span>
    </label>
  );
}

/** Lists enrolled PaddleOCR models using the canvas owner's tenant context. */
function ImageOcrEngineFields({ prefix }: CommonProps) {
  const { t } = useTranslation();
  const ownerTenantId = useOwnerTenantId();
  const groupId = useId();
  const {
    treeData: modelTree,
    models,
    isFetched,
    isError,
  } = useModelTree({
    modelTypes: ImageOcrModelTypes,
    providers: ImageOcrProviders,
    ownerTenantId,
  });
  // Auto-pick and restore need tenant UUIDs; entries without a model_id still
  // appear in the tree through buildModelTree's legacy id fallback.
  const ocrModels = useMemo(
    () => models.filter((model) => model.model_id),
    [models],
  );
  const modelIds = useMemo(
    () => new Set(ocrModels.map((model) => model.model_id)),
    [ocrModels],
  );
  const renderMissingModel = useCallback(
    (value: string) => (
      <MissingModelLabel value={value} ownerTenantId={ownerTenantId} />
    ),
    [ownerTenantId],
  );
  // The last external model survives a DeepDoc detour so switching back
  // restores it instead of silently landing on the first listed model.
  const [lastExternalModel, setLastExternalModel] = useState('');

  const description = isError ? (
    <span role="alert" className="text-state-error">
      {t('flow.imageOcrModelsError')}
    </span>
  ) : isFetched && ocrModels.length === 0 ? (
    <span role="status">{t('flow.imageOcrNoModels')}</span>
  ) : undefined;

  return (
    <RAGFlowFormItem
      name={buildFieldNameWithPrefix('ocr_model_id', prefix)}
      label={t('flow.imageOcrParsingMethod')}
      tooltip={t('flow.imageOcrParsingMethodTip')}
      description={description}
    >
      {(field) => {
        const engine = field.value
          ? ImageOcrEngine.External
          : ImageOcrEngine.DeepDoc;
        // External needs a model to point at: stay disabled until the list
        // arrives, and when it is empty or failed to load.
        const externalDisabled =
          !field.value && (!isFetched || ocrModels.length === 0);
        const handleEngineChange = (next: string) => {
          if (next === ImageOcrEngine.DeepDoc) {
            if (field.value) {
              setLastExternalModel(field.value);
            }
            field.onChange('');
            return;
          }
          const restored =
            lastExternalModel && modelIds.has(lastExternalModel)
              ? lastExternalModel
              : ocrModels[0]?.model_id;
          if (restored) {
            field.onChange(restored);
          }
        };
        return (
          <div className="w-full space-y-2">
            <RadioGroup
              value={engine}
              onValueChange={handleEngineChange}
              className="gap-2"
              aria-label={t('flow.imageOcrParsingMethod')}
            >
              <OcrEngineCard
                id={`${groupId}-deepdoc`}
                value={ImageOcrEngine.DeepDoc}
                checked={engine === ImageOcrEngine.DeepDoc}
                icon={ScanText}
                title={t('flow.imageOcrDeepDoc')}
                description={t('flow.imageOcrDeepDocDescription')}
              />
              <OcrEngineCard
                id={`${groupId}-external`}
                value={ImageOcrEngine.External}
                checked={engine === ImageOcrEngine.External}
                disabled={externalDisabled}
                icon={Cloud}
                title={t('flow.imageOcrExternal')}
                description={t('flow.imageOcrExternalDescription')}
              />
            </RadioGroup>
            {engine === ImageOcrEngine.External && (
              <TreeSelect
                data={modelTree}
                value={field.value}
                onChange={field.onChange}
                placeholder={t('flow.imageOcrModelPlaceholder')}
                loading={!isFetched}
                renderMissingValue={renderMissingModel}
                showSearch
                defaultExpandAll
                testId="image-ocr-model"
              />
            )}
          </div>
        );
      }}
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
      {ocrEnabled && <ImageOcrEngineFields prefix={prefix} />}
    </>
  );
}
