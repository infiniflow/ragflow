import { useSelectFileList } from '@/hooks/fileManagerHooks';
import { IFile } from '@/interfaces/database/file-manager';
import { formatDate } from '@/utils/date';
import { Button, Table } from 'antd';
import { ColumnsType } from 'antd/es/table';
import ActionCell from './action-cell';
import FileToolbar from './file-toolbar';
import {
  useGetRowSelection,
  useHandleCreateFolder,
  useNavigateToOtherFolder,
  useRenameCurrentFile,
  useSelectFileListLoading,
} from './hooks';

import RenameModal from '@/components/rename-modal';
import FolderCreateModal from './folder-create-modal';
import styles from './index.less';

const FileManager = () => {
  const fileList = useSelectFileList();
  const rowSelection = useGetRowSelection();
  const loading = useSelectFileListLoading();
  const navigateToOtherFolder = useNavigateToOtherFolder();
  const {
    fileRenameVisible,
    fileRenameLoading,
    hideFileRenameModal,
    showFileRenameModal,
    initialFileName,
    onFileRenameOk,
  } = useRenameCurrentFile();
  const {
    folderCreateModalVisible,
    showFolderCreateModal,
    hideFolderCreateModal,
    folderCreateLoading,
    onFolderCreateOk,
  } = useHandleCreateFolder();

  const columns: ColumnsType<IFile> = [
    {
      title: 'Name',
      dataIndex: 'name',
      key: 'name',
      render(value, record) {
        return record.type === 'folder' ? (
          <Button
            type={'link'}
            onClick={() => navigateToOtherFolder(record.id)}
          >
            {value}
          </Button>
        ) : (
          value
        );
      },
    },
    {
      title: 'Upload Date',
      dataIndex: 'create_date',
      key: 'create_date',
      render(text) {
        return formatDate(text);
      },
    },
    {
      title: 'Location',
      dataIndex: 'location',
      key: 'location',
    },
    {
      title: 'Action',
      dataIndex: 'action',
      key: 'action',
      render: (text, record) => (
        <ActionCell
          record={record}
          setCurrentRecord={(record: any) => {
            console.info(record);
          }}
          showRenameModal={showFileRenameModal}
        ></ActionCell>
      ),
    },
  ];

  return (
    <section className={styles.fileManagerWrapper}>
      <FileToolbar
        selectedRowKeys={rowSelection.selectedRowKeys as string[]}
        showFolderCreateModal={showFolderCreateModal}
      ></FileToolbar>
      <Table
        dataSource={fileList}
        columns={columns}
        rowKey={'id'}
        rowSelection={rowSelection}
        loading={loading}
      />
      <RenameModal
        visible={fileRenameVisible}
        hideModal={hideFileRenameModal}
        onOk={onFileRenameOk}
        initialName={initialFileName}
        loading={fileRenameLoading}
      ></RenameModal>
      <FolderCreateModal
        loading={folderCreateLoading}
        visible={folderCreateModalVisible}
        hideModal={hideFolderCreateModal}
        onOk={onFolderCreateOk}
      ></FolderCreateModal>
    </section>
  );
};

export default FileManager;
