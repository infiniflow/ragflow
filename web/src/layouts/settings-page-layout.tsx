import { Outlet } from 'react-router';

// Hosts the settings pages that live under the global sidebar
// (Integrations children and Team). Full-height flex column so pages
// with their own inner sidebars or `flex-1 h-0` card bodies stretch
// and scroll correctly. PageFrame normally owns right/bottom spacing;
// this layout owns all four sides, so suppress its duplicate padding.
export default function SettingsPageLayout() {
  return (
    <div className="flex size-full min-w-0 flex-col p-2.5 [&>*]:min-h-0 [&>.page-frame]:pr-0 [&>.page-frame]:pb-0">
      <Outlet />
    </div>
  );
}
