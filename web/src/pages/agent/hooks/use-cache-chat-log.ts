import {
  IEventList,
  INodeEvent,
  MessageEventType,
} from '@/hooks/use-send-message';
import { useCallback, useMemo, useState } from 'react';

export const ExcludeTypes = [
  MessageEventType.Message,
  MessageEventType.MessageEnd,
];

export function useCacheChatLog() {
  const [eventList, setEventList] = useState<IEventList>([]);
  const [currentMessageId, setCurrentMessageId] = useState('');

  const filterEventListByMessageId = useCallback(
    (messageId: string) => {
      return eventList.filter((x) => x.message_id === messageId);
    },
    [eventList],
  );

  const filterEventListByEventType = useCallback(
    (eventType: string) => {
      return eventList.filter((x) => x.event === eventType);
    },
    [eventList],
  );

  const clearEventList = useCallback(() => {
    setEventList([]);
  }, []);

  const addEventList = useCallback((events: IEventList) => {
    setEventList((list) => {
      const nextList = [...list];
      events.forEach((x) => {
        if (nextList.every((y) => y !== x)) {
          nextList.push(x);
        }
      });
      return nextList;
    });
  }, []);

  const currentEventListWithoutMessage = useMemo(() => {
    const list = eventList.filter(
      (x) =>
        x.message_id === currentMessageId &&
        ExcludeTypes.every((y) => y !== x.event),
    );

    return list as INodeEvent[];
  }, [currentMessageId, eventList]);

  return {
    eventList,
    currentEventListWithoutMessage,
    setEventList,
    clearEventList,
    addEventList,
    filterEventListByEventType,
    filterEventListByMessageId,
    setCurrentMessageId,
    currentMessageId,
  };
}
