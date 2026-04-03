import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';
import { useFetchKnowledgeGraph } from '@/hooks/use-knowledge-request';
import { Trash2 } from 'lucide-react';
import React from 'react';
import { useTranslation } from 'react-i18next';
import ForceGraph from './force-graph';
import { useDeleteKnowledgeGraph } from './use-delete-graph';

const KnowledgeGraph: React.FC = () => {
  const { data } = useFetchKnowledgeGraph();
  const { t } = useTranslation();
  const { handleDeleteKnowledgeGraph } = useDeleteKnowledgeGraph();

  return (
    <section className={'w-full h-[90dvh] relative p-6'}>
      <ConfirmDeleteDialog onOk={handleDeleteKnowledgeGraph}>
        <Button
          variant="outline"
          size={'sm'}
          className="absolute right-0 top-0 z-50"
        >
          <Trash2 /> {t('common.delete')}
        </Button>
      </ConfirmDeleteDialog>
      <ForceGraph data={data?.graph} show></ForceGraph>
    </section>
  );
};

export default KnowledgeGraph;
