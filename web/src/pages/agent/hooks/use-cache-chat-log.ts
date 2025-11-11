import {
  IEventList,
  INodeEvent,
  MessageEventType,
} from '@/hooks/use-send-message';
import { useCallback, useEffect, useMemo, useState } from 'react';

export const ExcludeTypes = [
  MessageEventType.Message,
  MessageEventType.MessageEnd,
];

export function useCacheChatLog() {
  const [eventList, setEventList] = useState<IEventList>([]);
  const [messageIdPool, setMessageIdPool] = useState<
    Record<string, IEventList>
  >({});

  const [currentMessageId, setCurrentMessageId] = useState('');
  useEffect(() => {
    setMessageIdPool((prev) => ({ ...prev, [currentMessageId]: eventList }));
  }, [currentMessageId, eventList]);

  const filterEventListByMessageId = useCallback(
    (messageId: string) => {
      return messageIdPool[messageId]?.filter(
        (x) => x.message_id === messageId,
      );
    },
    [messageIdPool],
  );

  const filterEventListByEventType = useCallback(
    (eventType: string) => {
      return messageIdPool[currentMessageId]?.filter(
        (x) => x.event === eventType,
      );
    },
    [messageIdPool, currentMessageId],
  );

  const clearEventList = useCallback(() => {
    setEventList([]);
    setMessageIdPool({});
  }, []);

  const addEventList = useCallback((events: IEventList, message_id: string) => {
    setEventList((x) => {
      const list = [...x, ...events];
      setMessageIdPool((prev) => ({ ...prev, [message_id]: list }));
      return list;
    });
  }, []);

  const currentEventListWithoutMessage = useMemo(() => {
    const list = messageIdPool[currentMessageId]?.filter(
      (x) =>
        x.message_id === currentMessageId &&
        ExcludeTypes.every((y) => y !== x.event),
    );
    return list as INodeEvent[];
  }, [currentMessageId, messageIdPool]);

  const currentEventListWithoutMessageById = useCallback(
    (messageId: string) => {
      const list = messageIdPool[messageId]?.filter(
        (x) =>
          x.message_id === messageId &&
          ExcludeTypes.every((y) => y !== x.event),
      );
      return list as INodeEvent[];
    },
    [messageIdPool],
  );

  const currentTaskId = useMemo(() => {
    return eventList.at(-1)?.task_id;
  }, [eventList]);

  return {
    eventList,
    currentEventListWithoutMessage,
    currentEventListWithoutMessageById,
    setEventList,
    clearEventList,
    addEventList,
    filterEventListByEventType,
    filterEventListByMessageId,
    setCurrentMessageId,
    currentMessageId,
    currentTaskId,
  };
}
