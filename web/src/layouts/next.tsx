import { Outlet } from 'umi';
import { Header } from './next-header';

export default function NextLayout() {
  return (
    <section className="h-full flex flex-col">
      <Header></Header>
      <Outlet />
    </section>
  );
}
