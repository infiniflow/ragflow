import { useSetModalState } from '@/hooks/common-hooks';
import { useSetDialog } from '@/hooks/use-chat-request';
import { IDialog } from '@/interfaces/database/chat';
import { isEmpty } from 'lodash';
import { useCallback, useState } from 'react';

const InitialData = {
  name: '',
  icon: '',
  language: 'English',
  prompt_config: {
    empty_response: '',
    prologue: '你好！ 我是你的助理，有什么可以帮到你的吗？',
    quote: true,
    keyword: false,
    tts: false,
    system:
      '你是一个智能助手，请总结知识库的内容来回答问题，请列举知识库中的数据详细回答。当所有知识库内容都与问题无关时，你的回答必须包括“知识库中未找到您要的答案！”这句话。回答需要考虑聊天历史。\n        以下是知识库：\n        {knowledge}\n        以上是知识库。',
    refine_multiturn: false,
    use_kg: false,
    reasoning: false,
    parameters: [{ key: 'knowledge', optional: false }],
  },
  llm_id: '',
  llm_setting: {},
  similarity_threshold: 0.2,
  vector_similarity_weight: 0.30000000000000004,
  top_n: 8,
};

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
      const nextChat = {
        ...(isEmpty(chat) ? InitialData : chat),
        name,
      };
      const ret = await setDialog(nextChat);

      if (ret === 0) {
        hideChatRenameModal();
      }
    },
    [setDialog, chat, hideChatRenameModal],
  );

  const handleShowChatRenameModal = useCallback(
    (record?: IDialog) => {
      if (record) {
        setChat(record);
      }
      showChatRenameModal();
    },
    [showChatRenameModal],
  );

  const handleHideModal = useCallback(() => {
    hideChatRenameModal();
    setChat({} as IDialog);
  }, [hideChatRenameModal]);

  return {
    chatRenameLoading: loading,
    initialChatName: chat?.name,
    onChatRenameOk,
    chatRenameVisible,
    hideChatRenameModal: handleHideModal,
    showChatRenameModal: handleShowChatRenameModal,
  };
};
