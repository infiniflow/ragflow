import { useSetModalState } from '@/hooks/common-hooks';
import { useCreateChat, usePatchChat } from '@/hooks/use-chat-request';
import { useFetchTenantInfo } from '@/hooks/use-user-setting-request';
import { IDialog } from '@/interfaces/database/chat';
import { isEmpty } from 'lodash';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

export const useRenameChat = () => {
  const [chat, setChat] = useState<IDialog>({} as IDialog);
  const {
    visible: chatRenameVisible,
    hideModal: hideChatRenameModal,
    showModal: showChatRenameModal,
  } = useSetModalState();
  const { createChat, loading: createLoading } = useCreateChat();
  const { patchChat, loading: patchLoading } = usePatchChat();
  const { t } = useTranslation();
  const tenantInfo = useFetchTenantInfo();

  const InitialData = useMemo(
    () => ({
      name: '',
      icon: '',
      language: 'English',
      description: '',
      dataset_ids: [],
      prompt_config: {
        empty_response: '',
        prologue: t('chat.setAnOpenerInitial'),
        quote: true,
        keyword: false,
        tts: false,
        system: t('chat.systemInitialValue'),
        refine_multiturn: false,
        use_kg: false,
        reasoning: false,
        parameters: [{ key: 'knowledge', optional: false }],
        toc_enhance: false,
      },
      llm_id: tenantInfo.data.llm_id,
      llm_setting: {},
      similarity_threshold: 0.2,
      vector_similarity_weight: 0.3,
      top_n: 8,
      top_k: 1024,
    }),
    [t, tenantInfo.data.llm_id],
  );

  const onChatRenameOk = useCallback(
    async (name: string) => {
      let ret: number | undefined;
      if (isEmpty(chat)) {
        ret = await createChat({ ...InitialData, name });
      } else {
        ret = await patchChat({
          chatId: chat.id,
          params: { name },
        });
      }

      if (ret === 0) {
        hideChatRenameModal();
      }
    },
    [chat, InitialData, createChat, patchChat, hideChatRenameModal],
  );

  const handleShowChatRenameModal = useCallback(
    (record?: IDialog) => {
      if (record) {
        setChat(record);
      } else {
        setChat({} as IDialog);
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
    chatRenameLoading: createLoading || patchLoading,
    initialChatName: chat?.name,
    onChatRenameOk,
    chatRenameVisible,
    hideChatRenameModal: handleHideModal,
    showChatRenameModal: handleShowChatRenameModal,
  };
};
