import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type PropsWithChildren,
} from 'react';

interface DatasetSidebarCollapseContextValue {
  collapsed: boolean;
  setCollapsed: (collapsed: boolean) => void;
  toggleCollapsed: () => void;
}

const DatasetSidebarCollapseContext =
  createContext<DatasetSidebarCollapseContextValue | null>(null);

// User-controlled collapse of the dataset secondary sidebar. Shared between
// the dataset pages (which hide the sidebar) and the primary AppSidebar
// (which hosts the expand entry), so the provider sits at the root layout.
export function DatasetSidebarCollapseProvider({
  children,
}: PropsWithChildren) {
  const [collapsed, setCollapsed] = useState(false);

  const toggleCollapsed = useCallback(() => {
    setCollapsed((prev) => !prev);
  }, [setCollapsed]);

  const value = useMemo(
    () => ({ collapsed, setCollapsed, toggleCollapsed }),
    [collapsed, setCollapsed, toggleCollapsed],
  );

  return (
    <DatasetSidebarCollapseContext.Provider value={value}>
      {children}
    </DatasetSidebarCollapseContext.Provider>
  );
}

export function useDatasetSidebarCollapse() {
  const context = useContext(DatasetSidebarCollapseContext);
  if (!context) {
    throw new Error(
      'useDatasetSidebarCollapse must be used within DatasetSidebarCollapseProvider',
    );
  }
  return context;
}
