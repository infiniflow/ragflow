import { renderHook } from '@testing-library/react';
import { useAutoResizeTextarea } from './use-auto-resize-textarea';
class FixtureResizeObserver {
  observe() {}
  disconnect() {}
  unobserve() {}
}
describe('textarea automatic border height', () => {
  const OriginalResizeObserver = globalThis.ResizeObserver;
  beforeEach(() => {
    globalThis.ResizeObserver =
      FixtureResizeObserver as unknown as typeof ResizeObserver;
  });
  afterEach(() => {
    globalThis.ResizeObserver = OriginalResizeObserver;
    document.body.replaceChildren();
  });
  function textarea(scrollHeight: number, boxSizing = 'border-box') {
    const el = document.createElement('textarea');
    Object.assign(el.style, {
      boxSizing,
      borderTopWidth: '1px',
      borderBottomWidth: '1px',
      paddingTop: '16px',
      paddingBottom: '16px',
      lineHeight: '24px',
      fontSize: '20px',
    });
    Object.defineProperty(el, 'scrollHeight', { value: scrollHeight });
    document.body.appendChild(el);
    return el;
  }
  test('fits a bordered single-line search input without clipping', () => {
    const el = textarea(56);
    const { result } = renderHook(() =>
      useAutoResizeTextarea({ current: el }, 'hello'),
    );
    expect(el.style.height).toBe('58px');
    expect(el.style.overflowY).toBe('hidden');
    expect(result.current).toBe(false);
  });
  test('enables scrolling when borders push content past the maximum', () => {
    const el = textarea(159);
    renderHook(() => useAutoResizeTextarea({ current: el }, 'many lines', 160));
    expect(el.style.height).toBe('160px');
    expect(el.style.overflowY).toBe('auto');
  });
  test('preserves existing content-box behavior', () => {
    const el = textarea(56, 'content-box');
    renderHook(() => useAutoResizeTextarea({ current: el }, 'hello'));
    expect(el.style.height).toBe('56px');
    expect(el.style.overflowY).toBe('hidden');
  });
});
