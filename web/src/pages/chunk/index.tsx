import { PageHeader } from '@/components/page-header';
import {
  QueryStringMap,
  useNavigatePage,
} from '@/hooks/logic-hooks/navigate-hooks';
import { Outlet } from 'umi';

export default function ChunkPage() {
  const { navigateToDataset, getQueryString } = useNavigatePage();
  return (
    <section>
      <PageHeader
        title="Editing block"
        back={navigateToDataset(
          getQueryString(QueryStringMap.KnowledgeId) as string,
        )}
      ></PageHeader>
      <Outlet />
    </section>
  );
}
