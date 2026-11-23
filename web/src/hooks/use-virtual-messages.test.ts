import { useVirtualizer } from '@tanstack/react-virtual';
import { act, fireEvent, renderHook } from '@testing-library/react';
import { useVirtualMessages } from './use-virtual-messages';

jest.mock('@tanstack/react-virtual', () => ({
  useVirtualizer: jest.fn(),
}));

jest.mock('@/utils/chat', () => ({
  buildMessageUuidWithRole: jest.fn(),
}));

const MockUseVirtualizer = jest.mocked(useVirtualizer);
const OriginalResizeObserver = globalThis.ResizeObserver;
const MockObserve = jest.fn();
const MockDisconnect = jest.fn();
const MockVirtualizer = {
  getTotalSize: jest.fn(),
  scrollToEnd: jest.fn(),
};
let resizeCallback: ResizeObserverCallback;
let container: HTMLDivElement;
let containerRef: { current: HTMLDivElement };

beforeEach(() => {
  jest.clearAllMocks();
  globalThis.ResizeObserver = jest.fn().mockImplementation((callback) => {
    resizeCallback = callback;
    return { observe: MockObserve, disconnect: MockDisconnect };
  });
  MockVirtualizer.getTotalSize.mockReturnValue(1000);
  MockUseVirtualizer.mockReturnValue(
    MockVirtualizer as unknown as import('@tanstack/react-virtual').ReactVirtualizer<
      Element,
      Element
    >,
  );
  container = document.createElement('div');
  Object.defineProperties(container, {
    scrollHeight: { value: 1000, writable: true },
    clientHeight: { value: 200, writable: true },
    scrollTop: { value: 800, writable: true },
  });
  containerRef = { current: container };
});

afterEach(() => {
  globalThis.ResizeObserver = OriginalResizeObserver;
});

function setContentHeight(height: number) {
  Object.defineProperty(container, 'scrollHeight', { value: height });
  MockVirtualizer.getTotalSize.mockReturnValue(height);
}

describe('useVirtualMessages bottom state', () => {
  it('rechecks content removal without a scroll or container resize event', () => {
    container.scrollTop = 600;
    const { result, rerender } = renderHook(() =>
      useVirtualMessages([], containerRef),
    );
    expect(result.current.isAtBottom).toBe(false);

    setContentHeight(800);
    rerender();

    expect(result.current.isAtBottom).toBe(true);
  });

  it('rechecks content growth without a scroll event', () => {
    const { result, rerender } = renderHook(() =>
      useVirtualMessages([], containerRef),
    );
    expect(result.current.isAtBottom).toBe(true);

    setContentHeight(1100);
    rerender();

    expect(result.current.isAtBottom).toBe(false);
  });

  it('stays at the bottom when the virtualizer follows growing content', () => {
    const { result, rerender } = renderHook(() =>
      useVirtualMessages([], containerRef),
    );
    setContentHeight(1100);
    container.scrollTop = 900;
    rerender();

    expect(result.current.isAtBottom).toBe(true);
  });

  it('uses the same 40 px boundary for following and the bottom button', () => {
    const { result } = renderHook(() => useVirtualMessages([], containerRef));
    const options = MockUseVirtualizer.mock.calls[0][0];
    expect(options.scrollEndThreshold).toBe(40);
    expect(options.followOnAppend).toBe(true);

    container.scrollTop = 760;
    fireEvent.scroll(container);
    expect(result.current.isAtBottom).toBe(true);

    container.scrollTop = 755;
    fireEvent.scroll(container);
    expect(result.current.isAtBottom).toBe(false);

    container.scrollTop = 800;
    fireEvent.scroll(container);
    expect(result.current.isAtBottom).toBe(true);
    act(() => result.current.scrollToBottom());
    expect(MockVirtualizer.scrollToEnd).toHaveBeenCalledTimes(2);
  });

  it('rechecks viewport resizing and releases observers on unmount', () => {
    const { result, unmount } = renderHook(() =>
      useVirtualMessages([], containerRef),
    );
    expect(MockObserve).toHaveBeenCalledWith(container);
    container.scrollTop = 700;
    act(() => resizeCallback([], {} as ResizeObserver));
    expect(result.current.isAtBottom).toBe(false);

    unmount();
    expect(MockDisconnect).toHaveBeenCalledTimes(1);
    container.scrollTop = 800;
    fireEvent.scroll(container);
    expect(result.current.isAtBottom).toBe(false);
  });
});
