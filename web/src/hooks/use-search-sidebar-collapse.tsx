import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type PropsWithChildren,
} from 'react';

interface SearchSidebarCollapseContextValue {
  collapsed: boolean;
  setCollapsed: (collapsed: boolean) => void;
  toggleCollapsed: () => void;
}

const SearchSidebarCollapseContext =
  createContext<SearchSidebarCollapseContextValue | null>(null);

// Shared by the search detail page and the primary AppSidebar, which hosts the
// expand entry while the secondary sidebar is hidden.
export function SearchSidebarCollapseProvider({ children }: PropsWithChildren) {
  const [collapsed, setCollapsed] = useState(false);

  const toggleCollapsed = useCallback(() => {
    setCollapsed((prev) => !prev);
  }, []);

  const value = useMemo(
    () => ({ collapsed, setCollapsed, toggleCollapsed }),
    [collapsed, toggleCollapsed],
  );

  return (
    <SearchSidebarCollapseContext.Provider value={value}>
      {children}
    </SearchSidebarCollapseContext.Provider>
  );
}

export function useSearchSidebarCollapse() {
  const context = useContext(SearchSidebarCollapseContext);
  if (!context) {
    throw new Error(
      'useSearchSidebarCollapse must be used within SearchSidebarCollapseProvider',
    );
  }
  return context;
}
