import { MessageType } from '@/constants/chat';
import { IAnswer, IMessage } from '@/interfaces/database/chat';
import { buildMessageUuid } from '@/utils/chat';
import { omit } from 'lodash';
import { CompletionChunk } from '@/services/chat-completion-stream';
import { PrologueMessageIdPrefix } from './constants';

/**
 * Accumulates a streamed answer chunk onto the previously accumulated text.
 * Ported verbatim from the legacy `setAnswer` reducer in
 * `useSendMessageWithSse` (src/hooks/logic-hooks.ts) to preserve behaviour:
 * the final chunk is skipped only when earlier chunks exist (single-shot
 * answers arrive with `final: true` only), a chunk that already contains the
 * previous text replaces it instead of being appended, and think markers are
 * injected inline.
 */
export function mergeAnswerChunk(
  previousAnswer: string,
  chunk: CompletionChunk,
): string {
  // A final chunk carries the COMPLETE answer (citations inserted, references
  // attached server-side). It replaces the accumulated ANSWER — but NOT the
  // thinking accumulated so far, which is the only record of the ReAct
  // trajectory the user watched unfold (the final chunk ships the deliverable
  // alone). A naive final is self-contained (`think + decorated answer`), so
  // it replaces everything wholesale instead of duplicating the think span.
  if (chunk.final === true && chunk.answer) {
    if (chunk.answer.includes('<think>')) {
      return chunk.answer;
    }
    const closeTag = '</think>';
    const closeIdx = previousAnswer.lastIndexOf(closeTag);
    if (closeIdx !== -1) {
      return (
        previousAnswer.slice(0, closeIdx + closeTag.length) +
        '\n\n' +
        chunk.answer
      );
    }
    return chunk.answer;
  }

  const currentAnswer = chunk.final && previousAnswer ? '' : chunk.answer || '';

  let nextAnswer: string;
  if (previousAnswer && currentAnswer.startsWith(previousAnswer)) {
    nextAnswer = currentAnswer;
  } else {
    nextAnswer = previousAnswer + currentAnswer;
  }

  if (chunk.start_to_think === true) {
    nextAnswer = nextAnswer + '<think>';
  }

  if (chunk.end_to_think === true) {
    // The closing marker must be followed by a blank line: the answer's first
    // line lands right after it, and a heading (`## Candidate Matrix`) glued
    // onto '</think>' stops being a heading — markdown only parses `#` at
    // line begin.
    nextAnswer = nextAnswer + '</think>\n\n';
  }

  return nextAnswer;
}

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
