import { Outlet } from 'umi';
import { SideBar } from './sidebar';

export default function DatasetWrapper() {
  return (
    <div className="text-foreground flex">
      <SideBar></SideBar>
      <div className="p-6">
        <Outlet />
      </div>
    </div>
  );
}
