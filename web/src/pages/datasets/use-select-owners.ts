import { FilterCollection } from '@/components/list-filter-bar/interface';
import { useGetDatasetFilter } from '@/hooks/use-knowledge-request';
import { useTranslation } from 'react-i18next';

export function useSelectOwners() {
  const { filter } = useGetDatasetFilter();
  const { t } = useTranslation();

  const filters: FilterCollection[] = [
    { field: 'owner', list: filter.owner, label: t('common.owner') },
  ];

  return filters;
}
