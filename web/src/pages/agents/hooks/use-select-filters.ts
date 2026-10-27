import { FilterCollection } from '@/components/list-filter-bar/interface';
import {
  useFetchAgentFilters,
  useFetchAgentTags,
} from '@/hooks/use-agent-request';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

export function useSelectFilters(canvasCategory?: string) {
  const { t } = useTranslation();
  const { data: agentFilters } = useFetchAgentFilters(canvasCategory);
  const { data: tagCounts } = useFetchAgentTags(canvasCategory);

  const tagList = useMemo(
    () =>
      (tagCounts ?? []).map((t) => ({
        id: t.tag,
        label: t.tag,
        count: t.count,
      })),
    [tagCounts],
  );

  const filters: FilterCollection[] = useMemo(
    () => [
      {
        field: 'owner',
        list: agentFilters?.owner,
        label: t('common.owner'),
      },
      {
        field: 'tags',
        list: tagList,
        label: t('flow.tags'),
      },
    ],
    [agentFilters, tagList, t],
  );

  return filters;
}
