import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import { useFetchKnowledgeGraph } from '@/hooks/use-knowledge-request';
import { LucideTrash2 } from 'lucide-react';
import React from 'react';
import { useTranslation } from 'react-i18next';
import ForceGraph from './force-graph';
import { useDeleteKnowledgeGraph } from './use-delete-graph';

const KnowledgeGraph: React.FC = () => {
  const { data } = useFetchKnowledgeGraph();
  const { t } = useTranslation();
  const { handleDeleteKnowledgeGraph } = useDeleteKnowledgeGraph();

  return (
    <Card
      as="article"
      className="relative me-5 mb-5 p-0 bg-transparent shadow-none overflow-hidden"
    >
      <ConfirmDeleteDialog onOk={handleDeleteKnowledgeGraph}>
        <Button
          variant="outline"
          size="sm"
          className="absolute right-5 top-5 z-50"
        >
          <LucideTrash2 />
          {t('common.delete')}
        </Button>
      </ConfirmDeleteDialog>

      <ForceGraph data={data?.graph} show />
    </Card>
  );
};

export default KnowledgeGraph;
