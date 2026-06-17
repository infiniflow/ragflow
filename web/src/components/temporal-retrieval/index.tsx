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
  { value: 'auto', label: 'Auto' },
  { value: 'latest', label: 'Latest' },
  { value: 'date_range', label: 'Date range' },
  { value: 'balanced', label: 'Balanced' },
];

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
  const kbIds = useMemo(() => datasetIds || oldKbIds || [], [datasetIds, oldKbIds]);
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
  const { data: profile, loading: profileLoading } =
    useFetchTemporalMetadataProfile(kbIds, enabled ? temporalField || '' : '');

  const fieldOptions = useMemo(
    () => (metadataKeys || []).map((key) => ({ value: key, label: key })),
    [metadataKeys],
  );
  const hasKnowledge = Array.isArray(kbIds) && kbIds.length > 0;

  useEffect(() => {
    if (!enabled || !profile?.temporal_field || profile.temporal_field !== temporalField) {
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
  }, [enabled, form, prefix, profile, temporalField]);

  if (!hasKnowledge) {
    return null;
  }

  return (
    <section className="space-y-4">
      <SwitchFormField
        name={prefix + 'temporal_retrieval.enabled'}
        label="Temporal retrieval"
        tooltip="Use document metadata dates to filter or rerank time-sensitive retrieval."
        vertical={false}
      />
      {enabled && (
        <>
          <RAGFlowFormItem
            label="Temporal mode"
            name={prefix + 'temporal_retrieval.mode'}
            tooltip="Auto only applies freshness when the query asks for recent or dated information."
          >
            <SelectWithSearch options={modeOptions} triggerClassName="!bg-bg-input" />
          </RAGFlowFormItem>
          <RAGFlowFormItem
            label="Temporal field"
            name={prefix + 'temporal_retrieval.temporal_field'}
            tooltip="Select the source metadata field that represents publication or event time."
            required
          >
            <SelectWithSearch
              options={fieldOptions}
              triggerClassName="!bg-bg-input"
              disabled={metadataKeysLoading}
            />
          </RAGFlowFormItem>
          <RAGFlowFormItem
            label="Half-life days"
            name={prefix + 'temporal_retrieval.half_life_days'}
            tooltip="Freshness boost decays by half over this many days."
          >
            <Input type="number" min={1} className="bg-bg-input" />
          </RAGFlowFormItem>
          {temporalField && (
            <div className="rounded-md border border-input-border bg-bg-card p-3 text-sm text-muted-foreground">
              {profileLoading ? (
                <span>{t('common.loading')}</span>
              ) : profile?.temporal_field ? (
                <div className="space-y-1">
                  <div>Detected format: {profile.detected_format || 'unknown'}</div>
                  <div>Parsed: {profile.parsed_percentage ?? 0}%</div>
                  <div>
                    Range: {profile.oldest_date || '-'} to {profile.newest_date || '-'}
                  </div>
                  <div>
                    Hard filter: {profile.supports_hard_filter ? 'supported' : 'not supported'}
                  </div>
                </div>
              ) : (
                <span>No temporal profile available for this field.</span>
              )}
            </div>
          )}
        </>
      )}
    </section>
  );
}
