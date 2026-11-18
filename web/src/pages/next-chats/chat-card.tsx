import { HomeCard } from '@/components/home-card';
import { MoreButton } from '@/components/more-button';
import { SharedBadge } from '@/components/shared-badge';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { IDialog } from '@/interfaces/database/chat';
import { useSidebarCollapse } from '@/layouts/components/app-sidebar/use-sidebar-collapse';
import { useCallback } from 'react';
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
  const { setCollapsed: setPrimarySidebarCollapsed } = useSidebarCollapse();

  const handleCardClick = useCallback(() => {
    setPrimarySidebarCollapsed(true);
    navigateToChat(data.id)();
  }, [data.id, navigateToChat, setPrimarySidebarCollapsed]);

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
      onClick={handleCardClick}
    />
  );
}
