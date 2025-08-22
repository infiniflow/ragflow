import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchDialogList } from '@/hooks/use-chat-request';
import { ApplicationCard } from './application-card';

export function ChatList() {
  const { data } = useFetchDialogList();
  const { navigateToChat } = useNavigatePage();

  return data.dialogs.slice(0, 10).map((x) => (
    <ApplicationCard
      key={x.id}
      app={{
        avatar: x.icon,
        title: x.name,
        update_time: x.update_time,
      }}
      onClick={navigateToChat(x.id)}
    ></ApplicationCard>
  ));
}
