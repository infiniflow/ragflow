import { act, renderHook } from '@testing-library/react';
import { type PropsWithChildren } from 'react';
import {
  DatasetSidebarCollapseProvider,
  useDatasetSidebarCollapse,
} from './use-dataset-sidebar-collapse';

function Wrapper({ children }: PropsWithChildren) {
  return (
    <DatasetSidebarCollapseProvider>{children}</DatasetSidebarCollapseProvider>
  );
}

describe('useDatasetSidebarCollapse', () => {
  it('defaults to expanded', () => {
    const { result } = renderHook(() => useDatasetSidebarCollapse(), {
      wrapper: Wrapper,
    });

    expect(result.current.collapsed).toBe(false);
  });

  it('toggles the collapsed flag', () => {
    const { result } = renderHook(() => useDatasetSidebarCollapse(), {
      wrapper: Wrapper,
    });

    act(() => result.current.toggleCollapsed());
    expect(result.current.collapsed).toBe(true);

    act(() => result.current.toggleCollapsed());
    expect(result.current.collapsed).toBe(false);
  });

  it('throws outside the provider', () => {
    const consoleSpy = jest
      .spyOn(console, 'error')
      .mockImplementation(() => {});

    expect(() => renderHook(() => useDatasetSidebarCollapse())).toThrow(
      'useDatasetSidebarCollapse must be used within DatasetSidebarCollapseProvider',
    );

    consoleSpy.mockRestore();
  });
});
