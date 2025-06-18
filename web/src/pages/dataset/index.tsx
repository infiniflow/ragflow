import { PageHeader } from '@/components/page-header';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useState } from 'react';
import { Outlet, useLocation } from 'umi';
import SettingContext from './data-set-context';
import { SideBar } from './sidebar';

export default function DatasetWrapper() {
  const { navigateToDatasetList } = useNavigatePage();

  const pageUrl = useLocation();
  const [_, kb_id] = pageUrl.pathname.match(/\/([^/]+)$/)!;

  const [refreshCount, setRefreshCount] = useState(1); // reload the avatar url on the top left corner

  return (
    <SettingContext.Provider value={{ setRefreshCount, kb_id }}>
      <section>
        <PageHeader
          title="Dataset details"
          back={navigateToDatasetList}
        ></PageHeader>
        <div className="flex flex-1">
          <SideBar refreshCount={refreshCount}></SideBar>
          <div className="flex-1">
            <Outlet />
          </div>
        </div>
      </section>
    </SettingContext.Provider>
  );
}
