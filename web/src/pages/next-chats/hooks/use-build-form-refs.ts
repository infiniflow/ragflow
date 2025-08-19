import { removeUselessFieldsFromValues } from '@/utils/form';
import { isEmpty } from 'lodash';
import { useCallback, useEffect, useRef } from 'react';

export function useBuildFormRefs(chatBoxIds: string[]) {
  const formRefs = useRef<Record<string, { getFormData: () => any }>>({});

  const setFormRef = (id: string) => (ref: { getFormData: () => any }) => {
    formRefs.current[id] = ref;
  };

  const cleanupFormRefs = useCallback(() => {
    const currentIds = new Set(chatBoxIds);
    Object.keys(formRefs.current).forEach((id) => {
      if (!currentIds.has(id)) {
        delete formRefs.current[id];
      }
    });
  }, [chatBoxIds]);

  const getLLMConfigById = useCallback(
    (chatBoxId?: string) => {
      const llmConfig = chatBoxId
        ? formRefs.current[chatBoxId].getFormData()
        : {};

      return removeUselessFieldsFromValues(llmConfig, '');
    },
    [formRefs],
  );

  const isLLMConfigEmpty = useCallback(
    (chatBoxId?: string) => {
      return isEmpty(getLLMConfigById(chatBoxId)?.llm_id);
    },
    [getLLMConfigById],
  );

  useEffect(() => {
    cleanupFormRefs();
  }, [cleanupFormRefs]);

  return {
    formRefs,
    setFormRef,
    getLLMConfigById,
    isLLMConfigEmpty,
  };
}
