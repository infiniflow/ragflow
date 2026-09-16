import { useSetModalState } from '@/hooks/common-hooks';
import { usePatchChat } from '@/hooks/use-chat-request';
import { IDialog } from '@/interfaces/database/chat';
import { useCallback, useState } from 'react';

export const useRenameChat = () => {
  const [chat, setChat] = useState<IDialog>({} as IDialog);
  const {
    visible: chatRenameVisible,
    hideModal: hideChatRenameModal,
    showModal: showChatRenameModal,
  } = useSetModalState();
  const { patchChat, loading: patchLoading } = usePatchChat();

  const onChatRenameOk = useCallback(
    async (name: string) => {
      const ret = await patchChat({
        chatId: chat.id,
        params: { name },
      });

      if (ret === 0) {
        hideChatRenameModal();
      }
    },
    [chat.id, patchChat, hideChatRenameModal],
  );

  const handleShowChatRenameModal = useCallback(
    (record: IDialog) => {
      setChat(record);
      showChatRenameModal();
    },
    [showChatRenameModal],
  );

  const handleHideModal = useCallback(() => {
    hideChatRenameModal();
    setChat({} as IDialog);
  }, [hideChatRenameModal]);

  return {
    chatRenameLoading: patchLoading,
    initialChatName: chat?.name,
    onChatRenameOk,
    chatRenameVisible,
    hideChatRenameModal: handleHideModal,
    showChatRenameModal: handleShowChatRenameModal,
  };
};
