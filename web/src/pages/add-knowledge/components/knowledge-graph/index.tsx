import { useFetchKnowledgeGraph } from '@/hooks/knowledge-hooks';
import React from 'react';
import ForceGraph from './force-graph';

const KnowledgeGraphModal: React.FC = () => {
  const { data } = useFetchKnowledgeGraph();

  return (
    <section className={'w-full h-full'}>
      <ForceGraph data={data?.graph} show></ForceGraph>
    </section>
  );
};

export default KnowledgeGraphModal;
