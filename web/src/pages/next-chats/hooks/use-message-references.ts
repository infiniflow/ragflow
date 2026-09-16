import { MessageType } from '@/constants/chat';
import { IMessage, IReference } from '@/interfaces/database/chat';
import { isEmpty } from 'lodash';
import { useMemo } from 'react';
import { EmptyReference } from '../utils';

/**
 * Resolves the reference of every message in one pass and caches the result
 * until the message list or the conversation's reference list changes.
 *
 * Doing this per message inside the render loop (the previous
 * `buildMessageItemReference` call site) was quadratic, and — worse — handed
 * `MessageItem` a freshly allocated object on every streaming flush, which
 * defeated its `memo` and re-parsed the markdown of the whole transcript
 * ~16 times a second.
 *
 * Keyed by message object rather than by id: a question and its answer share an
 * id, so an id-keyed map could not tell them apart.
 */
export function useMessageReferences(
  messages: IMessage[] | undefined,
  reference: IReference[] | undefined,
) {
  return useMemo(() => {
    const list = messages ?? [];
    const references = reference ?? [];

    // Index of each assistant message within the answer sequence, skipping the
    // prologue (which has no reference) and error answers. First id wins, which
    // is what the previous `findIndex` lookup did.
    const answerIndexById = new Map<string, number>();
    let answerCount = 0;
    list.forEach((message) => {
      if (
        message.role !== MessageType.Assistant ||
        message.content?.startsWith('**ERROR**:')
      ) {
        return;
      }
      if (answerCount > 0 && !answerIndexById.has(message.id)) {
        answerIndexById.set(message.id, answerCount - 1);
      }
      answerCount += 1;
    });

    const resolved = new Map<IMessage, IReference>();
    list.forEach((message) => {
      if (!isEmpty(message.reference)) {
        resolved.set(message, message.reference as IReference);
        return;
      }
      // An assistant message that has not received any content yet is still
      // being generated. Never resolve its reference from the conversation
      // reference list — the indices only align with completed answers, so the
      // lookup would surface a stale reference from a previous turn (or from a
      // previously opened conversation) while the answer is streaming.
      if (message.role === MessageType.Assistant && !message.content) {
        resolved.set(message, EmptyReference);
        return;
      }
      const answerIndex = answerIndexById.get(message.id);
      resolved.set(
        message,
        (answerIndex === undefined ? undefined : references[answerIndex]) ??
          EmptyReference,
      );
    });

    return resolved;
  }, [messages, reference]);
}
