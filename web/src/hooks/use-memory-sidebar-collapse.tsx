import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type PropsWithChildren,
} from 'react';

interface MemorySidebarCollapseContextValue {
  collapsed: boolean;
  setCollapsed: (collapsed: boolean) => void;
  toggleCollapsed: () => void;
}

const MemorySidebarCollapseContext =
  createContext<MemorySidebarCollapseContextValue | null>(null);

export function MemorySidebarCollapseProvider({ children }: PropsWithChildren) {
  const [collapsed, setCollapsed] = useState(false);

  const toggleCollapsed = useCallback(() => {
    setCollapsed((previous) => !previous);
  }, []);

  const value = useMemo(
    () => ({ collapsed, setCollapsed, toggleCollapsed }),
    [collapsed, toggleCollapsed],
  );

  return (
    <MemorySidebarCollapseContext.Provider value={value}>
      {children}
    </MemorySidebarCollapseContext.Provider>
  );
}

export function useMemorySidebarCollapse() {
  const context = useContext(MemorySidebarCollapseContext);
  if (!context) {
    throw new Error(
      'useMemorySidebarCollapse must be used within MemorySidebarCollapseProvider',
    );
  }
  return context;
}
