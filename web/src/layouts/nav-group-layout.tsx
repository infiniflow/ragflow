import { ListPageCard } from '@/components/list-page-card';
import { Outlet } from 'react-router';
import { NavGroupTitle } from './components/nav-group-title';

// Shared card and primary-navigation title for Context and Application lists.
export default function NavGroupLayout() {
  return (
    <ListPageCard>
      <div className="shrink-0 px-5 pt-5">
        <NavGroupTitle />
      </div>
      <div className="min-h-0 min-w-0 flex-1">
        <Outlet />
      </div>
    </ListPageCard>
  );
}
