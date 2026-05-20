import { FilterCollection } from '@/components/list-filter-bar/interface';
import { useFetchAllChatList } from '@/hooks/use-chat-request';
import { buildOwnersFilter } from '@/utils/list-filter-util';

export function useSelectOwners() {
  const { list } = useFetchAllChatList();

  const filters: FilterCollection[] = [buildOwnersFilter(list)];

  return filters;
}
