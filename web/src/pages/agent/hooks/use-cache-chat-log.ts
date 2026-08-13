import {
  IEventList,
  INodeEvent,
  MessageEventType,
} from '@/hooks/use-send-message';
import { get, isEmpty } from 'lodash';
import { useCallback, useMemo, useState } from 'react';
import { MessageWaitSuffix } from '../constant/chat';

export const ExcludeTypes = [
  MessageEventType.Message,
  MessageEventType.MessageEnd,
];

export const resolveMessageId = (messageId: string) =>
  messageId
    ?.replace(new RegExp(`${MessageWaitSuffix}$`), '')
    .replace(/#\d+$/, '');

/**
 * Возвращает component_id узлов, которые СЕЙЧАС выполняются.
 *
 * В цикле (loop) узел запускается многократно: пары node_started/node_finished
 * повторяются. Определять статус по факту «узел когда-либо завершался»
 * нельзя — тогда после первого прохода цикла крутилка гаснет навсегда.
 * Правильно смотреть на ПОСЛЕДНЕЕ событие узла в потоке: если это
 * node_started (нет завершающего node_finished после него) — узел работает.
 */
export const getRunningNodeIds = (eventList: INodeEvent[] | undefined): string[] => {
  if (!Array.isArray(eventList)) return [];
  const lastStatus = new Map<string, MessageEventType>();
  for (const x of eventList) {
    if (x.event === MessageEventType.NodeStarted) {
      lastStatus.set(x.data.component_id, x.event);
    } else if (x.event === MessageEventType.NodeFinished) {
      lastStatus.set(x.data.component_id, x.event);
    }
  }
  const running: string[] = [];
  for (const [componentId, event] of lastStatus) {
    if (event === MessageEventType.NodeStarted) {
      running.push(componentId);
    }
  }
  return running;
};

export function useCacheChatLog() {
  const [messageIdPool, setMessageIdPool] = useState<
    Record<string, IEventList>
  >({});

  const [latestTaskId, setLatestTaskId] = useState('');

  const [currentMessageId, setCurrentMessageId] = useState('');

  const filterEventListByMessageId = useCallback(
    (messageId: string) => {
      const resolvedId = resolveMessageId(messageId);
      return messageIdPool[resolvedId]?.filter(
        (x) => x.message_id === resolvedId,
      );
    },
    [messageIdPool],
  );

  const filterEventListByEventType = useCallback(
    (eventType: string) => {
      const resolvedId = resolveMessageId(currentMessageId);
      return messageIdPool[resolvedId]?.filter((x) => x.event === eventType);
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
    const resolvedId = resolveMessageId(currentMessageId);
    const list = messageIdPool[resolvedId]?.filter(
      (x) =>
        x.message_id === resolvedId && ExcludeTypes.every((y) => y !== x.event),
    );
    return list as INodeEvent[];
  }, [currentMessageId, messageIdPool]);

  const currentEventListWithoutMessageById = useCallback(
    (messageId: string) => {
      const resolvedId = resolveMessageId(messageId);
      const list = messageIdPool[resolvedId]?.filter(
        (x) =>
          x.message_id === resolvedId &&
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
