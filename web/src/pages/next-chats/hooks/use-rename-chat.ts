import { useSetModalState } from '@/hooks/common-hooks';
import { useSetDialog } from '@/hooks/use-chat-request';
import { IDialog } from '@/interfaces/database/chat';
import { useCallback, useState } from 'react';

export const useRenameChat = () => {
  const [chat, setChat] = useState<IDialog>({} as IDialog);
  const {
    visible: chatRenameVisible,
    hideModal: hideChatRenameModal,
    showModal: showChatRenameModal,
  } = useSetModalState();
  const { setDialog, loading } = useSetDialog();

  const onChatRenameOk = useCallback(
    async (name: string) => {
      const ret = await setDialog({
        ...chat,
        name,
      });

      if (ret === 0) {
        hideChatRenameModal();
      }
    },
    [setDialog, chat, hideChatRenameModal],
  );

  const handleShowChatRenameModal = useCallback(
    async (record: IDialog) => {
      setChat(record);
      showChatRenameModal();
    },
    [showChatRenameModal],
  );

  return {
    chatRenameLoading: loading,
    initialChatName: chat?.name,
    onChatRenameOk,
    chatRenameVisible,
    hideChatRenameModal,
    showChatRenameModal: handleShowChatRenameModal,
  };
};
