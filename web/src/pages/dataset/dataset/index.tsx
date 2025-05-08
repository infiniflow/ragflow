import { BulkOperateBar } from '@/components/bulk-operate-bar';
import { FileUploadDialog } from '@/components/file-upload-dialog';
import ListFilterBar from '@/components/list-filter-bar';
import { RenameDialog } from '@/components/rename-dialog';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useRowSelection } from '@/hooks/logic-hooks/use-row-selection';
import { useFetchDocumentList } from '@/hooks/use-document-request';
import { Upload } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { DatasetTable } from './dataset-table';
import { useBulkOperateDataset } from './use-bulk-operate-dataset';
import { useCreateEmptyDocument } from './use-create-empty-document';
import { useSelectDatasetFilters } from './use-select-filters';
import { useHandleUploadDocument } from './use-upload-document';

export default function Dataset() {
  const { t } = useTranslation();
  const {
    documentUploadVisible,
    hideDocumentUploadModal,
    showDocumentUploadModal,
    onDocumentUploadOk,
    documentUploadLoading,
  } = useHandleUploadDocument();

  const {
    searchString,
    documents,
    pagination,
    handleInputChange,
    setPagination,
    filterValue,
    handleFilterSubmit,
    loading,
  } = useFetchDocumentList();
  const { filters } = useSelectDatasetFilters();

  const {
    createLoading,
    onCreateOk,
    createVisible,
    hideCreateModal,
    showCreateModal,
  } = useCreateEmptyDocument();

  const { rowSelection, rowSelectionIsEmpty, setRowSelection, selectedCount } =
    useRowSelection();

  const { list } = useBulkOperateDataset({
    documents,
    rowSelection,
    setRowSelection,
  });

  return (
    <section className="p-5">
      <ListFilterBar
        title="Dataset"
        onSearchChange={handleInputChange}
        searchString={searchString}
        value={filterValue}
        onChange={handleFilterSubmit}
        filters={filters}
        leftPanel={
          <div className="items-start">
            <div className="pb-1">Dataset</div>
            <div className="text-text-sub-title-invert text-sm">
              Please wait for your files to finish parsing before starting an
              AI-powered chat.
            </div>
          </div>
        }
      >
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button size={'sm'}>
              <Upload />
              {t('knowledgeDetails.addFile')}
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent className="w-56">
            <DropdownMenuItem onClick={showDocumentUploadModal}>
              {t('fileManager.uploadFile')}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={showCreateModal}>
              {t('fileManager.newFolder')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </ListFilterBar>
      {rowSelectionIsEmpty || (
        <BulkOperateBar list={list} count={selectedCount}></BulkOperateBar>
      )}
      <DatasetTable
        documents={documents}
        pagination={pagination}
        setPagination={setPagination}
        rowSelection={rowSelection}
        setRowSelection={setRowSelection}
        loading={loading}
      ></DatasetTable>
      {documentUploadVisible && (
        <FileUploadDialog
          hideModal={hideDocumentUploadModal}
          onOk={onDocumentUploadOk}
          loading={documentUploadLoading}
        ></FileUploadDialog>
      )}
      {createVisible && (
        <RenameDialog
          hideModal={hideCreateModal}
          onOk={onCreateOk}
          loading={createLoading}
          title={'File Name'}
        ></RenameDialog>
      )}
    </section>
  );
}
