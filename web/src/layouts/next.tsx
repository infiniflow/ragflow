import { Outlet } from 'umi';
import { Header } from './next-header';

export default function NextLayout() {
  return (
    <section className="h-full flex flex-col text-colors-text-neutral-strong">
      <Header></Header>
      <Outlet />
    </section>
  );
}
