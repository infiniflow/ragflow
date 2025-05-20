import { PageHeader } from '@/components/page-header';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { Outlet } from 'umi';
import { SideBar } from './sidebar';

export default function DatasetWrapper() {
  const { navigateToDatasetList } = useNavigatePage();
  return (
    <section>
      <PageHeader
        title="Dataset details"
        back={navigateToDatasetList}
      ></PageHeader>
      <div className="flex flex-1">
        <SideBar></SideBar>
        <div className="flex-1">
          <Outlet />
        </div>
      </div>
    </section>
  );
}
