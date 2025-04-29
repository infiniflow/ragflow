import { FilterCollection } from '@/components/list-filter-bar/interface';
import { useFetchKnowledgeList } from '@/hooks/knowledge-hooks';
import { groupListByType } from '@/utils/dataset-util';
import { useMemo } from 'react';

export function useSelectOwners() {
  const { list } = useFetchKnowledgeList();

  const owners = useMemo(() => {
    return groupListByType(list, 'tenant_id', 'nickname');
  }, [list]);

  const filters: FilterCollection[] = [
    { field: 'owner', list: owners, label: 'Owner' },
  ];

  return filters;
}
