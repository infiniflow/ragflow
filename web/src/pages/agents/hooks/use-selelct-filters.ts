import { FilterCollection } from '@/components/list-filter-bar/interface';
import {
  useFetchAgentList,
  useFetchAgentTags,
} from '@/hooks/use-agent-request';
import { buildOwnersFilter, groupListByType } from '@/utils/list-filter-util';
import { useMemo } from 'react';

export function useSelectFilters() {
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
    buildOwnersFilter(data?.canvas ?? []),
    {
      field: 'canvasCategory',
      list: canvasCategory,
      label: 'Canvas category',
    },
    {
      field: 'tags',
      list: tagList,
      label: 'Tags',
    },
  ];

  return filters;
}
