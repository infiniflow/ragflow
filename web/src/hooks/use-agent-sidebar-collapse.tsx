import { useLocalStorageState } from 'ahooks';
import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  type PropsWithChildren,
} from 'react';

const AgentSidebarCollapsedStorageKey = 'ragflow-agent-sidebar-collapsed';

interface AgentSidebarCollapseContextValue {
  collapsed: boolean;
  setCollapsed: (collapsed: boolean) => void;
  toggleCollapsed: () => void;
}

const AgentSidebarCollapseContext =
  createContext<AgentSidebarCollapseContextValue | null>(null);

// Shared by the agent detail pages and the primary AppSidebar, which hosts
// the expand entry while the secondary sidebar is hidden.
export function AgentSidebarCollapseProvider({ children }: PropsWithChildren) {
  const [collapsed, setCollapsed] = useLocalStorageState<boolean>(
    AgentSidebarCollapsedStorageKey,
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
    <AgentSidebarCollapseContext.Provider value={value}>
      {children}
    </AgentSidebarCollapseContext.Provider>
  );
}

export function useAgentSidebarCollapse() {
  const context = useContext(AgentSidebarCollapseContext);
  if (!context) {
    throw new Error(
      'useAgentSidebarCollapse must be used within AgentSidebarCollapseProvider',
    );
  }
  return context;
}
