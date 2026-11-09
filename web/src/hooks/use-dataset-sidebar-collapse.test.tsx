import { act, renderHook } from '@testing-library/react';
import { type PropsWithChildren } from 'react';
import {
  DatasetSidebarCollapseProvider,
  useDatasetSidebarCollapse,
} from './use-dataset-sidebar-collapse';

const DatasetSidebarCollapsedStorageKey = 'ragflow-dataset-sidebar-collapsed';

function Wrapper({ children }: PropsWithChildren) {
  return (
    <DatasetSidebarCollapseProvider>{children}</DatasetSidebarCollapseProvider>
  );
}

describe('useDatasetSidebarCollapse', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('defaults to expanded', () => {
    const { result } = renderHook(() => useDatasetSidebarCollapse(), {
      wrapper: Wrapper,
    });

    expect(result.current.collapsed).toBe(false);
  });

  it('toggles and persists the collapsed flag', () => {
    const { result } = renderHook(() => useDatasetSidebarCollapse(), {
      wrapper: Wrapper,
    });

    act(() => result.current.toggleCollapsed());

    expect(result.current.collapsed).toBe(true);
    expect(localStorage.getItem(DatasetSidebarCollapsedStorageKey)).toBe(
      'true',
    );
  });

  it('restores the persisted collapsed flag', () => {
    localStorage.setItem(DatasetSidebarCollapsedStorageKey, 'true');

    const { result } = renderHook(() => useDatasetSidebarCollapse(), {
      wrapper: Wrapper,
    });

    expect(result.current.collapsed).toBe(true);
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
