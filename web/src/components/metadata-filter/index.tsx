import { DatasetMetadata } from '@/constants/chat';
import { useTranslate } from '@/hooks/common-hooks';
import { useFormContext, useWatch } from 'react-hook-form';
import { z } from 'zod';
import { SelectWithSearch } from '../originui/select-with-search';
import { RAGFlowFormItem } from '../ragflow-form';
import { MetadataFilterConditions } from './metadata-filter-conditions';

type MetadataFilterProps = {
  prefix?: string;
};

export const MetadataFilterSchema = {
  meta_data_filter: z
    .object({
      method: z.string().optional(),
      manual: z
        .array(
          z.object({
            key: z.string(),
            op: z.string(),
            value: z.string(),
          }),
        )
        .optional(),
    })
    .optional(),
};

export function MetadataFilter({ prefix = '' }: MetadataFilterProps) {
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
        ></MetadataFilterConditions>
      )}
    </>
  );
}
