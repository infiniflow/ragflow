import { MessageType } from '@/constants/chat';
import type { IMessage, IReference } from '@/interfaces/database/chat';
import { renderHook } from '@testing-library/react';
import { EmptyReference } from '../utils';
import { useMessageReferences } from './use-message-references';

it('resolves each answer reference independently of shared ids or answer order', () => {
  const reference: IReference = { chunks: [], doc_aggs: [], total: 1 };
  const messages: IMessage[] = [
    { id: 'turn', role: MessageType.User, content: 'question' },
    { id: 'turn', role: MessageType.Assistant, content: 'answer', reference },
    { id: 'next', role: MessageType.Assistant, content: '' },
  ];
  const { result, rerender } = renderHook(() => useMessageReferences(messages));

  expect(result.current.get(messages[0])).toBe(EmptyReference);
  expect(result.current.get(messages[1])).toBe(reference);
  expect(result.current.get(messages[2])).toBe(EmptyReference);
  const resolved = result.current;
  rerender();
  expect(result.current).toBe(resolved);
});

it('handles missing history', () => {
  const { result } = renderHook(() => useMessageReferences(undefined));
  expect(result.current.size).toBe(0);
});
