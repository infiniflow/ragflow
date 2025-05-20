import { FileUploadDialog } from '@/components/file-upload-dialog';
import ListFilterBar from '@/components/list-filter-bar';
import { Upload } from 'lucide-react';
import { FilesTable } from './files-table';
import { useHandleUploadFile } from './use-upload-file';

export default function Files() {
  const {
    fileUploadVisible,
    hideFileUploadModal,
    showFileUploadModal,
    fileUploadLoading,
    onFileUploadOk,
  } = useHandleUploadFile();

  return (
    <section className="p-8">
      <ListFilterBar title="Files" showDialog={showFileUploadModal}>
        <Upload />
        Upload file
      </ListFilterBar>
      <FilesTable></FilesTable>
      {fileUploadVisible && (
        <FileUploadDialog
          hideModal={hideFileUploadModal}
          onOk={onFileUploadOk}
          loading={fileUploadLoading}
        ></FileUploadDialog>
      )}
    </section>
  );
}
