import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type PropsWithChildren,
} from 'react';

interface ChatSidebarCollapseContextValue {
  collapsed: boolean;
  setCollapsed: (collapsed: boolean) => void;
  toggleCollapsed: () => void;
}

const ChatSidebarCollapseContext =
  createContext<ChatSidebarCollapseContextValue | null>(null);

// Shared by the chat detail page and the primary AppSidebar, which hosts the
// expand entry while the secondary sidebar is hidden.
export function ChatSidebarCollapseProvider({ children }: PropsWithChildren) {
  const [collapsed, setCollapsed] = useState(false);

  const toggleCollapsed = useCallback(() => {
    setCollapsed((prev) => !prev);
  }, [setCollapsed]);

  const value = useMemo(
    () => ({ collapsed, setCollapsed, toggleCollapsed }),
    [collapsed, setCollapsed, toggleCollapsed],
  );

  return (
    <ChatSidebarCollapseContext.Provider value={value}>
      {children}
    </ChatSidebarCollapseContext.Provider>
  );
}

export function useChatSidebarCollapse() {
  const context = useContext(ChatSidebarCollapseContext);
  if (!context) {
    throw new Error(
      'useChatSidebarCollapse must be used within ChatSidebarCollapseProvider',
    );
  }
  return context;
}
