import Spotlight from '@/components/spotlight';
import { Outlet } from 'react-router';
import { SideBar } from './sidebar';

export default function DatasetWrapper() {
  return (
    <section className="flex h-full flex-col w-full pt-3">
      <div className="flex flex-1 min-h-0">
        <SideBar></SideBar>
        <div className=" relative flex-1 overflow-auto border-[0.5px] border-border-button p-5 rounded-md mr-5 mb-5">
          <Spotlight />
          <Outlet />
        </div>
      </div>
    </section>
  );
}
