import { PageHeader } from '@/components/page-header';
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchKnowledgeBaseConfiguration } from '@/hooks/use-knowledge-request';
import { useTranslation } from 'react-i18next';
import { Outlet } from 'umi';
import { SideBar } from './sidebar';

export default function DatasetWrapper() {
  const { navigateToDatasetList } = useNavigatePage();
  const { t } = useTranslation();
  const { data } = useFetchKnowledgeBaseConfiguration();

  return (
    <section className="flex h-full flex-col w-full">
      <PageHeader>
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink onClick={navigateToDatasetList}>
                {t('knowledgeDetails.dataset')}
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage className="w-28 whitespace-nowrap text-ellipsis overflow-hidden">
                {data.name}
              </BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>
      </PageHeader>
      <div className="flex flex-1 min-h-0">
        <SideBar></SideBar>
        <div className="flex-1">
          <Outlet />
        </div>
      </div>
    </section>
  );
}
