import { useFetchKnowledgeBaseConfiguration } from '@/hooks/use-knowledge-request';
import { KnowledgeBaseProvider } from '@/pages/dataset/contexts/knowledge-base-context';

import { Outlet } from 'react-router';
import { SideBar } from './sidebar';

export default function DatasetWrapper() {
  const { data, loading } = useFetchKnowledgeBaseConfiguration();

  return (
    <KnowledgeBaseProvider knowledgeBase={data} loading={loading}>
      <article className="pt-3 size-full grid grid-cols-[auto_1fr] grid-rows-1">
        <SideBar dataset={data} />

        <Outlet />
      </article>
    </KnowledgeBaseProvider>
  );
}
