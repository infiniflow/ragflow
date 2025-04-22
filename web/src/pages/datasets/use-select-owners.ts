import { useFetchKnowledgeList } from '@/hooks/knowledge-hooks';
import { useMemo } from 'react';

export type OwnerFilterType = {
  id: string;
  label: string;
  count: number;
};

export function useSelectOwners() {
  const { list } = useFetchKnowledgeList();

  const owners = useMemo(() => {
    const ownerList: OwnerFilterType[] = [];
    list.forEach((x) => {
      const item = ownerList.find((y) => y.id === x.tenant_id);
      if (!item) {
        ownerList.push({ id: x.tenant_id, label: x.nickname, count: 1 });
      } else {
        item.count += 1;
      }
    });

    return ownerList;
  }, [list]);

  return owners;
}
