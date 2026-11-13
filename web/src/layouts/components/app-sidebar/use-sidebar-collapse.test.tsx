import { act, renderHook } from '@testing-library/react';
import { type PropsWithChildren } from 'react';
import {
  SidebarCollapseProvider,
  useSidebarCollapse,
} from './use-sidebar-collapse';

const SidebarCollapsedStorageKey = 'ragflow-sidebar-collapsed';

function Wrapper({ children }: PropsWithChildren) {
  return <SidebarCollapseProvider>{children}</SidebarCollapseProvider>;
}

describe('useSidebarCollapse', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('defaults to expanded', () => {
    const { result } = renderHook(() => useSidebarCollapse(), {
      wrapper: Wrapper,
    });

    expect(result.current.collapsed).toBe(false);
  });

  it('toggles and persists the collapsed flag', () => {
    const { result } = renderHook(() => useSidebarCollapse(), {
      wrapper: Wrapper,
    });

    act(() => result.current.toggleCollapsed());

    expect(result.current.collapsed).toBe(true);
    expect(localStorage.getItem(SidebarCollapsedStorageKey)).toBe('true');
  });

  it('restores the persisted collapsed flag', () => {
    localStorage.setItem(SidebarCollapsedStorageKey, 'true');

    const { result } = renderHook(() => useSidebarCollapse(), {
      wrapper: Wrapper,
    });

    expect(result.current.collapsed).toBe(true);
  });

  it('throws outside the provider', () => {
    const consoleSpy = jest
      .spyOn(console, 'error')
      .mockImplementation(() => {});

    expect(() => renderHook(() => useSidebarCollapse())).toThrow(
      'useSidebarCollapse must be used within SidebarCollapseProvider',
    );

    consoleSpy.mockRestore();
  });
});
