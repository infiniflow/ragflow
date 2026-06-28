import { MessageType } from '@/constants/chat';
import { IEventList, MessageEventType } from '@/hooks/use-send-message';
import { IMessage, IReference } from '@/interfaces/database/chat';
import { isEmpty } from 'lodash';

export const buildAgentMessageItemReference = (
  conversation: { messages: IMessage[]; reference: IReference[] },
  message: IMessage,
) => {
  const assistantMessages = conversation.messages?.filter(
    (x) => x.role === MessageType.Assistant,
  );
  const referenceIndex = assistantMessages.findIndex(
    (x) => x.id === message.id,
  );
  const reference = !isEmpty(message?.reference)
    ? message?.reference
    : (conversation?.reference ?? [])[referenceIndex];

  return reference ?? { doc_aggs: [], chunks: [], total: 0 };
};

/**
 * Determines whether the message should be split into two separate entries:
 * one for the assistant's answer and one for the user input prompt.
 *
 * A split is needed when all of the following are true:
 * 1. The event list contains a `MessageEnd` event.
 * 2. The event list contains a `UserInputs` event.
 * 3. The `MessageEnd` event occurs before the `UserInputs` event.
 * 4. There is actual message content (`content` is truthy).
 *
 * @param eventList - The list of SSE events received from the server.
 * @param content   - The assistant's message content extracted from the events.
 * @returns `true` if the message should be split, otherwise `false`.
 */
export function shouldSplitMessage(
  eventList: IEventList,
  content?: string,
): boolean {
  const messageEndIndex = eventList.findIndex(
    (x) => x.event === MessageEventType.MessageEnd,
  );
  const userInputsIndex = eventList.findIndex(
    (x) => x.event === MessageEventType.UserInputs,
  );

  return (
    messageEndIndex !== -1 &&
    userInputsIndex !== -1 &&
    messageEndIndex < userInputsIndex &&
    !!content
  );
}
