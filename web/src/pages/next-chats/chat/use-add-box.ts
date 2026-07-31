import { useCallback, useEffect, useState } from 'react';
import { v4 as uuid } from 'uuid';

export function useAddChatBox(isDebugMode: boolean) {
  const [ids, setIds] = useState<string[]>([uuid()]);

  const hasSingleChatBox = ids.length === 1;

  const hasThreeChatBox = ids.length === 3;

  const addChatBox = useCallback(() => {
    setIds((prev) => [...prev, uuid()]);
  }, []);

  const removeChatBox = useCallback((id: string) => {
    setIds((prev) => prev.filter((x) => x !== id));
  }, []);

  useEffect(() => {
    if (!isDebugMode) {
      setIds((pre) => pre.slice(0, 1));
    }
  }, [isDebugMode]);

  return {
    chatBoxIds: ids,
    hasSingleChatBox,
    hasThreeChatBox,
    addChatBox,
    removeChatBox,
  };
}
