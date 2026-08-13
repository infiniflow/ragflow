import { INodeEvent, MessageEventType } from '@/hooks/use-send-message';
import { IMessage } from '@/interfaces/database/chat';
import { useMemo, useState } from 'react';
import { getRunningNodeIds } from './use-cache-chat-log';

export const useNodeLoading = ({
  currentEventListWithoutMessageById,
}: {
  currentEventListWithoutMessageById: (messageId: string) => INodeEvent[];
}) => {
  const [derivedMessages, setDerivedMessages] = useState<IMessage[]>();

  const lastMessageId = useMemo(() => {
    return derivedMessages?.[derivedMessages?.length - 1]?.id;
  }, [derivedMessages]);

  const currentEventListWithoutMessage = useMemo(() => {
    if (!lastMessageId) {
      return [];
    }
    return currentEventListWithoutMessageById(lastMessageId);
  }, [currentEventListWithoutMessageById, lastMessageId]);

  const startedNodeList = useMemo(() => {
    const duplicateList = currentEventListWithoutMessage?.filter(
      (x) => x.event === MessageEventType.NodeStarted,
    ) as INodeEvent[];

    // Remove duplicate nodes
    return duplicateList?.reduce<Array<INodeEvent>>((pre, cur) => {
      if (pre.every((x) => x.data.component_id !== cur.data.component_id)) {
        pre.push(cur);
      }
      return pre;
    }, []);
  }, [currentEventListWithoutMessage]);

  const lastNode = useMemo(() => {
    if (!startedNodeList) {
      return null;
    }
    return startedNodeList[startedNodeList.length - 1];
  }, [startedNodeList]);

  // Узел крутится, если его ПОСЛЕДНЕЕ событие в потоке — node_started.
  // Это корректно работает и для loop: при повторном запуске узла
  // последним снова становится node_started.
  const startButNotFinishedNodeIds = useMemo(() => {
    return getRunningNodeIds(currentEventListWithoutMessage);
  }, [currentEventListWithoutMessage]);

  return {
    lastNode,
    startButNotFinishedNodeIds,
    setDerivedMessages,
  };
};
