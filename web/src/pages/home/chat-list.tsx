import { MoreButton } from '@/components/more-button';
import { RenameDialog } from '@/components/rename-dialog';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchDialogList } from '@/hooks/use-chat-request';
import { useTranslation } from 'react-i18next';
import { ChatDropdown } from '../next-chats/chat-dropdown';
import { useRenameChat } from '../next-chats/hooks/use-rename-chat';
import { ApplicationCard } from './application-card';

export function ChatList() {
  const { t } = useTranslation();
  const { data } = useFetchDialogList();
  const { navigateToChat } = useNavigatePage();

  const {
    initialChatName,
    chatRenameVisible,
    showChatRenameModal,
    hideChatRenameModal,
    onChatRenameOk,
    chatRenameLoading,
  } = useRenameChat();

  return (
    <>
      {data.dialogs.slice(0, 10).map((x) => (
        <ApplicationCard
          key={x.id}
          app={{
            avatar: x.icon,
            title: x.name,
            update_time: x.update_time,
          }}
          onClick={navigateToChat(x.id)}
          moreDropdown={
            <ChatDropdown chat={x} showChatRenameModal={showChatRenameModal}>
              <MoreButton></MoreButton>
            </ChatDropdown>
          }
        ></ApplicationCard>
      ))}
      {chatRenameVisible && (
        <RenameDialog
          hideModal={hideChatRenameModal}
          onOk={onChatRenameOk}
          initialName={initialChatName}
          loading={chatRenameLoading}
          title={initialChatName || t('chat.createChat')}
        ></RenameDialog>
      )}
    </>
  );
}
