import { Outlet } from 'umi';
import { Header } from './next-header';

export default function NextLayout() {
  return (
    <section>
      <Header></Header>
      <Outlet />
    </section>
  );
}
