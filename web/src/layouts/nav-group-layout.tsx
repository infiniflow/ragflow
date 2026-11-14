import { Outlet } from 'react-router';
import { NavGroupTabs } from './components/nav-group-tabs';

// Pathless layout for sidebar groups whose sibling list pages share a
// pill-tab strip (Context, Application). The pages themselves stay
// unchanged; the tabs render above their own headers, including on
// loading and empty states.
export default function NavGroupLayout() {
  return (
    <div className="flex size-full min-w-0 flex-col">
      <div className="shrink-0 px-5 pt-8">
        <NavGroupTabs />
      </div>
      <div className="min-h-0 min-w-0 flex-1">
        <Outlet />
      </div>
    </div>
  );
}
