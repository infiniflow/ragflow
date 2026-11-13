import { useLocalStorageState } from 'ahooks';
import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  type PropsWithChildren,
} from 'react';

const SidebarCollapsedStorageKey = 'ragflow-sidebar-collapsed';

interface SidebarCollapseContextValue {
  collapsed: boolean;
  setCollapsed: (collapsed: boolean) => void;
  toggleCollapsed: () => void;
}

const SidebarCollapseContext =
  createContext<SidebarCollapseContextValue | null>(null);

// User-controlled collapse of the primary sidebar. Below the `md` breakpoint
// the sidebar is always an icon rail regardless of this flag, so the toggle
// only takes effect on `md+` screens.
export function SidebarCollapseProvider({ children }: PropsWithChildren) {
  const [collapsed, setCollapsed] = useLocalStorageState<boolean>(
    SidebarCollapsedStorageKey,
    { defaultValue: false },
  );

  const toggleCollapsed = useCallback(() => {
    setCollapsed((prev) => !prev);
  }, [setCollapsed]);

  const value = useMemo(
    () => ({ collapsed, setCollapsed, toggleCollapsed }),
    [collapsed, setCollapsed, toggleCollapsed],
  );

  return (
    <SidebarCollapseContext.Provider value={value}>
      {children}
    </SidebarCollapseContext.Provider>
  );
}

export function useSidebarCollapse() {
  const context = useContext(SidebarCollapseContext);
  if (!context) {
    throw new Error(
      'useSidebarCollapse must be used within SidebarCollapseProvider',
    );
  }
  return context;
}
