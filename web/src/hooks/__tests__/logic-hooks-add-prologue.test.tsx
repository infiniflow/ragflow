jest.mock('eventsource-parser/stream', () => ({}));
// logic-hooks pulls in @/routes, which builds a browser router at module
// scope and crashes in jsdom; the hooks under test never touch it.
jest.mock('@/routes', () => ({ Routes: {} }));

import { MessageType } from '@/constants/chat';
import { act, renderHook } from '@testing-library/react';
import { useSelectDerivedMessages } from '../logic-hooks';

describe('useSelectDerivedMessages addPrologue', () => {
  it('inserts the prologue into an empty message list', () => {
    const { result } = renderHook(() => useSelectDerivedMessages());

    act(() => {
      result.current.addPrologue('Hi there');
    });

    expect(result.current.derivedMessages).toHaveLength(1);
    expect(result.current.derivedMessages[0]).toMatchObject({
      role: MessageType.Assistant,
      content: 'Hi there',
    });
  });

  it('updates the prologue while the conversation has not started', () => {
    const { result } = renderHook(() => useSelectDerivedMessages());

    act(() => {
      result.current.addPrologue('Hi there');
    });
    act(() => {
      result.current.addPrologue('Hello again');
    });

    expect(result.current.derivedMessages).toHaveLength(1);
    expect(result.current.derivedMessages[0].content).toBe('Hello again');
  });

  it('does not overwrite history once the user has sent a message', () => {
    const { result } = renderHook(() => useSelectDerivedMessages());

    act(() => {
      result.current.addNewestOneQuestion({
        content: 'How is the weather',
        role: MessageType.User,
      } as any);
    });
    act(() => {
      result.current.addPrologue('Hi there');
    });

    expect(result.current.derivedMessages).toHaveLength(1);
    expect(result.current.derivedMessages[0].content).toBe(
      'How is the weather',
    );
  });
});
