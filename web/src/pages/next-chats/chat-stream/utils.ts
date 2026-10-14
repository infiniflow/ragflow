import { MessageType } from '@/constants/chat';
import { IAnswer, IMessage } from '@/interfaces/database/chat';
import { buildMessageUuid } from '@/utils/chat';
import { omit } from 'lodash';
import { PrologueMessageIdPrefix } from './constants';

/** Builds the assistant message that replaces the trailing placeholder. */
export function buildAssistantMessageFromAnswer(answer: IAnswer): IMessage {
  return {
    role: MessageType.Assistant,
    content: answer.answer,
    reference: answer.reference,
    id: buildMessageUuid({ id: answer.id, role: MessageType.Assistant }),
    prompt: answer.prompt,
    audio_binary: answer.audio_binary,
    ...omit(answer, 'reference'),
  } as IMessage;
}

/**
 * Builds the user question plus the empty assistant placeholder that the
 * streamed answer will progressively fill in.
 */
export function buildQuestionAndPlaceholder(message: IMessage): IMessage[] {
  return [
    {
      ...message,
      id: buildMessageUuid(message),
    },
    {
      role: MessageType.Assistant,
      content: '',
      conversationId: message.conversationId,
      id: buildMessageUuid({ ...message, role: MessageType.Assistant }),
    } as IMessage,
  ];
}

/**
 * Server messages don't carry the locally uploaded `File` instances. Re-attach
 * them by message id so attachments survive a server refresh.
 */
export function mergeLocalFiles(
  serverMessages: IMessage[],
  localMessages: IMessage[],
): IMessage[] {
  const filesMap = new Map(
    localMessages.filter((x) => x.files?.length).map((x) => [x.id, x.files]),
  );

  if (filesMap.size === 0) {
    return serverMessages;
  }

  return serverMessages.map((x) => ({
    ...x,
    files: filesMap.get(x.id) ?? x.files,
  }));
}

export function buildPrologueMessageId(conversationId: string) {
  return `${PrologueMessageIdPrefix}${conversationId}`;
}
