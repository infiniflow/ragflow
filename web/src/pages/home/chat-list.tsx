import { useFetchNextDialogList } from '@/hooks/chat-hooks';
import { ApplicationCard } from './application-card';

export function ChatList() {
  const { data } = useFetchNextDialogList();

  return data
    .slice(0, 10)
    .map((x) => (
      <ApplicationCard
        key={x.id}
        app={{ avatar: x.icon, title: x.name, update_time: x.update_time }}
      ></ApplicationCard>
    ));
}
