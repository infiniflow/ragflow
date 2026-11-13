import { IMessage, IReference } from '@/interfaces/database/chat';
import { useMemo } from 'react';
import { EmptyReference } from '../utils';

// Key by object because a question and its answer share the same message id.
export function useMessageReferences(messages: IMessage[] | undefined) {
  return useMemo(() => {
    const resolved = new Map<IMessage, IReference>();
    (messages ?? []).forEach((message) => {
      resolved.set(message, message.reference ?? EmptyReference);
    });
    return resolved;
  }, [messages]);
}
