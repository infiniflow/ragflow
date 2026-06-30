import { Outlet } from 'react-router';
import { SideBar } from './sidebar';

import { cn } from '@/lib/utils';

const UserSetting = () => {
  return (
    <section className="pt-8 size-full grid grid-cols-[4rem_minmax(0,1fr)] md:grid-cols-[303px_minmax(0,1fr)] grid-rows-1 min-w-0">
      <SideBar />

      <div
        className={cn(
          'pr-2 md:pr-6 pb-6 flex flex-1 min-w-0 rounded-lg overflow-hidden',
        )}
      >
        <Outlet />
      </div>
    </section>
  );
};

export default UserSetting;
