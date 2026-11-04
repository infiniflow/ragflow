import { HomeCard } from '@/components/home-card';
import { MoreButton } from '@/components/more-button';
import { SharedBadge } from '@/components/shared-badge';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { IDialog } from '@/interfaces/database/chat';
import { ChatDropdown } from './chat-dropdown';
import { useRenameChat } from './hooks/use-rename-chat';

export type IProps = {
  data: IDialog;
  showPrivilegeModal(): void;
} & Pick<ReturnType<typeof useRenameChat>, 'showChatRenameModal'>;

export function ChatCard({
  data,
  showChatRenameModal,
  showPrivilegeModal,
}: IProps) {
  const { navigateToChat } = useNavigatePage();

  return (
    <HomeCard
      data={{
        name: data.name,
        description: data.description,
        avatar: data.icon,
        update_time: data.update_time,
      }}
      moreDropdown={
        <ChatDropdown
          chat={data}
          showChatRenameModal={showChatRenameModal}
          showPrivilegeModal={showPrivilegeModal}
        >
          <MoreButton></MoreButton>
        </ChatDropdown>
      }
      sharedBadge={<SharedBadge>{data.nickname}</SharedBadge>}
      onClick={navigateToChat(data?.id)}
    />
  );
}
