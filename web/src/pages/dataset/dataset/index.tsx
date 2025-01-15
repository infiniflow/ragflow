import { FileUploadDialog } from '@/components/file-upload-dialog';
import ListFilterBar from '@/components/list-filter-bar';
import { Upload } from 'lucide-react';
import { DatasetTable } from './dataset-table';
import { useHandleUploadDocument } from './hooks';

export default function Dataset() {
  const {
    documentUploadVisible,
    hideDocumentUploadModal,
    showDocumentUploadModal,
    onDocumentUploadOk,
    documentUploadLoading,
  } = useHandleUploadDocument();
  return (
    <section className="p-8">
      <ListFilterBar title="Files" showDialog={showDocumentUploadModal}>
        <Upload />
        Upload file
      </ListFilterBar>
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
