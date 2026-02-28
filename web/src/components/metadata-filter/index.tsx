import { DatasetMetadata } from '@/constants/chat';
import { useTranslate } from '@/hooks/common-hooks';
import { useFormContext, useWatch } from 'react-hook-form';
import { z } from 'zod';
import { SelectWithSearch } from '../originui/select-with-search';
import { RAGFlowFormItem } from '../ragflow-form';
import { MetadataFilterConditions } from './metadata-filter-conditions';
import { MetadataSemiAutoFields } from './metadata-semi-auto-fields';

type MetadataFilterProps = {
  prefix?: string;
  canReference?: boolean;
};

export const MetadataFilterSchema = {
  meta_data_filter: z
    .object({
      logic: z.string().optional(),
      method: z.string().optional(),
      manual: z
        .array(
          z.object({
            key: z.string(),
            op: z.string(),
            value: z.union([z.string(), z.array(z.string())]),
          }),
        )
        .optional(),
      semi_auto: z
        .array(
          z.union([
            z.string(),
            z.object({
              key: z.string(),
              op: z.string().optional(),
            }),
          ]),
        )
        .optional(),
    })
    .optional(),
};

export function MetadataFilter({
  prefix = '',
  canReference,
}: MetadataFilterProps) {
  const { t } = useTranslate('chat');
  const form = useFormContext();

  const methodName = prefix + 'meta_data_filter.method';

  const kbIds: string[] = useWatch({
    control: form.control,
    name: prefix + 'kb_ids',
  });
  const metadata = useWatch({
    control: form.control,
    name: methodName,
  });
  const hasKnowledge = Array.isArray(kbIds) && kbIds.length > 0;

  const MetadataOptions = Object.values(DatasetMetadata).map((x) => {
    return {
      value: x,
      label: t(`meta.${x}`),
    };
  });

  return (
    <>
      {hasKnowledge && (
        <RAGFlowFormItem
          label={t('metadata')}
          name={methodName}
          tooltip={t('metadataTip')}
        >
          <SelectWithSearch
            options={MetadataOptions}
            triggerClassName="!bg-bg-input"
          />
        </RAGFlowFormItem>
      )}
      {hasKnowledge && metadata === DatasetMetadata.Manual && (
        <MetadataFilterConditions
          kbIds={kbIds}
          prefix={prefix}
          canReference={canReference}
        ></MetadataFilterConditions>
      )}
      {hasKnowledge && metadata === DatasetMetadata.SemiAutomatic && (
        <MetadataSemiAutoFields
          kbIds={kbIds}
          prefix={prefix}
        ></MetadataSemiAutoFields>
      )}
    </>
  );
}
