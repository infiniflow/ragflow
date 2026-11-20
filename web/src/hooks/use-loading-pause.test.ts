import { act, renderHook } from '@testing-library/react';
import { useLoadingPause } from './use-loading-pause';
describe('streaming pause indicator', () => {
  beforeEach(() => jest.useFakeTimers());
  afterEach(() => jest.useRealTimers());
  test('hides again until the resumed content has paused', () => {
    const { result, rerender } = renderHook(
      ({ content }) => useLoadingPause(true, content),
      { initialProps: { content: 'first' } },
    );
    expect(result.current).toBe(false);
    act(() => jest.advanceTimersByTime(600));
    expect(result.current).toBe(true);
    rerender({ content: 'first second' });
    expect(result.current).toBe(false);
    act(() => jest.advanceTimersByTime(599));
    expect(result.current).toBe(false);
    act(() => jest.advanceTimersByTime(1));
    expect(result.current).toBe(true);
  });
  test('stays hidden when loading ends or content is empty', () => {
    const { result, rerender } = renderHook(
      ({ loading, content }) => useLoadingPause(loading, content),
      { initialProps: { loading: true, content: 'answer' } },
    );
    act(() => jest.advanceTimersByTime(600));
    expect(result.current).toBe(true);
    rerender({ loading: false, content: 'answer' });
    expect(result.current).toBe(false);
    rerender({ loading: true, content: '' });
    act(() => jest.advanceTimersByTime(1000));
    expect(result.current).toBe(false);
  });
  test('cancels the pending indicator after unmount', () => {
    const { unmount } = renderHook(() => useLoadingPause(true, 'answer'));
    unmount();
    expect(jest.getTimerCount()).toBe(0);
  });
});
