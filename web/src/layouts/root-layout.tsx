import { useState } from 'react';
import { Outlet } from 'react-router';
import { ChatHistorySidebar, Header } from './components/header';

export function RootLayoutContainer({ children }: React.PropsWithChildren) {
  const [isChatSidebarOpen, setIsChatSidebarOpen] = useState(false);

  return (
    <div className="size-full grid grid-cols-[minmax(0,1fr)] grid-rows-[auto_minmax(0,1fr)] overflow-hidden xl:grid-cols-[auto_minmax(0,1fr)]">
      <ChatHistorySidebar
        open={isChatSidebarOpen}
        onOpenChange={setIsChatSidebarOpen}
      />

      <Header
        className="col-start-1 row-start-1 px-5 py-4 xl:col-start-2"
        open={isChatSidebarOpen}
        onOpenChange={setIsChatSidebarOpen}
      />

      <main className="col-start-1 row-start-2 size-full overflow-hidden xl:col-start-2">
        {children}
      </main>
    </div>
  );
}

export default function RootLayout() {
  return (
    <RootLayoutContainer>
      <Outlet />
    </RootLayoutContainer>
  );
}
