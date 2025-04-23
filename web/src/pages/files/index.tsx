import { FileUploadDialog } from '@/components/file-upload-dialog';
import ListFilterBar from '@/components/list-filter-bar';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Upload } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { CreateFolderDialog } from './create-folder-dialog';
import { FileBreadcrumb } from './file-breadcrumb';
import { FilesTable } from './files-table';
import { useHandleCreateFolder } from './use-create-folder';
import { useHandleUploadFile } from './use-upload-file';

export default function Files() {
  const { t } = useTranslation();
  const {
    fileUploadVisible,
    hideFileUploadModal,
    showFileUploadModal,
    fileUploadLoading,
    onFileUploadOk,
  } = useHandleUploadFile();

  const {
    folderCreateModalVisible,
    showFolderCreateModal,
    hideFolderCreateModal,
    folderCreateLoading,
    onFolderCreateOk,
  } = useHandleCreateFolder();

  const leftPanel = (
    <div>
      <FileBreadcrumb></FileBreadcrumb>
    </div>
  );

  return (
    <section className="p-8">
      <ListFilterBar leftPanel={leftPanel}>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant={'tertiary'} size={'sm'}>
              <Upload />
              {t('knowledgeDetails.addFile')}
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent className="w-56">
            <DropdownMenuItem onClick={showFileUploadModal}>
              {t('fileManager.uploadFile')}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={showFolderCreateModal}>
              {t('fileManager.newFolder')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </ListFilterBar>
      <FilesTable></FilesTable>
      {fileUploadVisible && (
        <FileUploadDialog
          hideModal={hideFileUploadModal}
          onOk={onFileUploadOk}
          loading={fileUploadLoading}
        ></FileUploadDialog>
      )}
      {folderCreateModalVisible && (
        <CreateFolderDialog
          loading={folderCreateLoading}
          visible={folderCreateModalVisible}
          hideModal={hideFolderCreateModal}
          onOk={onFolderCreateOk}
        ></CreateFolderDialog>
      )}
    </section>
  );
}
