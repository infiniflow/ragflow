import { Outlet } from 'react-router';
import { SideBar } from './sidebar';

import { cn } from '@/lib/utils';

const UserSetting = () => {
  return (
    <section className="pt-8 size-full grid grid-cols-[auto_1fr] grid-rows-1">
      <SideBar></SideBar>

      <div className={cn('pr-6 pb-6 flex flex-1 rounded-lg overflow-hidden')}>
        <Outlet></Outlet>
      </div>
    </section>
  );
};

export default UserSetting;
