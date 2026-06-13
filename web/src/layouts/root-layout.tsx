import { lazy, Suspense } from 'react';
import { Outlet } from 'react-router';
import { Header } from './components/header';

const DocAssistantWidget = lazy(
  () => import('@/components/doc-assistant'),
);

export function RootLayoutContainer({ children }: React.PropsWithChildren) {
  return (
    <div className="size-full grid grid-rows-[auto_1fr] grid-cols-1 grid-flow-col">
      <Header className="px-5 py-4" />

      <main className="size-full overflow-hidden">{children}</main>
      <Suspense fallback={null}>
        <DocAssistantWidget />
      </Suspense>
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
