import {
  IEventList,
  INodeEvent,
  MessageEventType,
} from '@/hooks/use-send-message';
import { get, isEmpty } from 'lodash';
import { useCallback, useMemo, useState } from 'react';

export const ExcludeTypes = [
  MessageEventType.Message,
  MessageEventType.MessageEnd,
];

export function useCacheChatLog() {
  const [messageIdPool, setMessageIdPool] = useState<
    Record<string, IEventList>
  >({});

  const [latestTaskId, setLatestTaskId] = useState('');

  const [currentMessageId, setCurrentMessageId] = useState('');

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
    setMessageIdPool({});
  }, []);

  const addEventList = useCallback((events: IEventList, message_id: string) => {
    if (!isEmpty(events)) {
      const taskId = get(events, '0.task_id');
      setLatestTaskId(taskId);

      setMessageIdPool((prev) => {
        const list = [...(prev[message_id] ?? [])];

        events.forEach((event) => {
          if (!list.some((y) => y === event)) {
            list.push(event);
          }
        });

        return { ...prev, [message_id]: list };
      });
    }
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

  return {
    currentEventListWithoutMessage,
    currentEventListWithoutMessageById,
    clearEventList,
    addEventList,
    filterEventListByEventType,
    filterEventListByMessageId,
    setCurrentMessageId,
    currentMessageId,
    latestTaskId,
  };
}
