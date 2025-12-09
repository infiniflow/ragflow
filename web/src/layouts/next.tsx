import { Outlet } from 'umi';
import { Header } from './next-header';

export default function NextLayout() {
  return (
    <main className="h-full flex flex-col">
      <Header />
      <Outlet />
    </main>
  );
}
