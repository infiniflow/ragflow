import { CardContainer } from '@/components/card-container';
import { EmptyCardType } from '@/components/empty/constant';
import { EmptyAppCard } from '@/components/empty/empty';
import ListFilterBar from '@/components/list-filter-bar';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { useSetModalState } from '@/hooks/common-hooks';
import {
  useDeleteEvaluationDataset,
  useFetchEvaluationDatasets,
} from '@/hooks/use-evaluation-request';
import { Routes } from '@/routes';
import { formatPureDate } from '@/utils/date';
import { FlaskConical, Plus, Trash2 } from 'lucide-react';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { CreateEvaluationDatasetDialog } from './create-dataset-dialog';

export default function EvaluationsPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [page, setPage] = useState(1);
  const pageSize = 20;
  const { visible, showModal, hideModal } = useSetModalState();
  const { data, isLoading, refetch } = useFetchEvaluationDatasets(page, pageSize);
  const { mutateAsync: deleteDataset } = useDeleteEvaluationDataset();

  const datasets = data?.datasets ?? [];
  const total = data?.total ?? 0;

  const handlePageChange = useCallback((nextPage: number) => {
    setPage(nextPage);
  }, []);

  const handleDelete = async (id: string, e: React.MouseEvent) => {
    e.stopPropagation();
    await deleteDataset(id);
    refetch();
  };

  return (
    <>
      {datasets.length > 0 ? (
        <article className="size-full flex flex-col" data-testid="evaluations-list">
          <header className="px-5 pt-8 mb-4">
            <ListFilterBar
              icon="datasets"
              title={t('header.evaluations')}
              showSearch={false}
            >
              <Button onClick={showModal}>
                <Plus className="size-[1em]" />
                {t('evaluation.createDataset')}
              </Button>
            </ListFilterBar>
          </header>

          <CardContainer className="flex-1 overflow-auto px-5">
            {datasets.map((dataset) => (
              <Card
                key={dataset.id}
                className="cursor-pointer hover:border-text-primary transition-colors"
                onClick={() => navigate(`${Routes.Evaluation}/${dataset.id}`)}
              >
                <CardHeader className="flex flex-row items-start justify-between gap-4">
                  <div className="flex gap-3 min-w-0">
                    <FlaskConical className="size-5 mt-0.5 shrink-0 text-text-secondary" />
                    <div className="min-w-0">
                      <CardTitle className="text-base truncate">
                        {dataset.name}
                      </CardTitle>
                      {dataset.description && (
                        <CardDescription className="line-clamp-2 mt-1">
                          {dataset.description}
                        </CardDescription>
                      )}
                      <CardDescription className="mt-2">
                        {t('evaluation.kbCount', {
                          count: dataset.kb_ids?.length ?? 0,
                        })}
                        {' · '}
                        {formatPureDate(dataset.create_time)}
                      </CardDescription>
                    </div>
                  </div>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={(e) => handleDelete(dataset.id, e)}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </CardHeader>
              </Card>
            ))}
          </CardContainer>

          {total > pageSize && (
            <footer className="mt-4 px-5 pb-5">
              <RAGFlowPagination
                current={page}
                pageSize={pageSize}
                total={total}
                onChange={handlePageChange}
              />
            </footer>
          )}
        </article>
      ) : (
        <div className="flex-1 flex items-center justify-center">
          <EmptyAppCard
            showIcon
            size="large"
            className="w-[480px] p-14"
            type={EmptyCardType.Dataset}
            onClick={showModal}
            loading={isLoading}
          />
        </div>
      )}

      <CreateEvaluationDatasetDialog
        visible={visible}
        hideModal={hideModal}
        onSuccess={(id) => navigate(`${Routes.Evaluation}/${id}`)}
      />
    </>
  );
}
