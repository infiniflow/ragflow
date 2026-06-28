import { useSetModalState } from '@/hooks/common-hooks';
import { useCreateChat } from '@/hooks/use-chat-request';
import { useFetchDefaultModelDictionary } from '@/hooks/use-llm-request';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';

export const useCreateChatDialog = () => {
  const {
    visible: createChatVisible,
    hideModal: hideCreateChatModal,
    showModal: showCreateChatModal,
  } = useSetModalState();
  const { createChat, loading: createLoading } = useCreateChat();
  const { t } = useTranslation();
  const defaultModelDictionary =
    useFetchDefaultModelDictionary(createChatVisible);

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
      llm_id: defaultModelDictionary?.llm_id,
      llm_setting: {},
      similarity_threshold: 0.2,
      vector_similarity_weight: 0.3,
      top_n: 8,
      top_k: 1024,
    }),
    [t, defaultModelDictionary?.llm_id],
  );

  const onCreateChatOk = useCallback(
    async (name: string) => {
      const ret = await createChat({ ...InitialData, name });
      if (ret === 0) {
        hideCreateChatModal();
      }
    },
    [InitialData, createChat, hideCreateChatModal],
  );

  return {
    createChatLoading: createLoading,
    onCreateChatOk,
    createChatVisible,
    hideCreateChatModal,
    showCreateChatModal,
  };
};
