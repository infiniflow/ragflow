import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchSearchList } from '../next-searches/hooks';
import { ApplicationCard } from './application-card';

export function SearchList() {
  const { data } = useFetchSearchList();
  const { navigateToSearch } = useNavigatePage();

  return data?.data.search_apps.slice(0, 10).map((x) => (
    <ApplicationCard
      key={x.id}
      app={{
        avatar: x.avatar,
        title: x.name,
        update_time: x.update_time,
      }}
      onClick={navigateToSearch(x.id)}
    ></ApplicationCard>
  ));
}
