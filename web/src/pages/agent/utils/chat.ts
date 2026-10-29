import { IEventList, MessageEventType } from '@/hooks/use-send-message';

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
