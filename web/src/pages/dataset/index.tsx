import { Outlet } from 'umi';
import { SideBar } from './sidebar';

export default function DatasetWrapper() {
  return (
    <div className="text-foreground flex flex-1">
      <SideBar></SideBar>
      <div className="flex-1">
        <Outlet />
      </div>
    </div>
  );
}
