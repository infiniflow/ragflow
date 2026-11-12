import {
  RootLayoutBanner,
  RootLayoutContent,
  useRootLayoutChrome,
} from '@/ee/layouts/root-layout-shell';
import { Outlet } from 'react-router';
import { Header } from './components/header';

export function RootLayoutContainer({ children }: React.PropsWithChildren) {
  const { containerRef, showHeader } = useRootLayoutChrome();

  return (
    <div
      className="size-full min-w-0 grid grid-flow-col grid-cols-1 grid-rows-[auto_auto_1fr]"
      ref={containerRef}
    >
      <div>
        <RootLayoutBanner />
      </div>

      {showHeader && <Header className="px-5 py-4" />}

      <main className="size-full min-w-0 overflow-hidden">{children}</main>
    </div>
  );
}

export default function RootLayout() {
  return (
    <RootLayoutContainer>
      <RootLayoutContent>
        <Outlet />
      </RootLayoutContent>
    </RootLayoutContainer>
  );
}
