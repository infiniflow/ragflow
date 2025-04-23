import ListFilterBar from '@/components/list-filter-bar';
import { RenameDialog } from '@/components/rename-dialog';
import { useFetchNextKnowledgeListByPage } from '@/hooks/use-knowledge-request';
import { pick } from 'lodash';
import { Plus } from 'lucide-react';
import { PropsWithChildren, useCallback } from 'react';
import { DatasetCard } from './dataset-card';
import { DatasetCreatingDialog } from './dataset-creating-dialog';
import { DatasetsFilterPopover } from './datasets-filter-popover';
import { DatasetsPagination } from './datasets-pagination';
import { useSaveKnowledge } from './hooks';
import { useRenameDataset } from './use-rename-dataset';

export default function Datasets() {
  const {
    visible,
    hideModal,
    showModal,
    onCreateOk,
    loading: creatingLoading,
  } = useSaveKnowledge();

  const {
    kbs,
    total,
    pagination,
    setPagination,
    handleInputChange,
    searchString,
    setOwnerIds,
    ownerIds,
  } = useFetchNextKnowledgeListByPage();

  const {
    datasetRenameLoading,
    initialDatasetName,
    onDatasetRenameOk,
    datasetRenameVisible,
    hideDatasetRenameModal,
    showDatasetRenameModal,
  } = useRenameDataset();

  const handlePageChange = useCallback(
    (page: number, pageSize?: number) => {
      setPagination({ page, pageSize });
    },
    [setPagination],
  );

  return (
    <section className="p-8 text-foreground">
      <ListFilterBar
        title="Datasets"
        showDialog={showModal}
        count={ownerIds.length}
        FilterPopover={({ children }: PropsWithChildren) => (
          <DatasetsFilterPopover setOwnerIds={setOwnerIds} ownerIds={ownerIds}>
            {children}
          </DatasetsFilterPopover>
        )}
        searchString={searchString}
        onSearchChange={handleInputChange}
      >
        <Plus className="mr-2 h-4 w-4" />
        Create dataset
      </ListFilterBar>
      <div className="grid gap-6 sm:grid-cols-1 md:grid-cols-2 lg:grid-cols-4 xl:grid-cols-6 2xl:grid-cols-8">
        {kbs.map((dataset) => {
          return (
            <DatasetCard
              dataset={dataset}
              key={dataset.id}
              showDatasetRenameModal={showDatasetRenameModal}
            ></DatasetCard>
          );
        })}
      </div>
      <div className="mt-8">
        <DatasetsPagination
          {...pick(pagination, 'current', 'pageSize')}
          total={total}
          onChange={handlePageChange}
        ></DatasetsPagination>
      </div>
      {visible && (
        <DatasetCreatingDialog
          hideModal={hideModal}
          onOk={onCreateOk}
          loading={creatingLoading}
        ></DatasetCreatingDialog>
      )}
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
