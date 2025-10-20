import { FilterCollection } from '@/components/list-filter-bar/interface';
import { useFetchKnowledgeList } from '@/hooks/use-knowledge-request';
import { buildOwnersFilter } from '@/utils/list-filter-util';

export function useSelectOwners() {
  const { list } = useFetchKnowledgeList();

  const filters: FilterCollection[] = [buildOwnersFilter(list)];

  return filters;
}
