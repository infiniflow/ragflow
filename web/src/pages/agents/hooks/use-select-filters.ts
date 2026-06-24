import { FilterCollection } from '@/components/list-filter-bar/interface';
import {
  useFetchAgentList,
  useFetchAgentTags,
} from '@/hooks/use-agent-request';
import { buildOwnersFilter, groupListByType } from '@/utils/list-filter-util';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

export function useSelectFilters() {
  const { t } = useTranslation();
  const { data } = useFetchAgentList({});
  const { data: tagCounts } = useFetchAgentTags();

  const canvasCategory = useMemo(() => {
    return groupListByType(
      data?.canvas ?? [],
      'canvas_category',
      'canvas_category',
    );
  }, [data?.canvas]);

  const tagList = useMemo(
    () =>
      (tagCounts ?? []).map((t) => ({
        id: t.tag,
        label: t.tag,
        count: t.count,
      })),
    [tagCounts],
  );

  const filters: FilterCollection[] = [
    buildOwnersFilter(data?.canvas ?? [], undefined, t('common.owner')),
    {
      field: 'canvasCategory',
      list: canvasCategory,
      label: t('flow.canvasCategory'),
    },
    {
      field: 'tags',
      list: tagList,
      label: t('flow.tags'),
    },
  ];

  return filters;
}
