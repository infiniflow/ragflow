import { BulkOperateBar } from '@/components/bulk-operate-bar';
import { FileUploadDialog } from '@/components/file-upload-dialog';
import ListFilterBar from '@/components/list-filter-bar';
import { Button } from '@/components/ui/button';
import { useFetchDocumentList } from '@/hooks/use-document-request';
import { Upload } from 'lucide-react';
import { DatasetTable } from './dataset-table';
import { useBulkOperateDataset } from './use-bulk-operate-dataset';
import { useSelectDatasetFilters } from './use-select-filters';
import { useHandleUploadDocument } from './use-upload-document';

export default function Dataset() {
  const {
    documentUploadVisible,
    hideDocumentUploadModal,
    showDocumentUploadModal,
    onDocumentUploadOk,
    documentUploadLoading,
  } = useHandleUploadDocument();
  const { list } = useBulkOperateDataset();
  const {
    searchString,
    documents,
    pagination,
    handleInputChange,
    setPagination,
    filterValue,
    handleFilterSubmit,
  } = useFetchDocumentList();
  const { filters } = useSelectDatasetFilters();

  return (
    <section className="p-8">
      <ListFilterBar
        title="Dataset"
        onSearchChange={handleInputChange}
        searchString={searchString}
        value={filterValue}
        onChange={handleFilterSubmit}
        filters={filters}
      >
        <Button
          variant={'tertiary'}
          size={'sm'}
          onClick={showDocumentUploadModal}
        >
          <Upload />
          Upload file
        </Button>
      </ListFilterBar>
      <BulkOperateBar list={list}></BulkOperateBar>
      <DatasetTable
        documents={documents}
        pagination={pagination}
        setPagination={setPagination}
      ></DatasetTable>
      {documentUploadVisible && (
        <FileUploadDialog
          hideModal={hideDocumentUploadModal}
          onOk={onDocumentUploadOk}
          loading={documentUploadLoading}
        ></FileUploadDialog>
      )}
    </section>
  );
}
