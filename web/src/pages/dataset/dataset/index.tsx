import { BulkOperateBar } from '@/components/bulk-operate-bar';
import { FileUploadDialog } from '@/components/file-upload-dialog';
import ListFilterBar from '@/components/list-filter-bar';
import { Button } from '@/components/ui/button';
import { Upload } from 'lucide-react';
import { DatasetTable } from './dataset-table';
import { useBulkOperateDataset } from './use-bulk-operate-dataset';
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

  return (
    <section className="p-8">
      <ListFilterBar title="Files">
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
      <DatasetTable></DatasetTable>
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
