import { FilterCollection } from '@/components/list-filter-bar/interface';
import { useFetchAgentList } from '@/hooks/use-agent-request';
import { buildOwnersFilter, groupListByType } from '@/utils/list-filter-util';
import { useMemo } from 'react';

export function useSelectFilters() {
  const { data } = useFetchAgentList({});

  const canvasCategory = useMemo(() => {
    return groupListByType(
      data?.canvas ?? [],
      'canvas_category',
      'canvas_category',
    );
  }, [data?.canvas]);

  const filters: FilterCollection[] = [
    buildOwnersFilter(data?.canvas ?? []),
    {
      field: 'canvasCategory',
      list: canvasCategory,
      label: 'Canvas category',
    },
  ];

  return filters;
}
