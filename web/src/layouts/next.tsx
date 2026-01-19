import { Outlet } from 'react-router';
import { Header } from './next-header';

export default function NextLayout() {
  return (
    <main className="h-full flex flex-col">
      <Header />
      <Outlet />
    </main>
  );
}
