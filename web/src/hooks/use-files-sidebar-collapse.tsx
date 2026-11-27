import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type PropsWithChildren,
} from 'react';

interface FilesSidebarCollapseContextValue {
  collapsed: boolean;
  setCollapsed: (collapsed: boolean) => void;
  toggleCollapsed: () => void;
}

const FilesSidebarCollapseContext =
  createContext<FilesSidebarCollapseContextValue | null>(null);

// Shared by the Files layout and the primary AppSidebar, which hosts the
// expand entry while the Library secondary sidebar is hidden.
export function FilesSidebarCollapseProvider({ children }: PropsWithChildren) {
  const [collapsed, setCollapsed] = useState(false);

  const toggleCollapsed = useCallback(() => {
    setCollapsed((previous) => !previous);
  }, []);

  const value = useMemo(
    () => ({ collapsed, setCollapsed, toggleCollapsed }),
    [collapsed, toggleCollapsed],
  );

  return (
    <FilesSidebarCollapseContext.Provider value={value}>
      {children}
    </FilesSidebarCollapseContext.Provider>
  );
}

export function useFilesSidebarCollapse() {
  const context = useContext(FilesSidebarCollapseContext);
  if (!context) {
    throw new Error(
      'useFilesSidebarCollapse must be used within FilesSidebarCollapseProvider',
    );
  }
  return context;
}
