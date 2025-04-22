import ListFilterBar from '@/components/list-filter-bar';
import { RenameDialog } from '@/components/rename-dialog';
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchNextKnowledgeListByPage } from '@/hooks/use-knowledge-request';
import { formatDate } from '@/utils/date';
import { pick } from 'lodash';
import { ChevronRight, Ellipsis, Plus } from 'lucide-react';
import { PropsWithChildren, useCallback } from 'react';
import { DatasetCreatingDialog } from './dataset-creating-dialog';
import { DatasetDropdown } from './dataset-dropdown';
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
  const { navigateToDataset } = useNavigatePage();

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
        {kbs.map((dataset) => (
          <Card
            key={dataset.id}
            className="bg-colors-background-inverse-weak flex-1"
          >
            <CardContent className="p-4">
              <div className="flex justify-between mb-4">
                <Avatar className="w-[70px] h-[70px] rounded-lg">
                  <AvatarImage src={dataset.avatar} />
                  <AvatarFallback className="rounded-lg">CN</AvatarFallback>
                </Avatar>
                <DatasetDropdown
                  showDatasetRenameModal={showDatasetRenameModal}
                  dataset={dataset}
                >
                  <Button variant="ghost" size="icon">
                    <Ellipsis />
                  </Button>
                </DatasetDropdown>
              </div>
              <div className="flex justify-between items-end">
                <div>
                  <h3 className="text-lg font-semibold mb-2">{dataset.name}</h3>
                  <p className="text-sm opacity-80">{dataset.doc_num} files</p>
                  <p className="text-sm opacity-80">
                    Created {formatDate(dataset.update_time)}
                  </p>
                </div>
                <Button
                  variant="icon"
                  size="icon"
                  onClick={navigateToDataset(dataset.id)}
                >
                  <ChevronRight className="h-6 w-6" />
                </Button>
              </div>
            </CardContent>
          </Card>
        ))}
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
