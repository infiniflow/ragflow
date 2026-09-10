import { Outlet } from 'react-router';
import { Header } from './components/header';

export function RootLayoutContainer({ children }: React.PropsWithChildren) {
  return (
    <div className="size-full min-w-0 grid grid-flow-col grid-cols-1 grid-rows-[auto_1fr] bg-cable-page">
      {/* Header bar keeps its own surface and hairline divider so page content
          scrolling underneath never touches the controls. */}
      <div className="border-b border-cable-divider bg-cable-surface">
        <Header />
      </div>

      <main className="size-full min-w-0 overflow-hidden">{children}</main>
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
