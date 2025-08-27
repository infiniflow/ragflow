import { IconFont } from '@/components/icon-font';
import { RenameDialog } from '@/components/rename-dialog';
import { CardSkeleton } from '@/components/ui/skeleton';
import { useFetchNextKnowledgeListByPage } from '@/hooks/use-knowledge-request';
import { useTranslation } from 'react-i18next';
import { DatasetCard, SeeAllCard } from '../datasets/dataset-card';
import { useRenameDataset } from '../datasets/use-rename-dataset';

export function Datasets() {
  const { t } = useTranslation();
  const { kbs, loading } = useFetchNextKnowledgeListByPage();
  const {
    datasetRenameLoading,
    initialDatasetName,
    onDatasetRenameOk,
    datasetRenameVisible,
    hideDatasetRenameModal,
    showDatasetRenameModal,
  } = useRenameDataset();

  return (
    <section>
      <h2 className="text-2xl font-bold mb-6 flex gap-2.5 items-center">
        <IconFont name="data" className="size-8"></IconFont>
        {t('header.knowledgeBase')}
      </h2>
      <div className="flex gap-6">
        {loading ? (
          <div className="flex-1">
            <CardSkeleton />
          </div>
        ) : (
          <div className="grid gap-6 sm:grid-cols-1 md:grid-cols-2 lg:grid-cols-4 xl:grid-cols-6 2xl:grid-cols-8 max-h-[78vh] overflow-auto">
            {kbs
              ?.slice(0, 6)
              .map((dataset) => (
                <DatasetCard
                  key={dataset.id}
                  dataset={dataset}
                  showDatasetRenameModal={showDatasetRenameModal}
                ></DatasetCard>
              ))}
            <div className="min-h-24">
              <SeeAllCard></SeeAllCard>
            </div>
          </div>
        )}
      </div>
      {datasetRenameVisible && (
        <RenameDialog
          hideModal={hideDatasetRenameModal}
          onOk={onDatasetRenameOk}
          initialName={initialDatasetName}
          loading={datasetRenameLoading}
        ></RenameDialog>
      )}
    </section>
  );
}
