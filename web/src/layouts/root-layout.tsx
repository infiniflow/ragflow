import { Outlet } from 'react-router';
import { AppSidebar } from './components/app-sidebar';

export function RootLayoutContainer({ children }: React.PropsWithChildren) {
  return (
    <div className="size-full min-w-0 grid grid-cols-[auto_minmax(0,1fr)] grid-rows-1">
      <AppSidebar />

      <main className="size-full min-w-0 min-h-0 overflow-hidden">
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
