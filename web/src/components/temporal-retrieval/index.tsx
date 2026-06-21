import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { SwitchFormField } from '@/components/switch-fom-field';
import { Input } from '@/components/ui/input';
import {
  useFetchKnowledgeMetadataKeys,
  useFetchTemporalMetadataProfile,
} from '@/hooks/use-knowledge-request';
import { useEffect, useMemo } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { resolveEffectiveDatasetIds } from './utils';

export { resolveEffectiveDatasetIds } from './utils';

type TemporalRetrievalProps = {
  prefix?: string;
};

export const TemporalRetrievalSchema = {
  temporal_retrieval: z
    .object({
      enabled: z.boolean().optional(),
      mode: z.enum(['auto', 'latest', 'date_range', 'balanced']).optional(),
      temporal_field: z.string().optional(),
      half_life_days: z.coerce.number().positive().optional(),
      detected_format: z.string().nullable().optional(),
      supports_hard_filter: z.boolean().optional(),
      supports_freshness_score: z.boolean().optional(),
    })
    .optional(),
};

const modeOptions = [
  { value: 'auto', labelKey: 'temporalRetrieval.modeAuto' },
  { value: 'latest', labelKey: 'temporalRetrieval.modeLatest' },
  { value: 'date_range', labelKey: 'temporalRetrieval.modeDateRange' },
  { value: 'balanced', labelKey: 'temporalRetrieval.modeBalanced' },
] as const;

const DERIVED_PROFILE_FIELDS = [
  'detected_format',
  'supports_hard_filter',
  'supports_freshness_score',
] as const;

export function TemporalRetrieval({ prefix = '' }: TemporalRetrievalProps) {
  const { t } = useTranslation();
  const form = useFormContext();
  const datasetIds: string[] = useWatch({
    control: form.control,
    name: prefix + 'dataset_ids',
  });
  const oldKbIds: string[] = useWatch({
    control: form.control,
    name: prefix + 'kb_ids',
  });
  const kbIds = useMemo(
    () => resolveEffectiveDatasetIds(datasetIds, oldKbIds),
    [datasetIds, oldKbIds],
  );
  const enabled = useWatch({
    control: form.control,
    name: prefix + 'temporal_retrieval.enabled',
  });
  const temporalField = useWatch({
    control: form.control,
    name: prefix + 'temporal_retrieval.temporal_field',
  });
  const { data: metadataKeys, loading: metadataKeysLoading } =
    useFetchKnowledgeMetadataKeys(kbIds);
  const { data: profile, loading: profileLoading, error: profileError } =
    useFetchTemporalMetadataProfile(kbIds, enabled ? temporalField || '' : '');

  const fieldOptions = useMemo(
    () => (metadataKeys || []).map((key) => ({ value: key, label: key })),
    [metadataKeys],
  );
  const hasKnowledge = Array.isArray(kbIds) && kbIds.length > 0;
  const translatedModeOptions = useMemo(
    () =>
      modeOptions.map((option) => ({
        value: option.value,
        label: t(`chat.${option.labelKey}`),
      })),
    [t],
  );

  const clearDerivedProfileFields = () => {
    for (const field of DERIVED_PROFILE_FIELDS) {
      form.setValue(prefix + `temporal_retrieval.${field}`, undefined);
    }
  };

  useEffect(() => {
    if (!enabled) {
      clearDerivedProfileFields();
    }
  }, [enabled, form, prefix]);

  useEffect(() => {
    clearDerivedProfileFields();
  }, [temporalField, form, prefix]);

  useEffect(() => {
    if (!enabled || profileLoading || profileError || !profile?.temporal_field) {
      return;
    }
    if (profile.temporal_field !== temporalField) {
      return;
    }
    form.setValue(
      prefix + 'temporal_retrieval.detected_format',
      profile.detected_format,
    );
    form.setValue(
      prefix + 'temporal_retrieval.supports_hard_filter',
      Boolean(profile.supports_hard_filter),
    );
    form.setValue(
      prefix + 'temporal_retrieval.supports_freshness_score',
      Boolean(profile.supports_freshness_score),
    );
  }, [
    enabled,
    form,
    prefix,
    profile,
    profileError,
    profileLoading,
    temporalField,
  ]);

  if (!hasKnowledge) {
    return null;
  }

  return (
    <section className="space-y-4">
      <SwitchFormField
        name={prefix + 'temporal_retrieval.enabled'}
        label={t('chat.temporalRetrieval.title')}
        tooltip={t('chat.temporalRetrieval.description')}
        vertical={false}
      />
      {enabled && (
        <>
          <RAGFlowFormItem
            label={t('chat.temporalRetrieval.modeLabel')}
            name={prefix + 'temporal_retrieval.mode'}
            tooltip={t('chat.temporalRetrieval.modeTip')}
          >
            <SelectWithSearch
              options={translatedModeOptions}
              triggerClassName="!bg-bg-input"
            />
          </RAGFlowFormItem>
          <RAGFlowFormItem
            label={t('chat.temporalRetrieval.fieldLabel')}
            name={prefix + 'temporal_retrieval.temporal_field'}
            tooltip={t('chat.temporalRetrieval.fieldTip')}
            required
          >
            <SelectWithSearch
              options={fieldOptions}
              triggerClassName="!bg-bg-input"
              disabled={metadataKeysLoading}
            />
          </RAGFlowFormItem>
          <RAGFlowFormItem
            label={t('chat.temporalRetrieval.halfLifeLabel')}
            name={prefix + 'temporal_retrieval.half_life_days'}
            tooltip={t('chat.temporalRetrieval.halfLifeTip')}
          >
            <Input type="number" min={1} className="bg-bg-input" />
          </RAGFlowFormItem>
          {temporalField && (
            <div className="rounded-md border border-input-border bg-bg-card p-3 text-sm text-muted-foreground">
              {profileLoading ? (
                <span>{t('common.loading')}</span>
              ) : profileError ? (
                <span>{t('chat.temporalRetrieval.profileError')}</span>
              ) : profile?.temporal_field ? (
                <div className="space-y-1">
                  <div>
                    {t('chat.temporalRetrieval.detectedFormat', {
                      format: profile.detected_format || t('chat.temporalRetrieval.unknownFormat'),
                    })}
                  </div>
                  <div>
                    {t('chat.temporalRetrieval.parsedRate', {
                      rate: profile.parsed_percentage ?? 0,
                    })}
                  </div>
                  <div>
                    {t('chat.temporalRetrieval.dateRange', {
                      oldest: profile.oldest_date || '-',
                      newest: profile.newest_date || '-',
                    })}
                  </div>
                  <div>
                    {profile.supports_hard_filter
                      ? t('chat.temporalRetrieval.hardFilterSupported')
                      : t('chat.temporalRetrieval.hardFilterUnsupported')}
                  </div>
                </div>
              ) : (
                <span>{t('chat.temporalRetrieval.noProfile')}</span>
              )}
            </div>
          )}
        </>
      )}
    </section>
  );
}
