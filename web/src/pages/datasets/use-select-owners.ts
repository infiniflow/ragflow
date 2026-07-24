import { FilterCollection } from '@/components/list-filter-bar/interface';
import { useFetchDatasetOwners } from '@/hooks/use-knowledge-request';
import { useTranslation } from 'react-i18next';

export function useSelectOwners(keywords = '') {
  const { owners } = useFetchDatasetOwners(keywords);
  const { t } = useTranslation();

  const filters: FilterCollection[] = [
    {
      field: 'owner',
      list: owners.map((item) => ({
        id: item.tenant_id,
        label: item.nickname,
        count: item.count,
      })),
      label: t('common.owner'),
    },
  ];

  return filters;
}
