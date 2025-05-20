import { RenameDialog } from '@/components/rename-dialog';
import { Button } from '@/components/ui/button';
import { CardSkeleton } from '@/components/ui/skeleton';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchNextKnowledgeListByPage } from '@/hooks/use-knowledge-request';
import { DatasetCard } from '../datasets/dataset-card';
import { useRenameDataset } from '../datasets/use-rename-dataset';

export function Datasets() {
  const { navigateToDatasetList } = useNavigatePage();
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
      <h2 className="text-2xl font-bold mb-6">Datasets</h2>
      <div className="flex gap-6">
        {loading ? (
          <div className="flex-1">
            <CardSkeleton />
          </div>
        ) : (
          <div className="flex gap-4 flex-1">
            {kbs.slice(0, 4).map((dataset) => (
              <DatasetCard
                key={dataset.id}
                dataset={dataset}
                showDatasetRenameModal={showDatasetRenameModal}
              ></DatasetCard>
            ))}
          </div>
        )}
        <Button
          className="h-auto "
          variant={'tertiary'}
          onClick={navigateToDatasetList}
        >
          See all
        </Button>
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
