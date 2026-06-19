import { FilterCollection } from '@/components/list-filter-bar/interface';
import { useFetchKnowledgeList } from '@/hooks/use-knowledge-request';
import { buildOwnersFilter } from '@/utils/list-filter-util';
import { useTranslation } from 'react-i18next';

export function useSelectOwners() {
  const { list } = useFetchKnowledgeList();
  const { t } = useTranslation();

  const filters: FilterCollection[] = [
    buildOwnersFilter(list, undefined, t('common.owner')),
  ];

  return filters;
}
