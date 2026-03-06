import { Outlet } from 'react-router';
import { Header } from './next-header';

export function NextLayoutContainer({ children }: React.PropsWithChildren) {
  return (
    <div className="size-full grid grid-rows-[auto_1fr] grid-cols-1 grid-flow-col">
      <Header className="px-5 py-4" />

      <main className="size-full overflow-hidden">{children}</main>
    </div>
  );
}

export default function NextLayout() {
  return (
    <NextLayoutContainer>
      <Outlet />
    </NextLayoutContainer>
  );
}
